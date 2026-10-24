import { createApp, ref, computed, watch, onMounted, onUnmounted, onUpdated, nextTick } from 'vue/dist/vue.esm-bundler.js'
import { Search, X, ArrowRight, RotateCw, Users, Paperclip, Download, Check, Pin, BellOff, Info, ChevronRight, ChevronDown, Clock3, ImagePlus, MoreHorizontal, MessageCircle, UserPlus, Bell, Contact, UsersRound, Minus, Square } from 'lucide-vue-next'
import { highlightMatches } from './message-search.js'
import { filterMembers } from './member-list.js'
import { filterDirectory, friendSearchFields, conversationSearchFields } from './directory-search.js'
import { toggleConversationSelection } from './conversation-selection.js'
import { appendPage, PendingSends, Requests, SyncTasks } from './realtime.js'
import { canChangeRole, canEditAnnouncement, canInvite, canLeave, canManageMembers, canRemoveMember, canTransferOwnership, roleLabel, roleOf } from './group.js'
import { groupByPin, showsUnreadBadge } from './preferences.js'
import { activeMentionToken, applyMention, canRecall, mentionCandidates, messageBody, recalledLabel, replyExcerpt, replyLabel } from './reply.js'
import { hasThread, hiddenReplyCount, replyTargetForWindow, threadHasMore, threadLabel, threadPreview, threadRootID } from './thread.js'
import { availableInitials, filterCandidates, groupByInitial, keepSelectable, OTHER_INITIAL, selectAllCandidates, selectionSummary, toggleSelection } from './invite.js'
import { formatBytes, mediaPreview, mediaURL, validateMediaFile } from './media.js'
import { collectImages, hasTransferableFiles, prepareImage } from './image-input.js'
import { hasBlocks, messageSegments, nextImageIndex } from './composer-blocks.js'
import { caretRange, collectPieces, deleteImageNearCaret, hasContent, imageIDsIn, insertImageAtCaret, piecesToBlocks, piecesToText, placeCaretAfter, removeImageFromComposer, renderInto, textBeforeCaret } from './composer-dom.js'
import { installEmojiPicker } from './emoji-picker.js'
import { useMessageActions } from './message-actions.js'
import { api, uploadMedia } from './api.js'
import { onImageLinkClick } from './image-viewer.js'
import { chatStorage, createSocket, installDesktopDownloads, isDesktop } from './desktop.js'
import { DesktopSettings } from './desktop-settings.js'
import { template } from './template.js'
import './style.css'
import './desktop.css'

// 头像组件：有头像用图片，否则回退到首字母占位。
const UserAvatar = {
  props: { user: { type: Object, default: null }, name: { type: String, default: '?' }, size: { type: String, default: '' } },
  setup(props) {
    const url = computed(() => (props.user?.avatar ? mediaURL(props.user.avatar) : ''))
    return { url }
  },
  template: '<img v-if="url" :src="url" class="avatar" :class="size" alt="" loading="lazy" /><span v-else class="avatar" :class="size">{{ (name || "?").slice(0, 1) }}</span>',
}

createApp({
  components: { Search, X, ArrowRight, RotateCw, Users, Paperclip, Download, Check, Pin, BellOff, Info, ChevronRight, ChevronDown, Clock3, ImagePlus, MoreHorizontal, MessageCircle, UserPlus, Bell, Contact, UsersRound, Minus, Square, DesktopSettings, UserAvatar },
  setup() {
    const me = ref(null)
    const mode = ref('login')
    const email = ref('')
    const password = ref('')
    const name = ref('')
    const error = ref('')
    const users = ref([])
    const friends = ref([])
    const conversations = ref([])
    const active = ref(null)
    const chatVisible = ref(false)
    const messages = ref([])
    const pendingSend = ref(null)
    const draft = ref('')
    const groupName = ref('')
    const friendQuery = ref('')
    const inviteUsers = ref([])
    // The picker narrows a potentially long user list, so its own search box and
    // initial filter live here rather than sharing the directory search state.
    const inviteQuery = ref('')
    const inviteInitial = ref('')
    const conversationFilter = ref('current')
    const directoryQuery = ref('')
    const directoryInput = ref(null)
    const showCreate = ref(false)
    const showAddUser = ref(false)
    const showInvite = ref(false)
    const showMembers = ref(false)
    const preferenceBusy = ref(false)
    const memberQuery = ref('')
    const memberBusy = ref(false)
    const groupBusy = ref(false)
    const draftArea = ref(null)
    const mentionQuery = ref(null)
    const mentionIndex = ref(0)
    const mentionList = ref([])
    const globalQuery = ref('')
    const globalResults = ref([])
    const globalBusy = ref(false)
    const globalError = ref('')
    const globalHasMore = ref(false)
    const globalCursor = ref({ time: '', id: '' })
    const globalStarted = ref(false)
    const showGlobalSearch = ref(false)
    const globalInput = ref(null)
    const showSettings = ref(false)
    const showProfile = ref(false)
    const showFriendRequests = ref(false)
    const friendRequests = ref({ incoming: [], outgoing: [] })
    const friendRequestBusy = ref(false)
    const requestMessage = ref('')
    const requestTarget = ref(null)
    const showRequestComposer = ref(false)
    // Contact management (remark, deletion, blocking) lives in one dialog.
    const showContacts = ref(false)
    const contactBusy = ref(false)
    const remarkEditingID = ref('')
    const remarkDraft = ref('')
    const blockedUsers = ref([])
    // Group governance: settings toggles, join approval queue, per-member tools.
    const groupSettingsBusy = ref(false)
    const joinRequests = ref([])
    const joinRequestBusy = ref(false)
    const groupToolBusy = ref(false)
    // Emoji responses are toggled from the message context menu.
    const REACTION_CHOICES = ['👍', '❤️', '😂', '🎉', '😮', '😢']
    const reactionBusy = ref(false)
    // Typing state is ephemeral: each entry carries an expiry the tick prunes.
    const typingUsers = ref({})
    const typingTick = ref(0)
    const typingSentAt = {}
    // Right-click menu on sidebar conversations (pin, mute, delete).
    const conversationMenu = ref(null)
    const conversationMenuElement = ref(null)
    // 顶部操作统一收进两个“更多”菜单：侧边栏与聊天标题栏。
    const sidebarMoreOpen = ref(false)
    const chatMoreOpen = ref(false)
    function toggleSidebarMore() { sidebarMoreOpen.value = !sidebarMoreOpen.value; chatMoreOpen.value = false }
    function toggleChatMore() { chatMoreOpen.value = !chatMoreOpen.value; sidebarMoreOpen.value = false }
    // 发送键偏好与底部动作（QQ NT 式分割发送按钮）。
    const sendKeyMode = ref(localStorage.getItem('im.sendKey') === 'ctrl' ? 'ctrl' : 'enter')
    const sendMenuOpen = ref(false)
    function setSendKey(mode) {
      sendKeyMode.value = mode
      localStorage.setItem('im.sendKey', mode)
      sendMenuOpen.value = false
    }
    const sendKeyHint = computed(() => sendKeyMode.value === 'ctrl' ? 'Ctrl + Enter 发送 · Enter 换行' : 'Enter 发送 · Shift + Enter 换行')
    function closeConversation() {
      if (!current.value) return
      active.value = null
      chatVisible.value = false
    }
    const memberSearchOpen = ref(false)
    async function toggleMemberSearch() {
      memberSearchOpen.value = !memberSearchOpen.value
      if (memberSearchOpen.value) { await nextTick(); document.querySelector('.member-block-search input')?.focus() }
    }
    // Per-member tools collapse into one dropdown so rows never overflow.
    const memberMenuID = ref('')
    function toggleMemberMenu(id) {
      memberMenuID.value = memberMenuID.value === id ? '' : id
    }
    const profileEditing = ref(false)
    const profileBusy = ref(false)
    const profileName = ref('')
    const profileEmail = ref('')
    const passwordEditing = ref(false)
    const passwordBusy = ref(false)
    const currentPassword = ref('')
    const newPassword = ref('')
    const confirmPassword = ref('')
    const conversationBusy = ref(false)
    const friendBusy = ref(false)
    const connected = ref(false)
    const loading = ref(false)
    const stream = ref(null)
    const sending = ref(false)
    const mediaInput = ref(null)
    const uploadBusy = ref(false)
    // Images live inside the draft as tokens, so one message can interleave text and
    // pictures in the order they were added. `attachment` remains for a video or a
    // generic file, which occupy a message on their own.
    // Mirrors the server's block limit: every image is one block in the message.
    const MAX_COMPOSER_IMAGES = 20
    const composerImages = ref([])
    const attachment = ref(null)
    const previewFile = ref(null)
    const visibleCount = ref(100)
    const showSearch = ref(false)
    const searchInput = ref(null)
    const searchQuery = ref('')
    const searchedQuery = ref('')
    const searchResults = ref([])
    const searchBusy = ref(false)
    const searchError = ref('')
    const searchHasMore = ref(false)
    const searchBefore = ref(0)
    const searchStarted = ref(false)
    const locatingMessage = ref(null)
    const highlightedMessage = ref(null)
    let searchController
    let searchVersion = 0
    let locateVersion = 0
    let highlightTimer
    const unreadCounts = ref({})
    const cacheVersion = ref(0)
    const session = chatStorage()
    const saved = chatStorage(true)
    const deviceID = session.getItem('im.device') || crypto.randomUUID()
    session.setItem('im.device', deviceID)
    const cache = new Map()
    const syncing = new SyncTasks()
    let socket
    let requests
    let pendingStore
    let reconnectTimer
    let refreshTimer
    let errorTimer
    let refreshTask
    let refreshTicks = 0
    let typingTimer
    let epoch = 0
    let disposed = false

    const current = computed(() => conversations.value.find(item => item.id === active.value))
    const latestMessage = item => item && cache.get(item.id)?.at(-1)
    const latestTime = item => Date.parse(latestMessage(item)?.created_at || '') || 0
    const sortByLatestTime = items => items.sort((left, right) => {
      const difference = latestTime(right) - latestTime(left)
      if (difference) return difference
      return (left.name || '').localeCompare(right.name || '', 'zh-CN')
    })
    const directConversation = userID => conversations.value.find(item => item.kind === 'direct' && item.members.some(member => member.id === userID))
    // Pinned conversations are surfaced separately so they stay in view even
    // when other conversations receive newer messages.
    const pinnedConversations = computed(() => {
      cacheVersion.value
      return groupByPin(conversations.value).pinned
    })
    const recentConversations = computed(() => {
      cacheVersion.value
      return sortByLatestTime(conversations.value.filter(item => latestMessage(item)))
    })
    const groupConversations = computed(() => {
      cacheVersion.value
      return sortByLatestTime(conversations.value.filter(item => item.kind === 'group'))
    })
    const pendingIncomingRequests = computed(() => friendRequests.value.incoming.filter(item => item.status === 'pending'))
    const friendRequestFor = userID => friendRequests.value.outgoing.find(item => item.recipient_id === userID && item.status === 'pending')
    const friendRequestStatus = status => ({ pending: '已发送', accepted: '已同意', rejected: '已拒绝' }[status] || status)
    const sortedFriends = computed(() => {
      cacheVersion.value
      return [...friends.value].sort((left, right) => {
        const difference = latestTime(directConversation(right.id)) - latestTime(directConversation(left.id))
        if (difference) return difference
        return left.name.localeCompare(right.name, 'zh-CN')
      })
    })
    const filteredFriends = computed(() => filterDirectory(sortedFriends.value, directoryQuery.value, friendSearchFields))
    const filteredGroups = computed(() => filterDirectory(groupConversations.value, directoryQuery.value, item => conversationSearchFields(item, me.value?.id)))
    const filteredRecent = computed(() => filterDirectory(recentConversations.value, directoryQuery.value, item => conversationSearchFields(item, me.value?.id)))
    const directoryPlaceholder = computed(() => conversationFilter.value === 'friends' ? '搜索好友昵称或账号' : conversationFilter.value === 'group' ? '搜索群聊名称' : '搜索当前会话')
    const directoryCount = computed(() => conversationFilter.value === 'friends' ? filteredFriends.value.length : conversationFilter.value === 'group' ? filteredGroups.value.length : filteredRecent.value.length)
    function clearDirectorySearch() {
      directoryQuery.value = ''
      directoryInput.value?.focus()
    }
    const friendCandidates = computed(() => {
      const friendIDs = new Set(friends.value.map(friend => friend.id))
      const query = friendQuery.value.trim().toLowerCase()
      return users.value.filter(person => {
        if (friendIDs.has(person.id)) return false
        if (!query) return true
        return [person.name, person.email, person.id].some(value => value.toLowerCase().includes(query))
      })
    })
    const inviteCandidates = computed(() => {
      const memberIDs = new Set(current.value?.members.map(member => member.id) || [])
      return users.value.filter(person => !memberIDs.has(person.id))
    })
    // Search and the chosen initial narrow the list together; the groups feed the
    // A–Z index while the flat list feeds select-all.
    const inviteVisible = computed(() => filterCandidates(inviteCandidates.value, inviteQuery.value, inviteInitial.value))
    const inviteGroups = computed(() => groupByInitial(inviteVisible.value))
    const inviteInitials = computed(() => availableInitials(groupByInitial(inviteCandidates.value)))
    const inviteSummary = computed(() => selectionSummary(inviteUsers.value, current.value?.members.length || 0, 1000))
    const inviteSelectedCount = computed(() => inviteUsers.value.length)
    // The chips need the person objects rather than bare ids, and a selected user
    // filtered out of the list must still be removable, so the whole candidate set
    // is searched instead of only the visible rows.
    const inviteSelectedPeople = computed(() => {
      const byID = new Map(inviteCandidates.value.map(person => [person.id, person]))
      return inviteUsers.value.map(id => byID.get(id)).filter(Boolean)
    })
    function chooseInviteInitial(initial) {
      // Clicking the active letter clears the filter, so the rail doubles as a
      // toggle instead of requiring a separate reset control.
      inviteInitial.value = inviteInitial.value === initial ? '' : initial
    }
    function toggleInvite(person) {
      inviteUsers.value = toggleSelection(inviteUsers.value, person.id)
    }
    function selectAllInvites() {
      inviteUsers.value = selectAllCandidates(inviteUsers.value, inviteVisible.value, current.value?.members.length || 0, 1000)
    }
    function clearInvites() {
      inviteUsers.value = []
    }
    function resetInviteFilters() {
      inviteQuery.value = ''
      inviteInitial.value = ''
    }
    const currentMembers = computed(() => filterMembers(current.value?.members, memberQuery.value, me.value?.id))
    const memberCount = computed(() => current.value?.members.length || 0)
    const currentRole = computed(() => roleOf(current.value, me.value?.id))
    const isGroupOwner = computed(() => canChangeRole(current.value, me.value?.id))
    const canInviteMembers = computed(() => canInvite(current.value, me.value?.id))
    const canManageGroupMembers = computed(() => canManageMembers(current.value, me.value?.id))
    const canEditGroupAnnouncement = computed(() => canEditAnnouncement(current.value, me.value?.id))
    const canLeaveGroup = computed(() => canLeave(current.value))
    const memberCanBeRemoved = member => canRemoveMember(current.value, me.value?.id, member.id)
    const memberCanChangeRole = member => isGroupOwner.value && member.id !== me.value?.id && member.role !== 'owner'
    const canTransferTo = member => canTransferOwnership(current.value, me.value?.id) && member.id !== me.value?.id
    // The right-hand panel holds two independent blocks: the announcement block
    // on top and the member list below. Both are always visible for a group, so
    // they no longer live in modal dialogs.
    const groupAnnouncements = ref([])
    const announcementsBusy = ref(false)
    const announcementsError = ref('')
    const announcementPanelOpen = ref(false)
    const announcementEditorOpen = ref(false)
    const announcementEditingID = ref('')
    const announcementDraftText = ref('')
    const announcementSaving = ref(false)
    const announcementCount = computed(() => groupAnnouncements.value.length)
    // The panel block shows the newest notice as a teaser; opening it reveals all.
    const latestAnnouncement = computed(() => groupAnnouncements.value[0] || null)
    const canManageAnnouncements = computed(() => canEditGroupAnnouncement.value)
    async function loadAnnouncements() {
      const id = active.value
      const version = epoch
      if (!id || current.value?.kind !== 'group') {
        groupAnnouncements.value = []
        return
      }
      announcementsBusy.value = true
      announcementsError.value = ''
      try {
        const items = await api(`/conversations/${id}/announcements`)
        if (version !== epoch) return
        groupAnnouncements.value = items || []
      } catch (cause) {
        if (version === epoch) announcementsError.value = cause.message || '加载群公告失败'
      } finally { if (version === epoch) announcementsBusy.value = false }
    }
    function openAnnouncementPanel() {
      announcementPanelOpen.value = true
      announcementEditorOpen.value = false
      loadAnnouncements().catch(showError)
    }
    function closeAnnouncementPanel() {
      announcementPanelOpen.value = false
      announcementEditorOpen.value = false
      announcementEditingID.value = ''
      announcementDraftText.value = ''
    }
    function startCreateAnnouncement() {
      if (!canManageAnnouncements.value) return
      announcementEditingID.value = ''
      announcementDraftText.value = ''
      announcementEditorOpen.value = true
    }
    function startEditAnnouncement(item) {
      if (!canManageAnnouncements.value || !item) return
      announcementEditingID.value = item.id
      announcementDraftText.value = item.content
      announcementEditorOpen.value = true
    }
    function cancelAnnouncementEditor() {
      announcementEditorOpen.value = false
      announcementEditingID.value = ''
      announcementDraftText.value = ''
    }
    async function saveAnnouncementText() {
      const text = announcementDraftText.value.trim()
      const id = active.value
      if (!text || !id || announcementSaving.value) return
      const version = epoch
      const editing = announcementEditingID.value
      announcementSaving.value = true
      try {
        if (editing) {
          await api(`/conversations/${id}/announcements/${editing}`, { method: 'PATCH', body: JSON.stringify({ content: text }) })
        } else {
          await api(`/conversations/${id}/announcements`, { method: 'POST', body: JSON.stringify({ content: text }) })
        }
        if (version !== epoch) return
        cancelAnnouncementEditor()
        await loadAnnouncements()
        await refresh()
      } catch (cause) { if (version === epoch) showError(cause) }
      finally { if (version === epoch) announcementSaving.value = false }
    }
    async function deleteAnnouncement(item) {
      if (!canManageAnnouncements.value || !item || announcementSaving.value) return
      if (!confirm('删除后群成员将看不到这条公告，确认删除？')) return
      const id = active.value
      const version = epoch
      announcementSaving.value = true
      try {
        await api(`/conversations/${id}/announcements/${item.id}`, { method: 'DELETE' })
        if (version !== epoch) return
        await loadAnnouncements()
        await refresh()
      } catch (cause) { if (version === epoch) showError(cause) }
      finally { if (version === epoch) announcementSaving.value = false }
    }
    async function removeMember(member) {
      if (memberBusy.value) return
      if (!confirm(`确认将 ${member.name} 移出群聊？`)) return
      const id = active.value
      const version = epoch
      memberBusy.value = true
      try {
        await api(`/conversations/${id}/members`, { method: 'DELETE', body: JSON.stringify({ user_id: member.id }) })
        if (version !== epoch) return
        await refresh()
      } catch (cause) { if (version === epoch) showError(cause) }
      finally { if (version === epoch) memberBusy.value = false }
    }
    async function changeRole(member, role) {
      if (memberBusy.value) return
      const id = active.value
      const version = epoch
      memberBusy.value = true
      try {
        await api(`/conversations/${id}/roles`, { method: 'POST', body: JSON.stringify({ user_id: member.id, role }) })
        if (version !== epoch) return
        await refresh()
      } catch (cause) { if (version === epoch) showError(cause) }
      finally { if (version === epoch) memberBusy.value = false }
    }
    async function transferOwnership(member) {
      if (memberBusy.value) return
      if (!confirm(`确认把群主转让给 ${member.name}？你将变为管理员。`)) return
      const id = active.value
      const version = epoch
      memberBusy.value = true
      try {
        await api(`/conversations/${id}/owner`, { method: 'POST', body: JSON.stringify({ user_id: member.id }) })
        if (version !== epoch) return
        await refresh()
      } catch (cause) { if (version === epoch) showError(cause) }
      finally { if (version === epoch) memberBusy.value = false }
    }
    async function leaveGroup() {
      if (groupBusy.value || !current.value) return
      if (!confirm(`确认退出群聊“${title(current.value)}”？退出后将不再接收该群消息。`)) return
      const id = active.value
      const version = epoch
      groupBusy.value = true
      try {
        await api(`/conversations/${id}/members/leave`, { method: 'POST' })
        if (version !== epoch) return
        closeMembers()
        active.value = null
        chatVisible.value = false
        await refresh()
      } catch (cause) { if (version === epoch) showError(cause) }
      finally { if (version === epoch) groupBusy.value = false }
    }
    // Preferences are private to the caller, so only the local list is updated;
    // no refresh of other members' views is needed.
    async function setPreference(id, change) {
      if (preferenceBusy.value) return
      const conversation = conversations.value.find(item => item.id === id)
      if (!conversation) return
      const version = epoch
      preferenceBusy.value = true
      try {
        const updated = await api(`/conversations/${id}/preferences`, { method: 'PATCH', body: JSON.stringify(change) })
        if (version !== epoch) return
        conversation.pinned = updated.pinned
        conversation.muted = updated.muted
        cacheVersion.value++
      } catch (cause) { if (version === epoch) showError(cause) }
      finally { if (version === epoch) preferenceBusy.value = false }
    }
    const togglePreference = (id, field) => {
      const conversation = conversations.value.find(item => item.id === id)
      if (!conversation) return
      setPreference(id, { [field]: !conversation[field] })
    }
    const currentPinned = computed(() => Boolean(current.value?.pinned))
    const currentMuted = computed(() => Boolean(current.value?.muted))
    // Conversation titles prefer the caller's private friend remark; names
    // inside a conversation prefer the group card over the account name.
    const friendRemark = id => friends.value.find(friend => friend.id === id)?.remark || ''
    const memberOf = (conversation, id) => conversation?.members.find(member => member.id === id)
    const myMembership = computed(() => current.value?.members.find(member => member.id === me.value?.id))
    // Muted members and plain members of an all-muted group see a quiet composer.
    const mutedHere = computed(() => {
      if (!current.value || current.value.dismissed) return false
      if (current.value.all_muted && currentRole.value === 'member') return true
      const until = myMembership.value?.muted_until
      return Boolean(until && Date.parse(until) > Date.now())
    })
    const dismissedHere = computed(() => Boolean(current.value?.dismissed))
    const memberByID = id => current.value?.members.find(member => member.id === id) || null
    // 单聊信息面板展示对方资料；群聊沿用右侧成员面板。
    const headMember = computed(() => (current.value?.kind === 'direct' ? memberByID(current.value.members.find(member => member.id !== me.value?.id)?.id) : null))
    const chatInfoOpen = ref(false)
    watch([active, me], () => { chatInfoOpen.value = false })
    // One line of context for the conversation header: who else is here and
    // which governance modes are active.
    const chatHeadMeta = computed(() => {
      if (!current.value) return ''
      if (current.value.kind === 'direct') {
        const other = memberOf(current.value, current.value.members.find(member => member.id !== me.value?.id)?.id)
        if (!other) return ''
        const remark = friendRemark(other.id)
        const base = remark && remark !== other.name ? other.name + ' · ' + other.email : other.email
        return other.online ? base + ' · 在线' : base
      }
      const flags = [current.value.all_muted && '全员禁言中', current.value.join_approval && '入群需审批', current.value.dismissed && '已解散'].filter(Boolean)
      return current.value.members.length + ' 名成员' + (flags.length ? ' · ' + flags.join(' · ') : '')
    })
    // Read receipts: a direct conversation shows whether the other side has read
    // the message, derived from the read cursors the conversation list carries.
    const readTicks = item => {
      if (!item || item.kind === 'system' || item.recalled_at || current.value?.kind !== 'direct' || item.sender_id !== me.value?.id) return ''
      const other = current.value.members.find(member => member.id !== me.value?.id)
      if (!other) return ''
      return (other.last_read_seq || 0) >= item.seq ? '已读' : '已送达'
    }
    const typingLabel = computed(() => {
      typingTick.value
      const now = Date.now()
      const activeTypists = Object.entries(typingUsers.value[active.value] || {}).filter(([, expiry]) => expiry > now)
      const names = activeTypists.map(([id]) => (id === me.value?.id ? '' : sender(id))).filter(Boolean)
      if (!names.length) return ''
      return names.length === 1 ? names[0] : names.slice(0, 2).join('、') + (names.length > 2 ? ' 等' : '')
    })
    // The thread window is a surface separate from the chat list: it shows one
    // root message and every reply to it, and replies sent here join that window.
    const threadOpen = ref(false)
    const threadRoot = ref(null)
    const threadReplies = ref([])
    const threadHasMorePages = ref(false)
    const threadBusy = ref(false)
    const threadError = ref('')
    const threadReplyCount = ref(0)
    const threadStream = ref(null)
    function closeThread() {
      threadOpen.value = false
      threadRoot.value = null
      threadReplies.value = []
      threadError.value = ''
      threadHasMorePages.value = false
      threadReplyCount.value = 0
      // A draft typed in the window is not carried over to the next one.
      threadDraft.value = ''
    }
    async function openThread(item) {
      if (!item) return
      const id = active.value
      const rootID = threadRootID(item)
      const version = epoch
      threadOpen.value = true
      threadRoot.value = item
      threadReplies.value = []
      threadError.value = ''
      threadHasMorePages.value = false
      threadBusy.value = true
      try {
        const page = await api(`/conversations/${id}/messages/${rootID}/thread?limit=100`)
        if (version !== epoch) return
        // The server resolves a reply to its window root, so the header shows the
        // root even when the user opened the window from a reply.
        threadRoot.value = page.root
        threadReplies.value = page.replies || []
        threadReplyCount.value = page.reply_count || 0
        threadHasMorePages.value = threadHasMore(page)
        await nextTick()
        threadStream.value?.scrollTo({ top: threadStream.value.scrollHeight })
      } catch (cause) {
        if (version === epoch) threadError.value = cause.message || '打开对话窗口失败'
      } finally { if (version === epoch) threadBusy.value = false }
    }
    async function loadMoreThread() {
      if (threadBusy.value || !threadHasMorePages.value || !threadRoot.value) return
      const id = active.value
      const version = epoch
      const last = threadReplies.value.at(-1)
      threadBusy.value = true
      try {
        const page = await api(`/conversations/${id}/messages/${threadRoot.value.id}/thread?limit=100&after_seq=${last?.seq || 0}`)
        if (version !== epoch) return
        threadReplies.value = [...threadReplies.value, ...(page.replies || [])]
        threadHasMorePages.value = threadHasMore(page)
        threadReplyCount.value = page.reply_count || threadReplyCount.value
      } catch (cause) { if (version === epoch) threadError.value = cause.message || '加载失败' }
      finally { if (version === epoch) threadBusy.value = false }
    }
    // The window has its own composer: a reply written here must be sent into the
    // window without disturbing whatever the user had typed in the main chat.
    const threadDraft = ref('')
    const threadSending = ref(false)
    const threadDraftArea = ref(null)
    async function sendInThread() {
      const content = threadDraft.value.trim()
      const root = threadRoot.value
      const id = active.value
      const version = epoch
      if (!content || !root || !id || threadSending.value) return
      threadSending.value = true
      try {
        await api(`/conversations/${id}/messages`, {
          method: 'POST',
          body: JSON.stringify({ client_msg_id: crypto.randomUUID(), content, media_id: '', reply_to_id: root.id, mentions: [] }),
        })
        if (version !== epoch) return
        threadDraft.value = ''
        await refreshThread()
        await syncConversation(id)
      } catch (cause) { if (version === epoch) showError(cause) }
      finally { if (version === epoch) threadSending.value = false }
    }
    // Reloads the window after a change so the reply appears with its true count.
    async function refreshThread() {
      const root = threadRoot.value
      const id = active.value
      if (!root || !id) return
      const page = await api(`/conversations/${id}/messages/${root.id}/thread?limit=100`)
      threadRoot.value = page.root
      threadReplies.value = page.replies || []
      threadReplyCount.value = page.reply_count || 0
      threadHasMorePages.value = threadHasMore(page)
      await nextTick()
      threadStream.value?.scrollTo({ top: threadStream.value.scrollHeight })
    }
    function onThreadEnter(event) {
      if (event.isComposing) return
      event.preventDefault()
      sendInThread()
    }
    function onThreadEnter(event) {
      if (event.isComposing) return
      event.preventDefault()
      sendInThread()
    }
    function openMembers() {
      memberQuery.value = ''
      memberMenuID.value = ''
      showMembers.value = true
      if (canManageGroupMembers.value) loadJoinRequests().catch(() => {})
    }
    function closeMembers() {
      showMembers.value = false
      memberQuery.value = ''
    }
    function inviteFromMembers() {
      closeMembers()
      openInvite()
    }
    const title = item => {
      if (item.kind !== 'direct') return item.name
      const otherID = item.members.find(member => member.id !== me.value?.id)?.id
      return (otherID && friendRemark(otherID)) || memberOf(item, otherID)?.name || '私聊'
    }
    const sender = id => {
      const member = memberOf(current.value, id)
      return member ? member.alias || member.name || '成员' : '成员'
    }
    // Global search hits may belong to any of the caller's conversations, so the
    // name is resolved across every known member list rather than the active one.
    const anySender = id => {
      for (const conversation of conversations.value) {
        const member = conversation.members?.find(person => person.id === id)
        if (member) return member.name
      }
      return me.value?.id === id ? me.value?.name : '成员'
    }
    const time = value => new Date(value).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
    const preview = item => {
      const message = latestMessage(item)
      if (!message) return '暂无消息'
      const label = mediaPreview(message)
      return label ? (message.content.trim() ? `${label} ${message.content}` : label) : message.content
    }
    const contactPreview = person => { const conversation = directConversation(person.id); return conversation ? preview(conversation) : '暂无消息' }
    const unreadLabel = id => { const count = unreadCounts.value[id] || 0; return count > 99 ? '99+' : String(count) }
    const hasUnread = id => (unreadCounts.value[id] || 0) > 0
    // A muted conversation keeps its unread count but shows no badge, so the
    // user is not pushed by a conversation they deliberately silenced.
    const showsBadge = item => showsUnreadBadge(item, unreadCounts.value[item.id] || 0)
    const seenKey = id => `im.seen.${me.value?.id}.${id}`
    const seenSeq = id => Number(saved.getItem(seenKey(id)) || 0)
    const markSeen = (id, seq) => {
      if (!me.value || !seq) return
      const previous = seenSeq(id)
      if (seq > previous) saved.setItem(seenKey(id), String(seq))
      if (unreadCounts.value[id]) unreadCounts.value = { ...unreadCounts.value, [id]: 0 }
    }
    const addUnread = (id, count) => {
      if (id === active.value || count < 1) return
      unreadCounts.value = { ...unreadCounts.value, [id]: (unreadCounts.value[id] || 0) + count }
    }
    const scroll = async () => { await nextTick(); if (stream.value) stream.value.scrollTop = stream.value.scrollHeight }
    const showError = cause => { error.value = cause?.message || '操作失败'; clearTimeout(errorTimer); errorTimer = setTimeout(() => { error.value = '' }, 5000) }
    const atBottom = () => !stream.value || stream.value.scrollHeight - stream.value.scrollTop - stream.value.clientHeight < 64
    const showMessages = id => { if (id === active.value) messages.value = (cache.get(id) || []).slice(-visibleCount.value) }
    function discardPending() {
      if (!pendingSend.value || !confirm('放弃确认不等于撤回：消息可能已经保存在服务器。请先检查聊天记录，确定仍要放弃吗？')) return
      pendingStore.clear(pendingSend.value.conversation_id, pendingSend.value.client_msg_id)
      pendingSend.value = null
    }
    function openMediaPicker() {
      if (!active.value || uploadBusy.value || sending.value) return
      mediaInput.value?.click()
    }
    function releaseAttachment() {
      if (previewFile.value) URL.revokeObjectURL(previewFile.value)
      previewFile.value = null
      attachment.value = null
    }
    function clearAttachment() {
      releaseAttachment()
      if (mediaInput.value) mediaInput.value.value = ''
    }
    // The file is uploaded immediately so the message send only has to reference
    // the stored attachment; a failed upload leaves the draft untouched.
    async function chooseMedia(event) {
      const file = event.target.files?.[0]
      if (mediaInput.value) mediaInput.value.value = ''
      if (!file || !active.value) return
      const check = validateMediaFile(file)
      if (!check.ok) { showError({ message: check.error }); return }
      if (pendingSend.value?.conversation_id === active.value) { showError({ message: '上一条发送结果未确认，请先恢复或放弃' }); return }
      const id = active.value
      const version = epoch
      releaseAttachment()
      uploadBusy.value = true
      try {
        const uploaded = await uploadMedia(id, file)
        if (version !== epoch || active.value !== id) return
        attachment.value = uploaded
        previewFile.value = URL.createObjectURL(file)
      } catch (cause) { if (version === epoch) showError(cause) }
      finally { if (version === epoch) uploadBusy.value = false }
    }
    // True while a file is being dragged over the chat, used to show the drop hint.
    const dragging = ref(false)
    // Re-encodes an image the server would not treat as a photo (bmp, avif, heic)
    // into PNG. Returns null when the browser cannot decode it, in which case the
    // original bytes are sent and the message falls back to a file.
    async function convertImageToPNG(file) {
      try {
        const bitmap = await createImageBitmap(file)
        const canvas = document.createElement('canvas')
        canvas.width = bitmap.width
        canvas.height = bitmap.height
        const context = canvas.getContext('2d')
        if (!context) return null
        context.drawImage(bitmap, 0, 0)
        bitmap.close?.()
        return await new Promise(resolve => canvas.toBlob(resolve, 'image/png'))
      } catch { return null }
    }
    // Uploads images and inserts each one as a real picture at the caret, so several
    // images can be interleaved with text in the order they are added and the draft
    // reads exactly as the message will. Nothing is sent here.
    async function acceptImages(files, placeholders = []) {
      const id = active.value
      if (!id || !files.length) return
      if (pendingSend.value?.conversation_id === id) {
        for (const placeholder of placeholders) placeholder.remove()
        if (placeholders.length) syncDraftFromDom()
        showError({ message: '上一条发送结果未确认，请先恢复或放弃' })
        return
      }
      const version = epoch
      uploadBusy.value = true
      try {
        for (const [fileIndex, candidate] of files.entries()) {
          // One message holds at most 20 segments on the server, and each image is
          // one segment, so the composer stops at that many images.
          if (composerImages.value.length >= MAX_COMPOSER_IMAGES) {
            showError({ message: `一条消息最多附带 ${MAX_COMPOSER_IMAGES} 张图片` })
            break
          }
          const prepared = await prepareImage(candidate, convertImageToPNG)
          const check = validateMediaFile(prepared)
          if (!check.ok) { placeholders[fileIndex]?.remove(); showError({ message: check.error }); continue }
          const uploaded = await uploadMedia(id, prepared)
          if (version !== epoch || active.value !== id) return
          const index = nextImageIndex(composerImages.value)
          const image = {
            index,
            mediaId: uploaded.id,
            previewURL: URL.createObjectURL(prepared),
            fileName: uploaded.file_name || prepared.name,
          }
          composerImages.value = [...composerImages.value, image]
          // The picture goes where the caret was, so it reads at the point the user
          // pasted it.
          const root = draftArea.value
          if (root) {
            const placeholder = placeholders[fileIndex]
            const range = placeholder?.isConnected ? document.createRange() : caretRange(root, window.getSelection())
            if (placeholder?.isConnected) range.selectNode(placeholder)
            const chip = insertImageAtCaret(root, range, image)
            placeCaretAfter(root, chip, window.getSelection())
            root.focus()
            syncDraftFromDom()
          }
        }
      } catch (cause) { if (version === epoch) showError(cause) }
      finally {
        if (version === epoch) {
          for (const placeholder of placeholders) placeholder.remove()
          uploadBusy.value = false
          if (active.value === id) syncDraftFromDom()
        }
      }
    }
    // Removing an image drops its picture from the draft as well as its entry.
    function removeComposerImage(image) {
      if (!image) return
      if (draftArea.value) removeImageFromComposer(draftArea.value, image.mediaId)
      if (image.previewURL) URL.revokeObjectURL(image.previewURL)
      composerImages.value = composerImages.value.filter(entry => entry.mediaId !== image.mediaId)
      syncDraftFromDom()
    }
    let undoRoot = null
    let undoHistory = []
    let undoPosition = -1
    let composerComposing = false
    function composerSnapshot() {
      const root = draftArea.value
      const selection = window.getSelection()
      const offsets = node => {
        const path = []
        while (node && node !== root) {
          path.unshift(Array.prototype.indexOf.call(node.parentNode.childNodes, node))
          node = node.parentNode
        }
        return path
      }
      const range = selection?.rangeCount ? selection.getRangeAt(0) : null
      const caret = range && root.contains(range.startContainer) && root.contains(range.endContainer)
        ? { start: offsets(range.startContainer), startOffset: range.startOffset, end: offsets(range.endContainer), endOffset: range.endOffset } : null
      return { html: root.innerHTML, images: composerImages.value.map(image => ({ ...image })), caret }
    }
    function resetComposerUndo() {
      undoHistory = draftArea.value ? [composerSnapshot()] : []
      undoPosition = undoHistory.length - 1
      composerComposing = false
    }
    function recordComposerUndo() {
      const root = draftArea.value
      if (!root || composerComposing || uploadBusy.value || root.querySelector('[data-rich-paste-image]')) return
      const snapshot = composerSnapshot()
      if (undoHistory[undoPosition]?.html === snapshot.html) return
      undoHistory.splice(undoPosition + 1)
      undoHistory.push(snapshot)
      if (undoHistory.length > 100) undoHistory.shift()
      undoPosition = undoHistory.length - 1
    }
    function onComposerUndoKey(event) {
      if (!(event.ctrlKey || event.metaKey) || event.altKey || event.isComposing || event.key.toLowerCase() !== 'z') return
      event.preventDefault()
      const root = draftArea.value
      if (composerComposing || uploadBusy.value || root.querySelector('[data-rich-paste-image]')) return
      const position = undoPosition + (event.shiftKey ? 1 : -1)
      const snapshot = undoHistory[position]
      if (!snapshot) return
      undoPosition = position
      for (const image of composerImages.value) if (image.previewURL?.startsWith('blob:')) URL.revokeObjectURL(image.previewURL)
      composerImages.value = snapshot.images.map(image => ({ ...image, previewURL: mediaURL(image.mediaId) }))
      root.innerHTML = snapshot.html
      for (const image of composerImages.value) {
        const chip = [...root.querySelectorAll('[data-media-id]')].find(node => node.getAttribute('data-media-id') === image.mediaId)
        if (chip?.querySelector('img')) chip.querySelector('img').src = image.previewURL
      }
      snapshot.html = root.innerHTML
      draft.value = piecesToText(collectPieces(root))
      root.focus()
      const range = document.createRange()
      const caret = snapshot.caret
      const resolve = path => path.reduce((node, index) => node?.childNodes[index], root)
      if (caret && resolve(caret.start) && resolve(caret.end)) {
        range.setStart(resolve(caret.start), caret.startOffset)
        range.setEnd(resolve(caret.end), caret.endOffset)
      } else { range.selectNodeContents(root); range.collapse(false) }
      const selection = window.getSelection()
      selection.removeAllRanges()
      selection.addRange(range)
      updateMentionState()
    }
    function installComposerUndo(root) {
      if (undoRoot === root) return
      undoRoot = root
      resetComposerUndo()
      root.addEventListener('keydown', onComposerUndoKey)
      root.addEventListener('compositionstart', () => { composerComposing = true })
      root.addEventListener('compositionend', () => { composerComposing = false; recordComposerUndo() })
      root.addEventListener('beforeinput', () => {
        if (undoHistory[undoPosition]?.html === root.innerHTML) undoHistory[undoPosition].caret = composerSnapshot().caret
      })
    }
    function clearComposerImages() {
      for (const image of composerImages.value) if (image.previewURL) URL.revokeObjectURL(image.previewURL)
      composerImages.value = []
      resetComposerUndo()
    }
    // The rich region is the source of truth while editing, so the draft string the
    // rest of the component reads is derived from it after every change.
    function syncDraftFromDom() {
      const root = draftArea.value
      if (!root) return
      draft.value = piecesToText(collectPieces(root))
      recordComposerUndo()
    }
    function composerBlocks() {
      const root = draftArea.value
      return root ? piecesToBlocks(collectPieces(root)) : []
    }
    function composerHasContent() {
      const root = draftArea.value
      return root ? hasContent(collectPieces(root)) : false
    }
    // Keep uploads owned by the component in step with edits the browser performs
    // itself, such as cutting a selected picture or deleting a selected range.
    function syncComposerImagesWithDom() {
      const ids = new Set(imageIDsIn(draftArea.value))
      const removed = composerImages.value.filter(image => !ids.has(image.mediaId))
      if (!removed.length) return
      for (const image of removed) if (image.previewURL) URL.revokeObjectURL(image.previewURL)
      composerImages.value = composerImages.value.filter(image => ids.has(image.mediaId))
    }
    function onComposerInput() {
      syncComposerImagesWithDom()
      syncDraftFromDom()
      updateMentionState()
      notifyTyping()
    }
    function onComposerKeydown(event) {
      if (event.isComposing || event.altKey || event.ctrlKey || event.metaKey) return
      if (event.key !== 'Backspace' && event.key !== 'Delete') return
      const mediaId = deleteImageNearCaret(draftArea.value, window.getSelection(), event.key === 'Delete' ? 'forward' : 'backward')
      if (!mediaId) return
      event.preventDefault()
      const image = composerImages.value.find(entry => entry.mediaId === mediaId)
      if (image?.previewURL) URL.revokeObjectURL(image.previewURL)
      composerImages.value = composerImages.value.filter(entry => entry.mediaId !== mediaId)
      syncDraftFromDom()
      updateMentionState()
    }
    // A click on the remove control deletes that image without selecting the chip.
    function onComposerClick(event) {
      const remove = event.target?.closest?.('[data-remove-media]')
      if (remove) {
        event.preventDefault()
        const mediaId = remove.getAttribute('data-remove-media')
        const image = composerImages.value.find(entry => entry.mediaId === mediaId)
        if (image) removeComposerImage(image)
        return
      }
      updateMentionState()
    }
    function onComposerBlur() {
      // The mention popup is dismissed when focus leaves the composer, but choosing an
      // entry cancels the blur first.
      mentionQuery.value = null
      mentionList.value = []
    }
    function pasteRichHTML(html, clipboardImages = []) {
      const source = new DOMParser().parseFromString(html, 'text/html')
      const fragment = document.createDocumentFragment()
      const images = []
      const addBreak = () => {
        if (fragment.lastChild?.nodeName !== 'BR') fragment.append(document.createElement('br'))
      }
      const walk = node => {
        if (node.nodeType === Node.TEXT_NODE) {
          if (!node.nodeValue.trim() && /[\r\n]/.test(node.nodeValue)) return
          fragment.append(document.createTextNode(node.nodeValue || ''))
          return
        }
        if (node.nodeType !== Node.ELEMENT_NODE || node.hasAttribute('data-composer-chrome') || ['SCRIPT', 'STYLE'].includes(node.tagName)) return
        if (node.tagName === 'BR') { addBreak(); return }
        if (node.tagName === 'IMG') {
          const src = node.getAttribute('src') || ''
          const match = /^data:(image\/[a-z0-9.+-]+);base64,(.*)$/is.exec(src)
          const name = (node.getAttribute('alt') || 'pasted.png').replace(/[\\/:*?"<>|]/g, '_').slice(0, 128) || 'pasted.png'
          const placeholder = document.createElement('span')
          placeholder.setAttribute('contenteditable', 'false')
          placeholder.setAttribute('data-composer-chrome', '')
          placeholder.setAttribute('data-rich-paste-image', String(images.length))
          placeholder.textContent = '图片加载中…'
          let file = null
          if (match && match[2].length <= 14 * 1024 * 1024) {
            try {
              const bytes = Uint8Array.from(atob(match[2].replace(/\s/g, '')), character => character.charCodeAt(0))
              file = new File([bytes], name, { type: match[1] })
            } catch {}
          }
          images.push({ src, file: file || clipboardImages[images.length] || null, name, placeholder })
          fragment.append(placeholder)
          return
        }
        const block = ['DIV', 'P', 'LI', 'BLOCKQUOTE'].includes(node.tagName)
        if (block && fragment.childNodes.length) addBreak()
        for (const child of node.childNodes) walk(child)
        let nextSibling = node.nextSibling
        while (nextSibling?.nodeType === Node.TEXT_NODE && !nextSibling.nodeValue.trim() && /[\r\n]/.test(nextSibling.nodeValue)) nextSibling = nextSibling.nextSibling
        if (block && nextSibling && nextSibling.nodeName !== 'BR') addBreak()
      }
      for (const child of source.body.childNodes) walk(child)
      if (!images.length) {
        for (const file of clipboardImages) {
          const placeholder = document.createElement('span')
          placeholder.setAttribute('contenteditable', 'false')
          placeholder.setAttribute('data-composer-chrome', '')
          placeholder.setAttribute('data-rich-paste-image', String(images.length))
          placeholder.textContent = '图片加载中…'
          images.push({ file, name: file.name || 'pasted.png', placeholder })
          fragment.append(placeholder)
        }
      }
      const root = draftArea.value
      if (!root || !fragment.childNodes.length) return false
      const range = caretRange(root, window.getSelection())
      range.deleteContents()
      const last = fragment.lastChild
      range.insertNode(fragment)
      if (last) {
        range.setStartAfter(last)
        range.collapse(true)
        const selection = window.getSelection()
        selection.removeAllRanges()
        selection.addRange(range)
      }
      root.focus()
      root.dispatchEvent(new window.Event('input', { bubbles: true }))
      if (images.length) {
        Promise.all(images.map(async image => {
          if (image.file) return image.file
          try {
            const url = new URL(image.src, location.href)
            if (url.origin !== location.origin || !(url.pathname.startsWith('/api/v1/media/') || url.protocol === 'blob:')) return null
            const response = await fetch(url.href, { credentials: 'same-origin' })
            if (!response.ok) return null
            const blob = await response.blob()
            if (!blob.type.startsWith('image/')) return null
            return new File([blob], image.name, { type: blob.type })
          } catch { return null }
        })).then(files => {
          const readyFiles = []
          const placeholders = []
          files.forEach((file, index) => {
            if (file) { readyFiles.push(file); placeholders.push(images[index].placeholder) }
            else images[index].placeholder.remove()
          })
          if (readyFiles.length) acceptImages(readyFiles, placeholders).catch(showError)
        }).catch(showError)
      }
      return true
    }
    // Pasting an image keeps it out of the text box and holds it for sending.
    function onComposerPaste(event) {
      const html = event.clipboardData?.getData?.('text/html') || ''
      const images = collectImages(event.clipboardData)
      if (html && pasteRichHTML(html, images)) {
        event.preventDefault()
        return
      }
      if (images.length) {
        event.preventDefault()
        acceptImages(images).catch(showError)
      }
    }
    function onDragOver(event) {
      if (!active.value || !hasTransferableFiles(event.dataTransfer)) return
      event.preventDefault()
      if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
      dragging.value = true
    }
    function onDragLeave(event) {
      // Ignore the leave events produced while moving between child elements.
      if (event.currentTarget?.contains?.(event.relatedTarget)) return
      dragging.value = false
    }
    function onDrop(event) {
      dragging.value = false
      const images = collectImages(event.dataTransfer)
      if (!images.length || !active.value) return
      event.preventDefault()
      acceptImages(images).catch(showError)
    }
    // Reply, mention and recall helpers.
    const replyPreview = item => replyExcerpt(item)
    const replyAuthor = item => replyLabel(item, sender)
    const bodyOf = item => messageBody(item)
    // A message with ordered blocks renders segment by segment; the older shapes keep
    // their single media plus caption.
    const segmentsOf = item => messageSegments(item)
    const usesBlocks = item => hasBlocks(item)
    const recalledText = item => recalledLabel(item, me.value?.id)
    const canRecallMessage = item => canRecall(item, me.value?.id, current.value)
    // Mention list is derived from the active "@" token in the composer.
    const showMentions = computed(() => mentionQuery.value !== null && current.value?.kind === 'group')
    // Clicking "reply" opens the conversation window for that message, which is
    // where the user reads every reply and writes their own. Replies never travel
    // through the main composer, so the two surfaces stay distinct.
    function startReply(item) {
      if (!item) return
      openThread(item)
    }
    function updateMentionState() {
      const area = draftArea.value
      if (!area || current.value?.kind !== 'group') { mentionQuery.value = null; mentionList.value = []; return }
      // A contenteditable region has no selectionStart, so the caret is measured as
      // the text before it inside the region.
      const before = textBeforeCaret(area, window.getSelection())
      const token = activeMentionToken(before, before.length)
      if (!token) { mentionQuery.value = null; mentionList.value = []; return }
      mentionQuery.value = token
      mentionList.value = mentionCandidates(current.value?.members, me.value?.id, token.query).slice(0, 8)
      mentionIndex.value = 0
    }
    function chooseMention(person) {
      const token = mentionQuery.value
      if (!token || !person) return
      const applied = applyMention(draft.value, token, person.name)
      mentionQuery.value = null
      mentionList.value = []
      // Rewriting the region from the applied text keeps the mention edit simple; the
      // caret is then placed where the mention ended.
      const area = draftArea.value
      if (!area) return
      area.textContent = applied.text
      draft.value = applied.text
      recordComposerUndo()
      nextTick(() => {
        const root = draftArea.value
        if (!root) return
        root.focus()
        const range = document.createRange()
        const target = root.firstChild
        if (target && target.nodeType === 3) {
          const offset = Math.min(applied.caret, target.length)
          range.setStart(target, offset)
        } else {
          range.selectNodeContents(root)
          range.collapse(false)
        }
        range.collapse(true)
        const selection = window.getSelection()
        selection.removeAllRanges()
        selection.addRange(range)
      })
    }
    function mentionKeydown(event) {
      if (!showMentions.value || !mentionList.value.length) return false
      if (event.key === 'ArrowDown') { mentionIndex.value = (mentionIndex.value + 1) % mentionList.value.length; return true }
      if (event.key === 'ArrowUp') { mentionIndex.value = (mentionIndex.value - 1 + mentionList.value.length) % mentionList.value.length; return true }
      if (event.key === 'Enter' || event.key === 'Tab') { chooseMention(mentionList.value[mentionIndex.value]); return true }
      if (event.key === 'Escape') { mentionQuery.value = null; mentionList.value = []; return true }
      return false
    }
    // Mentioned user IDs are resolved from the final text so a name edited after
    // insertion does not silently drop the notification.
    function resolveMentions() {
      if (current.value?.kind !== 'group') return []
      const text = draft.value
      const found = []
      for (const member of current.value?.members || []) {
        if (member.id === me.value?.id) continue
        if (text.includes(`@${member.name}`)) found.push(member.id)
      }
      return found
    }
    async function recallMessage(item) {
      if (!canRecallMessage(item) || sending.value) return
      if (!confirm('撤回后消息内容将不可见，其他成员会看到撤回提示。确认撤回？')) return
      const id = active.value
      const version = epoch
      sending.value = true
      try {
        await api(`/conversations/${id}/messages/${item.seq}/recall`, { method: 'POST' })
        if (version !== epoch) return
        await syncConversation(id)
        if (threadOpen.value && active.value === id) await refreshThread()
      } catch (cause) { if (version === epoch) showError(cause) }
      finally { if (version === epoch) sending.value = false }
    }
    const messageActions = useMessageActions({ messages, threadRoot, threadReplies, active, me, current, conversations, startReply, recallMessage, syncConversation, showError })
    const isGenericFile = computed(() => attachment.value?.media_type === 'file')
    function resetGlobalSearch() {
      globalResults.value = []
      globalCursor.value = { time: '', id: '' }
      globalHasMore.value = false
      globalStarted.value = false
      globalError.value = ''
    }
    function closeGlobalSearch() {
      resetGlobalSearch()
      globalQuery.value = ''
      showGlobalSearch.value = false
    }
    async function openGlobalSearch() {
      showGlobalSearch.value = true
      await nextTick()
      globalInput.value?.focus()
    }
    // Global search spans every conversation the caller belongs to, so each hit
    // carries its own conversation context instead of using the active one.
    async function searchAll(more = false) {
      const query = globalQuery.value.trim()
      if (!query || (more && (globalBusy.value || !globalHasMore.value))) return
      if ([...query].length > 100) { globalError.value = '关键词最多 100 个字符'; return }
      globalBusy.value = true
      globalError.value = ''
      globalStarted.value = true
      try {
        const params = new URLSearchParams({ q: query, limit: '20' })
        if (more && globalCursor.value.time && globalCursor.value.id) {
          params.set('before_time', globalCursor.value.time)
          params.set('before_id', globalCursor.value.id)
        }
        const page = await api(`/messages/search?${params}`)
        globalResults.value = more ? [...globalResults.value, ...page.results] : page.results
        globalHasMore.value = page.has_more
        globalCursor.value = { time: page.next_before_time, id: page.next_before_id }
      } catch (cause) { globalError.value = cause.message || '搜索失败，请重试' }
      finally { globalBusy.value = false }
    }
    async function openGlobalResult(hit) {
      const id = hit.message.conversation_id
      const version = epoch
      await select(id)
      if (version !== epoch) return
      const items = cache.get(id) || []
      const index = items.findIndex(message => message.id === hit.message.id)
      if (index < 0) {
        // Older than the loaded window: locate it the same way a conversation
        // search result is located.
        await locateMessage({ ...hit.message, conversation_id: id })
        closeGlobalSearch()
        return
      }
      visibleCount.value = Math.max(visibleCount.value, items.length - index + 10)
      showMessages(id)
      closeGlobalSearch()
      highlightedMessage.value = hit.message.seq
      await nextTick()
      stream.value?.querySelector(`[data-message-seq="${hit.message.seq}"]`)?.scrollIntoView({ block: 'center' })
      clearTimeout(highlightTimer)
      highlightTimer = setTimeout(() => { highlightedMessage.value = null }, 4000)
    }
    async function refresh() {
      if (refreshTask) return refreshTask
      const version = epoch
      const task = (async () => {
        const [list, people, friendList, requestList] = await Promise.all([api('/conversations'), api('/users'), api('/friends'), api('/friends/requests')])
        if (version !== epoch || !me.value) return
        conversations.value = list
        users.value = people
        friends.value = friendList
        friendRequests.value = requestList
        if (!active.value && list.length) active.value = list[0].id
        if (active.value && !list.some(item => item.id === active.value)) active.value = null
        for (const item of list) {
          if (!me.value || version !== epoch) return
          await syncConversation(item.id)
        }
      })()
      refreshTask = task
      try { await task } finally { if (refreshTask === task) refreshTask = null }
    }
    function syncConversation(id, lastSeq = 0) {
      const version = epoch
      return syncing.run(id, lastSeq, async () => {
        let hasMore = true
        while (hasMore && me.value && version === epoch) {
          const existing = cache.get(id) || []
          const after = existing.at(-1)?.seq || 0
           const page = await api(`/conversations/${id}/sync?after_seq=${after}&limit=100`)
           if (version !== epoch || !me.value) return
           const seen = seenSeq(id)
           const incoming = page.messages.filter(message => message.seq > seen && message.sender_id !== me.value.id).length
           if (id === active.value) markSeen(id, page.next_seq)
           else addUnread(id, incoming)
           const stick = atBottom()
          const merged = appendPage(existing, page, id)
          cache.set(id, merged)
          cacheVersion.value++
          const pending = pendingStore?.get(id)
          if (pending && page.messages.some(message => message.sender_id === me.value.id && message.client_msg_id === pending.client_msg_id)) {
            pendingStore.clear(id, pending.client_msg_id)
            if (pendingSend.value?.client_msg_id === pending.client_msg_id) pendingSend.value = null
            if (active.value === id && draft.value.trim() === pending.content) draft.value = ''
          }
          showMessages(id)
          const conversation = conversations.value.find(item => item.id === id)
          if (conversation) conversation.last_seq = Math.max(conversation.last_seq, page.last_seq)
           if (page.next_seq > 0) await api(`/conversations/${id}/received`, { method: 'POST', body: JSON.stringify({ device_id: deviceID, seq: page.next_seq }) })
           if (id === active.value && stick && !showSearch.value) await scroll()
          hasMore = page.has_more
        }
        if (version === epoch && me.value) return cache.get(id)?.at(-1)?.seq || 0
      })
    }
    async function catchUp(cursors) {
      const version = epoch
      if (cursors.some(cursor => !conversations.value.some(item => item.id === cursor.conversation_id))) await refresh()
      for (const cursor of cursors) {
        if (version !== epoch || !me.value) return
        const conversation = conversations.value.find(item => item.id === cursor.conversation_id)
        if (conversation) conversation.last_seq = Math.max(conversation.last_seq, cursor.last_seq)
        const localSeq = cache.get(cursor.conversation_id)?.at(-1)?.seq || 0
        if (cursor.last_seq > localSeq || cursor.received_seq < localSeq) await syncConversation(cursor.conversation_id, cursor.last_seq)
      }
    }
    async function reconcile() {
      const version = epoch
      const cursors = await api(`/devices/${deviceID}/cursors`)
      if (version === epoch && me.value) await catchUp(cursors)
    }
    const searchDate = value => new Date(value).toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
    function resetSearch() {
      searchVersion++
      locateVersion++
      searchController?.abort()
      searchBusy.value = false
      locatingMessage.value = null
      searchResults.value = []
      searchedQuery.value = ''
      searchHasMore.value = false
      searchBefore.value = 0
      searchStarted.value = false
      searchError.value = ''
    }
    function closeSearch() {
      resetSearch()
      showSearch.value = false
      searchQuery.value = ''
    }
    async function openSearch() {
      showSearch.value = true
      await nextTick()
      searchInput.value?.focus()
    }
    watch([active, me], () => {
      messageActions.cancelSelection()
      messageActions.closeMenu()
      messageActions.closeForward()
      sidebarMoreOpen.value = false
      chatMoreOpen.value = false
      clearAttachment()
      // Switching conversation closes the window; its replies belong to the old one.
      closeThread()
      mentionQuery.value = null
      mentionList.value = []
      closeSearch()
      highlightedMessage.value = null
      clearTimeout(highlightTimer)
    }, { flush: 'sync' })
    function changeSearch() {
      resetSearch()
    }
    async function searchMessages(more = false) {
      const query = more ? searchedQuery.value : searchQuery.value.trim()
      if (!active.value || !query || (more && (searchBusy.value || !searchHasMore.value))) return
      if (!more) resetSearch()
      if ([...query].length > 100) { searchError.value = '关键词最多 100 个字符'; return }
      const id = active.value
      const version = ++searchVersion
      searchController?.abort()
      const controller = new AbortController()
      searchController = controller
      searchBusy.value = true
      searchStarted.value = true
      searchError.value = ''
      searchedQuery.value = query
      const params = new URLSearchParams({ q: query, limit: '30' })
      if (more) params.set('before_seq', String(searchBefore.value))
      try {
        const page = await api(`/conversations/${id}/messages/search?${params}`, { signal: controller.signal })
        if (version !== searchVersion || id !== active.value) return
        searchResults.value = more ? [...searchResults.value, ...page.messages] : page.messages
        searchBefore.value = page.next_before_seq
        searchHasMore.value = page.has_more
      } catch (cause) {
        if (version === searchVersion && !controller.signal.aborted) searchError.value = '搜索失败，请重试'
      } finally {
        if (version === searchVersion) searchBusy.value = false
      }
    }
    async function locateMessage(item) {
      if (locatingMessage.value !== null) return
      const id = active.value
      const version = ++locateVersion
      locatingMessage.value = item.seq
      try {
        if (!(cache.get(id) || []).some(message => message.id === item.id)) await syncConversation(id, item.seq)
        if (version !== locateVersion || id !== active.value) return
        const items = cache.get(id) || []
        const index = items.findIndex(message => message.id === item.id)
        if (index < 0) throw new Error('暂时无法定位该消息，请稍后重试')
        visibleCount.value = Math.max(visibleCount.value, items.length - index + 10)
        showMessages(id)
        closeSearch()
        highlightedMessage.value = item.seq
        await nextTick()
        stream.value?.querySelector(`[data-message-seq="${item.seq}"]`)?.scrollIntoView({ block: 'center' })
        clearTimeout(highlightTimer)
        highlightTimer = setTimeout(() => { highlightedMessage.value = null }, 4000)
      } catch (cause) {
        if (version === locateVersion) searchError.value = cause.message || '定位失败，请重试'
      } finally {
        if (version === locateVersion) locatingMessage.value = null
      }
    }
    async function select(id) {
      const selection = toggleConversationSelection(active.value, chatVisible.value, id)
      active.value = selection.activeID
      chatVisible.value = selection.chatVisible
      if (!selection.chatVisible) {
        closeSearch()
        return
      }
      await nextTick()
      if (draftArea.value) installEmojiPicker(draftArea.value)
      markSeen(id, cache.get(id)?.at(-1)?.seq || 0)
      pendingSend.value = pendingStore?.get(id) || null
      visibleCount.value = 100
      showMessages(id)
      try {
        await syncConversation(id)
        // The right-hand panel shows the group's announcements, so they are
        // loaded when a conversation is opened rather than on every refresh.
        await loadAnnouncements()
        if (active.value === id && chatVisible.value) await scroll()
      } catch (cause) { showError(cause) }
    }
    const onVisible = () => { if (me.value) reconcile().catch(showError) }
    function connect() {
      if (disposed || !me.value) return
      const connection = createSocket(deviceID)
      socket = connection
      const channel = new Requests(connection)
      requests = channel
      connection.onmessage = async event => {
        if (socket !== connection || !me.value) return
        try {
          const envelope = JSON.parse(event.data)
          if (channel.receive(envelope)) return
          if (envelope.event === 'connection.ready') { connected.value = true; await refresh() }
          if (envelope.event === 'message.created') {
            if (!conversations.value.some(item => item.id === envelope.data.conversation_id)) await refresh()
            await syncConversation(envelope.data.conversation_id, envelope.data.seq)
          }
          if (envelope.event === 'message.recalled') await syncConversation(envelope.data.conversation_id, envelope.data.seq)
          if (envelope.event === 'message.updated') applyMessageUpdate(envelope.data)
          if (envelope.event === 'typing') noteTyping(envelope.data)
          if (envelope.event === 'message.sync_required') await catchUp(envelope.data.cursors)
          if (envelope.event === 'conversation.created') await refresh()
          if (envelope.event === 'conversation.updated') await refresh()
          if (envelope.event === 'error') throw new Error(envelope.error.message)
        } catch (cause) { showError(cause) }
      }
      connection.onclose = () => {
        channel.close()
        if (socket !== connection) return
        connected.value = false
        if (!disposed && me.value) reconnectTimer = setTimeout(connect, 3000)
      }
      connection.onerror = () => connection.close()
    }
    async function authenticate() {
      if (loading.value) return
      loading.value = true
      try {
        const path = mode.value === 'login' ? '/auth/login' : '/auth/register'
        me.value = await api(path, { method: 'POST', body: JSON.stringify({ email: email.value, password: password.value, name: name.value }) })
        epoch++
        pendingStore = new PendingSends(session, me.value.id)
        password.value = ''
        connect()
        await refresh()
        if (active.value) await select(active.value)
      } catch (cause) { showError(cause) }
      finally { loading.value = false }
    }
    async function logout() {
      pendingStore?.forget()
      clearAttachment()
      epoch++; me.value = null; connected.value = false
      clearTimeout(reconnectTimer); requests?.close(); socket?.close(); socket = null
      cache.clear(); syncing.clear(); refreshTask = null; pendingStore = null; pendingSend.value = null; unreadCounts.value = {}
      active.value = null; messages.value = []; conversations.value = []; users.value = []; friends.value = []; draft.value = ''
      showCreate.value = false; showAddUser.value = false; showInvite.value = false; showSettings.value = false; showProfile.value = false; showFriendRequests.value = false; showRequestComposer.value = false; showContacts.value = false; chatInfoOpen.value = false; sendMenuOpen.value = false; memberSearchOpen.value = false; sidebarMoreOpen.value = false; chatMoreOpen.value = false; requestTarget.value = null; friendRequests.value = { incoming: [], outgoing: [] }; blockedUsers.value = []; joinRequests.value = []; typingUsers.value = {}; conversationMenu.value = null; directoryQuery.value = ''; conversationFilter.value = 'current'
      try { await api('/auth/logout', { method: 'POST' }) } catch (cause) { showError(cause) }
    }
    function openCreate() {
      groupName.value = ''
      showCreate.value = true
    }
    function openAddUser() {
      friendQuery.value = ''
      requestMessage.value = ''
      showAddUser.value = true
    }
    async function openFriendRequests() {
      showFriendRequests.value = true
      try { await loadFriendRequests() } catch (cause) { showError(cause) }
    }
    async function loadFriendRequests() {
      friendRequests.value = await api('/friends/requests')
    }
    function openServerSettings() {
      showSettings.value = false
      window.dispatchEvent(new Event('desktop:open-server-settings'))
    }
    // 登录浮块内的窗口控制按钮：转发给 DesktopSettings 处理（登录态没有标题栏）。
    function windowControl(action) {
      window.dispatchEvent(new CustomEvent('desktop:window-control', { detail: { action } }))
    }
    function openProfile() {
      showSettings.value = false
      profileName.value = me.value?.name || ''
      profileEmail.value = me.value?.email || ''
      profileEditing.value = false
      showProfile.value = true
    }
    async function updateProfile() {
      if (profileBusy.value) return
      profileBusy.value = true
      try {
        const account = await api('/me', { method: 'PATCH', body: JSON.stringify({ email: profileEmail.value, name: profileName.value }) })
        me.value = account
        profileEditing.value = false
        await refresh()
      } catch (cause) { showError(cause) }
      finally { profileBusy.value = false }
    }
    function startPasswordChange() {
      currentPassword.value = ''
      newPassword.value = ''
      confirmPassword.value = ''
      passwordEditing.value = true
    }
    // Changing the password revokes every other session, so the user is told what
    // will happen before the request is sent.
    async function changePassword() {
      if (passwordBusy.value) return
      if (newPassword.value !== confirmPassword.value) { showError({ message: '两次输入的新密码不一致' }); return }
      if ([...newPassword.value].length < 8) { showError({ message: '新密码至少 8 位' }); return }
      passwordBusy.value = true
      try {
        await api('/auth/password', { method: 'POST', body: JSON.stringify({ current_password: currentPassword.value, new_password: newPassword.value }) })
        passwordEditing.value = false
        currentPassword.value = ''
        newPassword.value = ''
        confirmPassword.value = ''
        alert('密码已修改。其他设备上的登录已被退出，当前会话保持登录。')
      } catch (cause) { showError(cause) }
      finally { passwordBusy.value = false }
    }
    function addFriend(userID) {
      const existing = friendRequestFor(userID)
      if (existing) {
        openFriendRequests()
        return
      }
      if (friendBusy.value) return
      requestTarget.value = users.value.find(person => person.id === userID) || null
      requestMessage.value = ''
      showRequestComposer.value = Boolean(requestTarget.value)
    }
    async function sendFriendRequest() {
      if (friendBusy.value || !requestTarget.value) return
      friendBusy.value = true
      try {
        await api('/friends', { method: 'POST', body: JSON.stringify({ user_id: requestTarget.value.id, message: requestMessage.value }) })
        showRequestComposer.value = false
        requestTarget.value = null
        requestMessage.value = ''
        await refresh()
      } catch (cause) { showError(cause) }
      finally { friendBusy.value = false }
    }
    async function decideFriendRequest(requestID, action) {
      if (friendRequestBusy.value) return
      friendRequestBusy.value = true
      try {
        await api(`/friends/requests/${requestID}/${action}`, { method: 'POST' })
        await refresh()
      } catch (cause) { showError(cause) }
      finally { friendRequestBusy.value = false }
    }
    async function openDirect(userID) {
      if (conversationBusy.value || !userID || userID === me.value?.id) return
      const existing = conversations.value.find(item => item.kind === 'direct' && item.members.some(member => member.id === userID))
      if (existing) { await select(existing.id); return }
      const version = epoch
      conversationBusy.value = true
      try {
        const created = await api('/conversations', { method: 'POST', body: JSON.stringify({ kind: 'direct', member_ids: [userID] }) })
        if (version !== epoch) return
        await refresh()
        if (version === epoch) await select(created.id)
      } catch (cause) { if (version === epoch) showError(cause) }
      finally { conversationBusy.value = false }
    }
    function openInvite() {
      inviteUsers.value = []
      // Filters reset each time so the dialog always opens on the full list.
      resetInviteFilters()
      showInvite.value = true
    }
    async function addMembers() {
      if (conversationBusy.value || !inviteUsers.value.length || current.value?.kind !== 'group') return
      const version = epoch
      conversationBusy.value = true
      try {
        await api(`/conversations/${active.value}/members`, { method: 'POST', body: JSON.stringify({ member_ids: inviteUsers.value }) })
        if (version !== epoch) return
        showInvite.value = false
        inviteUsers.value = []
        await refresh()
      } catch (cause) { if (version === epoch) showError(cause) }
      finally { conversationBusy.value = false }
    }
    async function createGroup() {
      if (conversationBusy.value) return
      const version = epoch
      conversationBusy.value = true
      try {
        const created = await api('/conversations', { method: 'POST', body: JSON.stringify({ kind: 'group', name: groupName.value, member_ids: [] }) })
        if (version !== epoch) return
        showCreate.value = false; groupName.value = ''
        conversationFilter.value = 'group'
        await refresh()
        if (version === epoch) await select(created.id)
      } catch (cause) { if (version === epoch) showError(cause) }
      finally { conversationBusy.value = false }
    }
    async function send() {
      const id = active.value
      const version = epoch
      const media = attachment.value
      // The rich region is turned into ordered blocks, so an image and the text around
      // it travel together in the order they were written.
      syncDraftFromDom()
      const blocks = composerBlocks()
      const content = draft.value.trim()
      if ((!blocks.length && !content && !media) || !id || sending.value || uploadBusy.value) return
      sending.value = true
      try {
        // The main composer sends a message without quoting: quoting happens in
        // the conversation window, which targets that window's root. Mentions are
        // still resolved here. The stored record doubles as the WebSocket payload,
        // so every field the server reads is passed in rather than left out of it.
        const replyToID = ''
        const mentions = resolveMentions()
        const input = pendingStore.save(id, content, media?.id || '', replyToID, mentions, blocks)
        pendingSend.value = input
        const payload = { client_msg_id: input.client_msg_id, content, media_id: media?.id || '', blocks, reply_to_id: replyToID, mentions }
        if (connected.value) {
          try { await requests.send('message.send', input) }
          catch (cause) {
            if (version !== epoch) return
            if (cause.application) throw cause
            await api(`/conversations/${id}/messages`, { method: 'POST', body: JSON.stringify(payload) })
          }
        } else await api(`/conversations/${id}/messages`, { method: 'POST', body: JSON.stringify(payload) })
        if (version !== epoch) return
        pendingStore.clear(id, input.client_msg_id)
        if (pendingSend.value?.client_msg_id === input.client_msg_id) pendingSend.value = null
        // The composer is emptied only if it still holds what was just sent, so text
        // typed during the round trip is not lost.
        if (active.value === id && draft.value.trim() === content && draftArea.value) {
          draftArea.value.textContent = ''
          draft.value = ''
        }
        if (active.value === id) { clearAttachment(); clearComposerImages() }
        await syncConversation(id)
      } catch (cause) { showError(cause) }
      finally { sending.value = false }
    }
    async function loadOlder() {
      const element = stream.value
      const previousHeight = element?.scrollHeight || 0
      visibleCount.value += 100; showMessages(active.value)
      await nextTick()
      if (element) element.scrollTop += element.scrollHeight - previousHeight
    }
    function onEnter(event) {
      if (event.isComposing) return
      if (event.key === 'Enter' && (showMentions.value && mentionList.value.length)) return
      if (event.shiftKey) return
      if (sendKeyMode.value === 'ctrl') {
        if (event.ctrlKey) { event.preventDefault(); send() }
        return
      }
      event.preventDefault()
      send()
    }
    // Restoring an uncertain send puts its text and images back into the rich region,
    // so the user can retry exactly what they had written.
    function restorePending() {
      const pending = pendingSend.value
      const root = draftArea.value
      if (!pending) return
      if (root) {
        renderInto(root, pending.blocks || [], composerImages.value)
        syncDraftFromDom()
        // A send that had no blocks carries its text in content instead.
        if (!(pending.blocks || []).length) {
          root.textContent = pending.content || ''
          draft.value = pending.content || ''
          recordComposerUndo()
        }
      } else {
        draft.value = pending.content || ''
      }
    }
    // ---- message reactions ----
    function applyMessageUpdate(updated) {
      if (!updated?.id || !updated.conversation_id) return
      const items = cache.get(updated.conversation_id) || []
      const index = items.findIndex(message => message.id === updated.id)
      if (index >= 0) items.splice(index, 1, updated)
      cacheVersion.value++
      if (threadRoot.value?.id === updated.id) threadRoot.value = updated
      const replyIndex = threadReplies.value.findIndex(message => message.id === updated.id)
      if (replyIndex >= 0) threadReplies.value.splice(replyIndex, 1, updated)
      showMessages(updated.conversation_id)
    }
    const reactionMine = (item, emoji) => Boolean(item?.reactions?.find(entry => entry.emoji === emoji)?.users.includes(me.value?.id))
    async function toggleReaction(item, emoji) {
      if (!item?.id || reactionBusy.value || !active.value) return
      reactionBusy.value = true
      try {
        const method = reactionMine(item, emoji) ? 'DELETE' : 'POST'
        const updated = await api(`/conversations/${active.value}/messages/${item.id}/reactions`, { method, body: JSON.stringify({ emoji }) })
        applyMessageUpdate(updated)
        if (threadOpen.value) await refreshThread()
      } catch (cause) { showError(cause) }
      finally { reactionBusy.value = false }
    }
    // ---- typing relay ----
    function noteTyping(data) {
      if (!data?.conversation_id || !data.user_id || data.user_id === me.value?.id) return
      const map = { ...(typingUsers.value[data.conversation_id] || {}), [data.user_id]: Date.now() + 4000 }
      typingUsers.value = { ...typingUsers.value, [data.conversation_id]: map }
    }
    function notifyTyping() {
      if (!connected.value || !requests || !active.value || mutedHere.value || dismissedHere.value) return
      const now = Date.now()
      if (now - (typingSentAt[active.value] || 0) < 2000) return
      typingSentAt[active.value] = now
      try { requests.send('typing.send', { conversation_id: active.value }).catch(() => {}) } catch { /* the relay is best-effort */ }
    }
    // ---- message date dividers ----
    const sameDay = (left, right) => {
      const a = new Date(left)
      const b = new Date(right)
      return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate()
    }
    const dayDivider = index => index === 0 || !sameDay(messages.value[index - 1]?.created_at, messages.value[index].created_at)
    const dayLabel = value => {
      const date = new Date(value)
      const now = new Date()
      const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
      const time = date.getTime()
      if (time >= startOfToday) return '今天'
      if (time >= startOfToday - 86400000) return '昨天'
      const sameYear = date.getFullYear() === now.getFullYear()
      const label = date.getMonth() + 1 + ' 月 ' + date.getDate() + ' 日'
      return sameYear ? label : date.getFullYear() + ' 年 ' + label
    }
    // ---- poke ----
    async function pokeMember(userID) {
      if (!active.value || !userID || userID === me.value?.id) return
      try { await api(`/conversations/${active.value}/poke`, { method: 'POST', body: JSON.stringify({ user_id: userID }) }) } catch (cause) { showError(cause) }
    }
    // ---- direct info panel actions ----
    const directRemarkTarget = computed(() => (current.value?.kind === 'direct' ? friends.value.find(friend => friend.id === current.value.members.find(member => member.id !== me.value?.id)?.id) || null : null))
    async function startDirectRemark() {
      const target = directRemarkTarget.value
      if (!target) return
      const remark = prompt('设置好友备注（留空清除）', target.remark || '')
      if (remark === null) return
      try {
        await api(`/friends/${target.id}`, { method: 'PATCH', body: JSON.stringify({ remark: remark.trim() }) })
        await refresh()
      } catch (cause) { showError(cause) }
    }
    async function clearLocalHistory() {
      const id = active.value
      if (!id || !confirm('确认清空本机的聊天记录？服务器与对方设备不受影响。')) return
      cache.delete(id)
      unreadCounts.value = { ...unreadCounts.value, [id]: 0 }
      visibleCount.value = 100
      showMessages(id)
    }
    // ---- conversation menu and private deletion ----
    function openConversationMenu(event, item) {
      conversationMenu.value = { id: item.id, x: event.clientX || 8, y: event.clientY || 8, pinned: Boolean(item.pinned), muted: Boolean(item.muted) }
    }
    function closeConversationMenu() {
      conversationMenu.value = null
    }
    function onConversationMenuOutside(event) {
      if (conversationMenu.value && !conversationMenuElement.value?.contains(event.target)) closeConversationMenu()
      if (memberMenuID.value && !event.target.closest?.('.member-row-actions')) memberMenuID.value = ''
      if (sendMenuOpen.value && !event.target.closest?.('.send-split')) sendMenuOpen.value = false
      if (sidebarMoreOpen.value && !event.target.closest?.('.sidebar-more')) sidebarMoreOpen.value = false
      if (chatMoreOpen.value && !event.target.closest?.('.chat-more')) chatMoreOpen.value = false
    }
    function runConversationMenu(action) {
      const menu = conversationMenu.value
      closeConversationMenu()
      if (!menu) return
      if (action === 'pin') togglePreference(menu.id, 'pinned')
      if (action === 'mute') togglePreference(menu.id, 'muted')
      if (action === 'delete') deleteConversation(menu.id)
    }
    async function deleteConversation(id) {
      if (!id || conversationBusy.value) return
      if (!confirm('删除会话后将从列表中移除，收到新消息时会重新出现。确认删除？')) return
      const version = epoch
      conversationBusy.value = true
      try {
        await api(`/conversations/${id}`, { method: 'DELETE' })
        if (version !== epoch) return
        conversations.value = conversations.value.filter(item => item.id !== id)
        cache.delete(id)
        unreadCounts.value = { ...unreadCounts.value, [id]: 0 }
        if (active.value === id) { active.value = null; chatVisible.value = false }
      } catch (cause) { if (version === epoch) showError(cause) }
      finally { if (version === epoch) conversationBusy.value = false }
    }
    // ---- contact management ----
    async function openContacts() {
      showSettings.value = false
      showContacts.value = true
      const version = epoch
      try {
        const list = await api('/blocks')
        if (version === epoch) blockedUsers.value = list
      } catch (cause) { if (version === epoch) showError(cause) }
    }
    function startRemark(person) {
      remarkEditingID.value = person.id
      remarkDraft.value = person.remark || ''
    }
    async function saveRemark(person) {
      if (contactBusy.value) return
      contactBusy.value = true
      try {
        const updated = await api(`/friends/${person.id}`, { method: 'PATCH', body: JSON.stringify({ remark: remarkDraft.value }) })
        const friend = friends.value.find(item => item.id === person.id)
        if (friend) friend.remark = updated.remark
        remarkEditingID.value = ''
      } catch (cause) { showError(cause) }
      finally { contactBusy.value = false }
    }
    async function removeFriend(person) {
      if (contactBusy.value) return
      if (!confirm(`确认删除好友 ${person.remark || person.name}？聊天记录不受影响。`)) return
      contactBusy.value = true
      try {
        await api(`/friends/${person.id}`, { method: 'DELETE' })
        friends.value = friends.value.filter(item => item.id !== person.id)
      } catch (cause) { showError(cause) }
      finally { contactBusy.value = false }
    }
    const isBlocked = id => blockedUsers.value.some(entry => entry.id === id)
    async function blockUser(person) {
      if (contactBusy.value || isBlocked(person.id)) return
      if (!confirm(`确认将 ${person.remark || person.name} 加入黑名单？拉黑后不再收到对方的私聊与好友申请。`)) return
      contactBusy.value = true
      try {
        await api('/blocks', { method: 'POST', body: JSON.stringify({ user_id: person.id }) })
        blockedUsers.value = await api('/blocks')
      } catch (cause) { showError(cause) }
      finally { contactBusy.value = false }
    }
    async function unblockUser(person) {
      if (contactBusy.value) return
      contactBusy.value = true
      try {
        await api(`/blocks/${person.id}`, { method: 'DELETE' })
        blockedUsers.value = await api('/blocks')
      } catch (cause) { showError(cause) }
      finally { contactBusy.value = false }
    }
    // ---- group governance ----
    async function loadJoinRequests() {
      if (!active.value) return
      const version = epoch
      const queue = await api(`/conversations/${active.value}/join-requests`)
      if (version === epoch) joinRequests.value = queue
    }
    async function decideJoinRequest(requestID, action) {
      if (joinRequestBusy.value) return
      joinRequestBusy.value = true
      try {
        await api(`/conversations/${active.value}/join-requests/${requestID}/${action}`, { method: 'POST' })
        await loadJoinRequests()
        await refresh()
      } catch (cause) { showError(cause) }
      finally { joinRequestBusy.value = false }
    }
    async function updateGroupSettings(change) {
      if (groupSettingsBusy.value || current.value?.kind !== 'group') return
      const version = epoch
      groupSettingsBusy.value = true
      try {
        const settings = await api(`/conversations/${active.value}/settings`, { method: 'PATCH', body: JSON.stringify(change) })
        if (version !== epoch || !current.value) return
        current.value.all_muted = settings.all_muted
        current.value.join_approval = settings.join_approval
      } catch (cause) { if (version === epoch) showError(cause) }
      finally { if (version === epoch) groupSettingsBusy.value = false }
    }
    const canDismissGroup = computed(() => current.value?.kind === 'group' && currentRole.value === 'owner' && !current.value.dismissed)
    async function dismissGroup() {
      if (!current.value || !canDismissGroup.value || groupBusy.value) return
      if (!confirm(`确认解散群聊“${title(current.value)}”？解散后不能再发言，历史记录保留。`)) return
      const version = epoch
      groupBusy.value = true
      try {
        await api(`/conversations/${active.value}/dismiss`, { method: 'POST' })
        if (version !== epoch) return
        closeMembers()
        await refresh()
      } catch (cause) { if (version === epoch) showError(cause) }
      finally { if (version === epoch) groupBusy.value = false }
    }
    async function setMemberAlias(member) {
      if (groupToolBusy.value) return
      const alias = prompt('设置群昵称（留空恢复默认）', member.alias || '')
      if (alias === null) return
      groupToolBusy.value = true
      try {
        await api(`/conversations/${active.value}/members/${member.id}/alias`, { method: 'PATCH', body: JSON.stringify({ alias: alias.trim() }) })
        await refresh()
      } catch (cause) { showError(cause) }
      finally { groupToolBusy.value = false }
    }
    const memberMuted = member => Boolean(member.muted_until && Date.parse(member.muted_until) > Date.now())
    const memberCanBeMuted = member => {
      if (!canManageGroupMembers.value || member.id === me.value?.id || member.role === 'owner') return false
      return currentRole.value === 'owner' || member.role === 'member'
    }
    const memberAliasEditable = member => member.id === me.value?.id || canManageGroupMembers.value
    async function setMemberMute(member, minutes) {
      if (groupToolBusy.value) return
      if (minutes > 0 && !confirm(`确认禁言 ${member.alias || member.name} ${minutes} 分钟？`)) return
      groupToolBusy.value = true
      try {
        await api(`/conversations/${active.value}/members/${member.id}/mute`, { method: 'POST', body: JSON.stringify({ minutes }) })
        await refresh()
      } catch (cause) { showError(cause) }
      finally { groupToolBusy.value = false }
    }
    onMounted(async () => {
      try { me.value = await api('/me'); pendingStore = new PendingSends(session, me.value.id); await refresh(); if (active.value) await select(active.value) }
      catch (cause) { if (cause.status === 401) me.value = null; else showError(cause) }
      if (me.value) connect()
      refreshTimer = setInterval(async () => {
        if (!me.value) return
        try {
          refreshTicks++
          if (refreshTicks % 12 === 0) await refresh()
          else { await reconcile(); await loadFriendRequests() }
        } catch (cause) { showError(cause) }
      }, 5000)
      document.addEventListener('visibilitychange', onVisible)
      window.addEventListener('focus', onVisible)
      document.addEventListener('pointerdown', onConversationMenuOutside)
      document.getElementById('app')?.addEventListener('click', onImageLinkClick)
      typingTimer = setInterval(() => { typingTick.value++ }, 1000)
    })
    onUpdated(() => {
      if (draftArea.value) { installEmojiPicker(draftArea.value); installComposerUndo(draftArea.value) }
    })
    const removeDesktopDownloads = installDesktopDownloads(showError)
    onUnmounted(() => { removeDesktopDownloads(); document.getElementById('app')?.removeEventListener('click', onImageLinkClick); resetSearch(); clearTimeout(highlightTimer); releaseAttachment(); disposed = true; epoch++; requests?.close(); socket?.close(); clearTimeout(reconnectTimer); clearInterval(refreshTimer); clearInterval(typingTimer); clearTimeout(errorTimer); document.removeEventListener('visibilitychange', onVisible); window.removeEventListener('focus', onVisible); document.removeEventListener('pointerdown', onConversationMenuOutside) })
    return { isDesktop, openServerSettings, windowControl, sidebarMoreOpen, chatMoreOpen, toggleSidebarMore, toggleChatMore, sendKeyMode, sendKeyHint, setSendKey, sendMenuOpen, closeConversation, memberSearchOpen, toggleMemberSearch, preferenceBusy, memberByID, headMember, chatInfoOpen, dayDivider, dayLabel, pokeMember, startDirectRemark, clearLocalHistory, UserAvatar, chatHeadMeta, memberMenuID, toggleMemberMenu, messageActions, REACTION_CHOICES, reactionMine, toggleReaction, reactionBusy, typingLabel, mutedHere, dismissedHere, readTicks, showContacts, contactBusy, remarkEditingID, remarkDraft, blockedUsers, isBlocked, openContacts, startRemark, saveRemark, removeFriend, blockUser, unblockUser, conversationMenu, conversationMenuElement, openConversationMenu, closeConversationMenu, runConversationMenu, deleteConversation, joinRequests, joinRequestBusy, decideJoinRequest, updateGroupSettings, groupSettingsBusy, canDismissGroup, dismissGroup, setMemberAlias, setMemberMute, memberMuted, memberCanBeMuted, memberAliasEditable, groupToolBusy, showMembers, memberQuery, currentMembers, memberCount, memberBusy, currentRole, isGroupOwner, canInviteMembers, canManageGroupMembers, canEditGroupAnnouncement, canLeaveGroup, memberCanBeRemoved, memberCanChangeRole, canTransferTo, roleLabel, groupAnnouncements, announcementsBusy, announcementsError, announcementPanelOpen, announcementEditorOpen, announcementEditingID, announcementDraftText, announcementSaving, announcementCount, latestAnnouncement, canManageAnnouncements, loadAnnouncements, openAnnouncementPanel, closeAnnouncementPanel, startCreateAnnouncement, startEditAnnouncement, cancelAnnouncementEditor, saveAnnouncementText, deleteAnnouncement, groupBusy, removeMember, changeRole, transferOwnership, leaveGroup, pinnedConversations, showsBadge, threadOpen, threadRoot, threadReplies, threadHasMorePages, threadBusy, threadError, threadReplyCount, threadStream, openThread, closeThread, loadMoreThread, threadDraft, threadSending, threadDraftArea, sendInThread, refreshThread, onThreadEnter, hasThread, threadLabel, threadPreview, hiddenReplyCount, setPreference, togglePreference, currentPinned, currentMuted, openMembers, closeMembers, inviteFromMembers, globalQuery, globalResults, globalBusy, globalError, globalHasMore, globalStarted, showGlobalSearch, globalInput, openGlobalSearch, closeGlobalSearch, searchAll, openGlobalResult, anySender, isGenericFile, directoryQuery, directoryInput, directoryPlaceholder, directoryCount, filteredFriends, filteredGroups, filteredRecent, clearDirectorySearch, showSearch, searchInput, searchQuery, searchedQuery, searchResults, searchBusy, searchError, searchHasMore, searchStarted, locatingMessage, highlightedMessage, openSearch, closeSearch, changeSearch, searchMessages, locateMessage, highlightMatches, searchDate, me, mode, email, password, name, error, users, friends, recentConversations, groupConversations, friendCandidates, sortedFriends, friendQuery, conversationFilter, active, messages, pendingSend, draft, groupName, inviteUsers, inviteCandidates, inviteQuery, inviteInitial, inviteVisible, inviteGroups, inviteInitials, inviteSummary, inviteSelectedCount, inviteSelectedPeople, chooseInviteInitial, toggleInvite, selectAllInvites, clearInvites, resetInviteFilters, OTHER_INITIAL, conversationBusy, friendBusy, showCreate, showAddUser, showInvite, showSettings, showProfile, showFriendRequests, showRequestComposer, friendRequests, pendingIncomingRequests, friendRequestFor, friendRequestStatus, friendRequestBusy, requestMessage, requestTarget, profileEditing, profileBusy, profileName, profileEmail, passwordEditing, passwordBusy, currentPassword, newPassword, confirmPassword, startPasswordChange, changePassword, connected, loading, sending, stream, current, title, sender, time, directConversation, preview, contactPreview, unreadLabel, hasUnread, authenticate, logout, openCreate, openAddUser, openFriendRequests, openProfile, updateProfile, addFriend, sendFriendRequest, decideFriendRequest, openDirect, openInvite, addMembers, createGroup, select, send, loadOlder, onEnter, discardPending, mediaInput, uploadBusy, attachment, previewFile, mediaURL, formatBytes, openMediaPicker, chooseMedia, clearAttachment, composerImages, composerHasContent, removeComposerImage, clearComposerImages, syncDraftFromDom, onComposerInput, onComposerKeydown, onComposerClick, onComposerBlur, restorePending, dragging, onComposerPaste, onDragOver, onDragLeave, onDrop, draftArea, startReply, replyPreview, replyAuthor, bodyOf, segmentsOf, usesBlocks, recalledText, canRecallMessage, recallMessage, showMentions, mentionList, mentionIndex, mentionQuery, chooseMention, mentionKeydown, updateMentionState }
  },
  template,
}).mount('#app')
