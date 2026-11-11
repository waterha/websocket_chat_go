import ContentState from './content-state.js'
import { buildConversationIndex, latestVisibleMessages, sortConversationsByLatest } from './conversation-index.js'
import { useStickers } from './stickers.js'
import { messagePresentation } from './message-presentation.js'
import { useConversationDrafts } from './conversation-drafts.js'
import { useMessageNotifications } from './message-notifications.js'
import './message-notifications.css'
import { useComposerUndo } from './composables/use-composer-undo.js'
import { useGlobalSearch } from './composables/use-global-search.js'
import { useHistorySearch } from './composables/use-history-search.js'
import { useContacts } from './composables/use-contacts.js'
import { useThreadWindow } from './composables/use-thread-window.js'
import { useAnnouncements } from './composables/use-announcements.js'
import { useProfile } from './composables/use-profile.js'
import { createApp, ref, computed, watch, onMounted, onUnmounted, onUpdated, nextTick } from 'vue/dist/vue.esm-bundler.js'
import { Search, X, ArrowRight, RotateCw, Users, Paperclip, Scissors, Download, Check, Pin, BellOff, Info, ChevronRight, ChevronDown, Clock3, ImagePlus, MoreHorizontal, MessageCircle, UserPlus, Bell, Contact, UsersRound } from 'lucide-vue-next'
import { highlightMatches } from './message-search.js'
import { historyMessagePreview } from './history-filters.js'
import { filterMembers } from './member-list.js'
import { filterDirectory, friendSearchFields, conversationSearchFields } from './directory-search.js'
import { toggleConversationSelection } from './conversation-selection.js'
import { appendPage, PendingSends, Requests, SyncTasks } from './realtime.js'
import { canChangeRole, canEditAnnouncement, canInvite, canLeave, canManageMembers, canRemoveMember, canTransferOwnership, roleLabel, roleOf } from './group.js'
import { groupByPin, showsUnreadBadge } from './preferences.js'
import { activeMentionToken, applyMention, canRecall, mentionCandidates, messageBody, recalledLabel, replyExcerpt, replyLabel } from './reply.js'
import { hasThread, hiddenReplyCount, replyTargetForWindow, threadLabel, threadPreview } from './thread.js'
import { availableInitials, filterCandidates, groupByInitial, keepSelectable, OTHER_INITIAL, selectAllCandidates, selectionSummary, toggleSelection } from './invite.js'
import { formatBytes, mediaPreview, mediaURL, validateMediaFile } from './media.js'
import { collectImages, hasTransferableFiles, prepareImage } from './image-input.js'
import { hasBlocks, messageSegments, nextImageIndex } from './composer-blocks.js'
import { caretRange, collectPieces, deleteImageNearCaret, hasContent, imageIDsIn, insertImageAtCaret, insertTextAtCaret, piecesToBlocks, piecesToText, placeCaretAfter, removeImageFromComposer, renderInto, textBeforeCaret } from './composer-dom.js'
import { installEmojiPicker } from './emoji-picker.js'
import { installScreenshot } from './screenshot.js'
import './screenshot.css'
import { useMessageActions } from './message-actions.js'
import { api, uploadMedia } from './api.js'
import { onImageLinkClick } from './image-viewer.js'
import { chatStorage, createSocket, installDesktopDownloads, isDesktop } from './desktop.js'
import { DesktopSettings } from './desktop-settings.js'
import { AppearanceSettings } from './appearance-settings.js'
import { applyTheme, loadTheme } from './appearance-theme.js'
import './appearance.css'
import { template } from './template.js'
import { LayoutDivider } from './layout-divider.js'
import { GroupDirectory } from './group-directory.js'
import { FileTypeIcon } from './file-type-icon.js'
import { GroupFiles } from './group-files.js'
import { useVoice } from './voice.js'
import { UserAvatar } from './user-avatar.js'
import { FavoritesPanel, useFavorites } from './favorites.js'
import './group-directory.css'
import './style.css'
import './desktop.css'


createApp({
  components: { ContentState, Search, X, ArrowRight, RotateCw, Users, Paperclip, Scissors, Download, Check, Pin, BellOff, Info, ChevronRight, ChevronDown, Clock3, ImagePlus, MoreHorizontal, MessageCircle, UserPlus, Bell, Contact, UsersRound, DesktopSettings, AppearanceSettings, UserAvatar, FavoritesPanel, GroupDirectory, GroupFiles, FileTypeIcon, LayoutDivider },
  setup() {
    applyTheme(loadTheme())
    const showAppearance = ref(false)
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
    const messagesLoading = ref(false)
    const messagesError = ref('')
    let messageLoadVersion = 0
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
    const groupDirectory = ref(null)
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
    const { globalQuery, globalResults, globalBusy, globalError, globalStarted, globalHasMore, globalCursor, showGlobalSearch, globalInput, resetGlobalSearch, closeGlobalSearch, openGlobalSearch, searchAll } = useGlobalSearch()
    watch(() => me.value?.id, closeGlobalSearch, { flush: 'sync' })
    const showSettings = ref(false)
    const showFriendRequests = ref(false)
    const friendRequests = ref({ incoming: [], outgoing: [] })
    const friendRequestBusy = ref(false)
    const requestMessage = ref('')
    const requestTarget = ref(null)
    const showRequestComposer = ref(false)
    const {
      showContacts, contactBusy, remarkEditingID, remarkDraft, blockedUsers, openContacts, startRemark,
      saveRemark, removeFriend, isBlocked, blockUser, unblockUser,
    } = useContacts({ showSettings, friends, showError, getEpoch: () => epoch })
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
    const {
      showProfile, profileEditing, profileBusy, profileName, profileEmail, profileAvatarBusy, avatarInput,
      passwordEditing, passwordBusy, currentPassword, newPassword, confirmPassword, openProfile,
      updateProfile, uploadProfileAvatar, removeProfileAvatar, startPasswordChange, changePassword,
    } = useProfile({ me, showSettings, refresh, showError, convertImageToPNG, getEpoch: () => epoch })
    const conversationBusy = ref(false)
    const friendBusy = ref(false)
    const connected = ref(false)
    const loading = ref(false)
    const stream = ref(null)
    const sending = ref(false)
    const mediaInput = ref(null)
    const fileInput = ref(null)
    const uploadBusy = ref(false)
    const uploadName = ref('')
    let attachmentController
    let attachmentUploadVersion = 0
    // Images live inside the draft as tokens, so one message can interleave text and
    // pictures in the order they were added. `attachment` remains for a video or a
    // generic file, which occupy a message on their own.
    // Mirrors the server's block limit: every image is one block in the message.
    const MAX_COMPOSER_IMAGES = 20
    const composerImages = ref([])
    const attachment = ref(null)
    const previewFile = ref(null)
    const visibleCount = ref(100)
    const locatingMessage = ref(null)
    const highlightedMessage = ref(null)
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

    const conversationIndex = computed(() => {
      cacheVersion.value
      return buildConversationIndex(conversations.value, me.value?.id, cache)
    })
    const current = computed(() => conversationIndex.value.byID.get(active.value))
    const {
      showSearch, searchInput, searchQuery, searchedQuery, searchSender, historyMemberQuery,
      historyMemberOptions, searchType, searchStart, searchEnd, searchResults, searchBusy, searchError,
      searchHasMore, searchBefore, searchStarted, resetSearch, closeSearch, clearSearchFilters,
      openSearch, changeSearch, searchMessages,
    } = useHistorySearch({ active, current, onReset: () => { locateVersion++; locatingMessage.value = null } })
    const latestMessage = item => conversationIndex.value.latestByID.get(item?.id)
    const latestTime = item => Date.parse(latestMessage(item)?.created_at || '') || 0
    const sortByLatestTime = items => sortConversationsByLatest(items, conversationIndex.value)
    const directConversation = userID => conversationIndex.value.directByUser.get(userID)
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
    const {
      groupAnnouncements, announcementsBusy, announcementsError, announcementPanelOpen,
      announcementEditorOpen, announcementEditingID, announcementDraftText, announcementSaving,
      announcementCount, latestAnnouncement, canManageAnnouncements, loadAnnouncements,
      openAnnouncementPanel, closeAnnouncementPanel, startCreateAnnouncement, startEditAnnouncement,
      cancelAnnouncementEditor, saveAnnouncementText, deleteAnnouncement,
    } = useAnnouncements({ active, current, canEditGroupAnnouncement, refresh, showError, getEpoch: () => epoch })
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
    const friendsByID = computed(() => new Map(friends.value.map(friend => [friend.id, friend])))
    const friendRemark = id => friendsByID.value.get(id)?.remark || ''
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
    const currentMembersByID = computed(() => new Map((current.value?.members || []).map(member => [member.id, member])))
    const memberByID = id => currentMembersByID.value.get(id) || null
    const conversationAvatar = conversation => conversation?.kind === 'direct'
      ? conversation.members?.find(member => member.id !== me.value?.id) || null
      : null
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
    const {
      threadOpen, threadRoot, threadReplies, threadHasMorePages, threadBusy, threadError,
      threadReplyCount, threadStream, threadDraft, threadSending, threadDraftArea, closeThread,
      openThread, loadMoreThread, sendInThread, refreshThread, onThreadEnter,
    } = useThreadWindow({ active, syncConversation, showError, getEpoch: () => epoch })
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
      const member = memberByID(id)
      return member ? member.alias || member.name || '成员' : '成员'
    }
    // Global search hits may belong to any of the caller's conversations, so the
    // name is resolved across every known member list rather than the active one.
    const anySender = id => conversationIndex.value.memberNames.get(id) || (me.value?.id === id ? me.value?.name : '成员')
    const time = value => new Date(value).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
    const preview = item => {
      const draftPreview = conversationDrafts.draftPreview(item.id)
      if (draftPreview) return draftPreview
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
    function showError(cause) { error.value = cause?.message || '操作失败'; clearTimeout(errorTimer); errorTimer = setTimeout(() => { error.value = '' }, 5000) }
    const atBottom = () => !stream.value || stream.value.scrollHeight - stream.value.scrollTop - stream.value.clientHeight < 64
    const showMessages = id => { if (id === active.value) messages.value = latestVisibleMessages(cache.get(id) || [], visibleCount.value) }
    function discardPending() {
      if (!pendingSend.value || !confirm('放弃确认不等于撤回：消息可能已经保存在服务器。请先检查聊天记录，确定仍要放弃吗？')) return
      pendingStore.clear(pendingSend.value.conversation_id, pendingSend.value.client_msg_id)
      pendingSend.value = null
    }
    function openMediaPicker() {
      if (!active.value || uploadBusy.value || sending.value) return
      mediaInput.value?.click()
    }
    function openFilePicker() {
      if (!active.value || uploadBusy.value || sending.value) return
      fileInput.value?.click()
    }
    function cancelFileUpload() {
      if (!attachmentController) return
      attachmentUploadVersion++
      attachmentController.abort()
      attachmentController = null
      uploadBusy.value = false
      uploadName.value = ''
    }
    function releaseAttachment() {
      if (previewFile.value) URL.revokeObjectURL(previewFile.value)
      previewFile.value = null
      attachment.value = null
    }
    function clearAttachment() {
      cancelFileUpload()
      releaseAttachment()
      if (mediaInput.value) mediaInput.value.value = ''
      if (fileInput.value) fileInput.value.value = ''
    }
    async function chooseMedia(event) {
      const input = event.target
      const file = input.files?.[0]
      input.value = ''
      await acceptAttachment(file)
    }
    // Upload first, then hold the attachment for an explicit message send.
    async function acceptAttachment(file) {
      if (!file || !active.value || uploadBusy.value || sending.value) return
      if (voice.enabled.value && !file.type?.startsWith('audio/')) { showError({ message: '请先关闭语音模式，再上传文件' }); return }
      const check = validateMediaFile(file)
      if (!check.ok) { showError({ message: check.error }); return }
      if (composerImages.value.length) { showError({ message: '请先发送或移除输入框中的图片，再上传文件' }); return }
      if (pendingSend.value?.conversation_id === active.value) { showError({ message: '上一条发送结果未确认，请先恢复或放弃' }); return }
      const id = active.value
      const version = epoch
      const uploadVersion = ++attachmentUploadVersion
      const controller = new AbortController()
      attachmentController = controller
      uploadBusy.value = true
      uploadName.value = file.name || '文件'
      try {
        const uploaded = await uploadMedia(id, file, { signal: controller.signal })
        if (controller.signal.aborted || uploadVersion !== attachmentUploadVersion || version !== epoch || active.value !== id) return
        releaseAttachment()
        attachment.value = uploaded
        if (uploaded.media_type !== 'file') previewFile.value = URL.createObjectURL(file)
      } catch (cause) { if (!controller.signal.aborted && version === epoch && uploadVersion === attachmentUploadVersion) showError(cause) }
      finally {
        if (uploadVersion === attachmentUploadVersion) {
          attachmentController = null
          uploadBusy.value = false
          uploadName.value = ''
        }
      }
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
      if (!id || !files.length || uploadBusy.value || sending.value) return
      if (voice.enabled.value) { showError({ message: '请先关闭语音模式，再添加图片' }); return }
      if (attachment.value) {
        for (const placeholder of placeholders) placeholder.remove()
        if (placeholders.length) syncDraftFromDom()
        showError({ message: '请先发送或移除附件，再添加图片' })
        return
      }
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
    const voice = useVoice({
      canRecord: () => Boolean(me.value && active.value && !sending.value && !uploadBusy.value
        && !attachment.value && !composerImages.value.length && !draft.value.trim()
        && !threadOpen.value && !showSearch.value && !mutedHere.value && !dismissedHere.value
        && pendingSend.value?.conversation_id !== active.value),
      getContext: () => ({ id: active.value, version: epoch }),
      isCurrent: context => context.id === active.value && context.version === epoch,
      showError,
      onSend: async (file, context) => {
        await acceptAttachment(file)
        if (context.id !== active.value || context.version !== epoch || attachment.value?.media_type !== 'audio') return
        await send()
      },
    })
    const voiceEnabled = voice.enabled, voiceState = voice.state, voiceSeconds = voice.seconds
    function toggleVoice() {
      if (!voice.enabled.value && (attachment.value || composerImages.value.length || draft.value.trim())) {
        showError({ message: '请先发送或清空当前草稿，再开启语音' }); return
      }
      voice.toggle()
    }
    watch(active, () => voice.reset(), { flush: 'sync' })
    const screenshot = installScreenshot({
      canCapture: () => Boolean(!voice.enabled.value && me.value && active.value && draftArea.value
        && !threadOpen.value && !showSearch.value && !sending.value && !uploadBusy.value
        && !mutedHere.value && !dismissedHere.value && pendingSend.value?.conversation_id !== active.value
        && !document.querySelector('.modal-backdrop, .image-viewer-overlay')),
      getContext: () => ({ id: active.value, version: epoch, root: draftArea.value, range: caretRange(draftArea.value, window.getSelection()).cloneRange() }),
      isCurrent: context => context.version === epoch && context.id === active.value && context.root === draftArea.value,
      onCapture: async (file, context) => {
        const root = context.root
        const range = root.contains(context.range.startContainer) && root.contains(context.range.endContainer)
          ? context.range : caretRange(root, window.getSelection())
        root.focus()
        const placeholder = document.createElement('span')
        placeholder.textContent = '截图上传中…'
        range.deleteContents()
        range.insertNode(placeholder)
        await acceptImages([file], [placeholder])
      },
      onError: showError,
    })
    watch([active, me], () => screenshot.cancel())
    // Removing an image drops its picture from the draft as well as its entry.
    function removeComposerImage(image) {
      if (!image) return
      if (draftArea.value) removeImageFromComposer(draftArea.value, image.mediaId)
      if (image.previewURL) URL.revokeObjectURL(image.previewURL)
      composerImages.value = composerImages.value.filter(entry => entry.mediaId !== image.mediaId)
      syncDraftFromDom()
    }
    const {
      resetComposerUndo, recordComposerUndo, installComposerUndo, isComposerComposing,
    } = useComposerUndo({ draftArea, composerImages, uploadBusy, draft, updateMentionState })
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
      if (event.isComposing || isComposerComposing() || event.keyCode === 229 || event.altKey || event.ctrlKey || event.metaKey) return
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
      } else {
        const files = Array.from(event.clipboardData?.files || [])
        if (!files.length) return
        event.preventDefault()
        if (files.length > 1) { showError({ message: '一次只能上传一个文件，请分次发送' }); return }
        acceptAttachment(files[0]).catch(showError)
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
      if (!active.value || !hasTransferableFiles(event.dataTransfer)) return
      event.preventDefault()
      const files = Array.from(event.dataTransfer?.files || [])
      const images = collectImages(event.dataTransfer)
      if (images.length && (!files.length || images.length === files.length)) {
        acceptImages(images).catch(showError)
      } else if (files.length === 1) {
        acceptAttachment(files[0]).catch(showError)
      } else if (files.length > 1) {
        showError({ message: '一次只能上传一个文件，请分次发送' })
      }
    }
    // Reply, mention and recall helpers.
    const replyPreview = item => replyExcerpt(item)
    const replyAuthor = item => replyLabel(item, sender)
    const bodyOf = item => messageBody(item)
    // A message with ordered blocks renders segment by segment; the older shapes keep
    // their single media plus caption.
    const segmentsOf = item => messageSegments(item)
    const usesBlocks = item => hasBlocks(item)
    const recalledText = item => recalledLabel(item, me.value?.id, sender)
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
        const updated = await api(`/conversations/${id}/messages/${item.seq}/recall`, { method: 'POST' })
        if (version !== epoch) return
        applyMessageUpdate(updated)
      } catch (cause) { if (version === epoch) showError(cause) }
      finally { if (version === epoch) sending.value = false }
    }
    function captureConversationDraft() {
      const pieces = draftArea.value ? collectPieces(draftArea.value) : [{ type: 'text', text: draft.value }]
      return {
        blocks: pieces.map(piece => piece.type === 'image' ? { type: 'image', media_id: piece.mediaId } : { type: 'text', text: piece.text }),
        images: composerImages.value, attachment: attachment.value,
      }
    }
    function clearConversationDraft() {
      clearAttachment()
      clearComposerImages()
      draft.value = ''
      if (draftArea.value) draftArea.value.textContent = ''
      resetComposerUndo()
    }
    function restoreConversationDraft(snapshot) {
      composerImages.value = snapshot.images
      attachment.value = snapshot.attachment
      previewFile.value = snapshot.attachment && snapshot.attachment.media_type !== 'file' ? mediaURL(snapshot.attachment.id) : null
      draft.value = snapshot.blocks.filter(block => block.type === 'text').map(block => block.text).join('')
      if (draftArea.value) renderInto(draftArea.value, snapshot.blocks, snapshot.images)
      resetComposerUndo()
    }
    const conversationDrafts = useConversationDrafts({
      active, userID: computed(() => me.value?.id), storage: saved,
      sources: [draft, composerImages, attachment], capture: captureConversationDraft,
      clear: clearConversationDraft, restore: restoreConversationDraft, showError,
    })
    const messageNotifications = useMessageNotifications({
      me, conversations, title, showError,
      isViewingConversation: id => active.value === id && chatVisible.value && !showSearch.value,
      onOpen: async id => { if (active.value !== id || !chatVisible.value) await select(id) },
    })
    const showFavorites = ref(false)
    const favorites = useFavorites()
    const stickers = useStickers()
    const { notices: messageNotices, notificationPermission, openNotice, dismissNotice, enableSystemNotifications } = messageNotifications
    const messageActions = useMessageActions({ onAddSticker: stickers.save, onFavorite: favorites.save, messages, threadRoot, threadReplies, active, me, current, conversations, startReply, recallMessage, syncConversation, showError })
    let stickerContextVersion = 0
    watch([active, () => me.value?.id], () => { stickerContextVersion++ }, { flush: 'sync' })
    function canInsertSticker() {
      return Boolean(me.value && active.value && chatVisible.value && draftArea.value
        && !showSearch.value && !threadOpen.value && !sending.value && !uploadBusy.value
        && !voice.enabled.value && !mutedHere.value && !dismissedHere.value
        && pendingSend.value?.conversation_id !== active.value)
    }
    async function selectSticker(sticker) {
      if (!canInsertSticker()) throw new Error('当前无法添加表情，请先关闭搜索、语音或等待正在进行的操作完成')
      const root = draftArea.value
      if (sticker.kind === 'emoji') {
        insertTextAtCaret(root, sticker.text, window.getSelection())
        return
      }
      if (attachment.value) throw new Error('请先发送或移除附件，再添加图片表情')
      if (composerImages.value.length >= MAX_COMPOSER_IMAGES) throw new Error('一条消息最多附带20张图片')
      const id = active.value, version = stickerContextVersion
      const range = caretRange(root, window.getSelection()).cloneRange()
      uploadBusy.value = true
      try {
        const uploaded = await stickers.prepare(sticker, id)
        if (version !== stickerContextVersion || active.value !== id || root !== draftArea.value || !root.isConnected) return
        if (mutedHere.value || dismissedHere.value) throw new Error('当前会话不可发送消息')
        if (!uploaded.id || uploaded.media_type !== 'image') throw new Error('图片表情准备失败，请重试')
        const image = { index: nextImageIndex(composerImages.value), mediaId: uploaded.id,
          previewURL: mediaURL(uploaded.id), fileName: uploaded.file_name || sticker.file_name || '表情图片' }
        composerImages.value = [...composerImages.value, image]
        const target = root.contains(range.startContainer) && root.contains(range.endContainer) ? range : caretRange(root, window.getSelection())
        const chip = insertImageAtCaret(root, target, image)
        placeCaretAfter(root, chip, window.getSelection())
        root.focus()
        syncDraftFromDom()
      } finally { if (version === stickerContextVersion) uploadBusy.value = false }
    }
    const emojiPickerOptions = { getStickers: stickers.list, onSelectSticker: selectSticker,
      uploadSticker: stickers.upload, removeSticker: stickers.remove,
      getUserID: () => me.value?.id || '', onError: showError }
    const isGenericFile = computed(() => attachment.value?.media_type === 'file')
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
        messageNotifications.prime(list)
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
    async function refreshThreadSummaries(id, incoming) {
      const roots = [...new Set(incoming.map(message => message.thread_root_id).filter(Boolean))]
      if (!roots.length) return
      const items = cache.get(id) || []
      const pages = await Promise.all(roots.map(rootID => api(`/conversations/${id}/messages/${rootID}/thread?limit=2`)))
      const summaries = new Map(pages.map(page => [page.root.id, { reply_count: page.reply_count || 0, preview: page.replies || [] }]))
      cache.set(id, items.map(message => summaries.has(message.id) ? { ...message, thread: summaries.get(message.id) } : message))
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
          if (merged !== existing) cacheVersion.value++
          messageNotifications.receive(id, page.messages)
                     if (page.messages.some(message => message.thread_root_id)) await refreshThreadSummaries(id, page.messages)
           const pending = pendingStore?.get(id)
          if (pending && page.messages.some(message => message.sender_id === me.value.id && message.client_msg_id === pending.client_msg_id)) {
            pendingStore.clear(id, pending.client_msg_id)
            if (pendingSend.value?.client_msg_id === pending.client_msg_id) pendingSend.value = null
            if (active.value === id && draft.value.trim() === pending.content) draft.value = ''
          }
          if (merged !== existing) showMessages(id)
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
    watch([active, () => me.value?.id], () => {
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
      messageLoadVersion++
      messagesLoading.value = false
      messagesError.value = ''
      if (!selection.chatVisible) {
        closeSearch()
        return
      }
      await nextTick()
      if (draftArea.value) installEmojiPicker(draftArea.value, emojiPickerOptions)
      markSeen(id, cache.get(id)?.at(-1)?.seq || 0)
      pendingSend.value = pendingStore?.get(id) || null
      visibleCount.value = 100
      showMessages(id)
      await loadCurrentMessages(id)
    }
    async function loadCurrentMessages(id = active.value) {
      if (!id) return
      const version = ++messageLoadVersion
      const accountEpoch = epoch
      messagesLoading.value = true
      messagesError.value = ''
      try {
        await syncConversation(id)
        if (version !== messageLoadVersion || accountEpoch !== epoch || active.value !== id) return
        await loadAnnouncements()
        if (version === messageLoadVersion && active.value === id && chatVisible.value) await scroll()
      } catch (cause) {
        if (version === messageLoadVersion && accountEpoch === epoch && active.value === id) messagesError.value = cause.message || '聊天记录加载失败，请重试。'
      } finally {
        if (version === messageLoadVersion && accountEpoch === epoch) messagesLoading.value = false
      }
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
          if (envelope.event === 'message.recalled') applyMessageUpdate(envelope.data)
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
      epoch++; me.value = null; connected.value = false
      clearTimeout(reconnectTimer); requests?.close(); socket?.close(); socket = null
      cache.clear(); syncing.clear(); refreshTask = null; pendingStore = null; pendingSend.value = null; unreadCounts.value = {}
      active.value = null; messages.value = []; conversations.value = []; users.value = []; friends.value = []; draft.value = ''
      showCreate.value = false; showAddUser.value = false; showInvite.value = false; showSettings.value = false; showProfile.value = false; showFavorites.value = false; showAppearance.value = false; showFriendRequests.value = false; showRequestComposer.value = false; showContacts.value = false; chatInfoOpen.value = false; sendMenuOpen.value = false; memberSearchOpen.value = false; sidebarMoreOpen.value = false; chatMoreOpen.value = false; requestTarget.value = null; friendRequests.value = { incoming: [], outgoing: [] }; blockedUsers.value = []; joinRequests.value = []; typingUsers.value = {}; conversationMenu.value = null; directoryQuery.value = ''; conversationFilter.value = 'current'
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
    function openGroupDirectory() {
      sidebarMoreOpen.value = false
      groupDirectory.value?.open()
    }
    async function onGroupJoined(id) {
      const version = epoch
      try {
        await refresh()
        if (version !== epoch) return
        conversationFilter.value = 'group'
        directoryQuery.value = ''
        await select(id)
      } catch (cause) { if (version === epoch) showError(cause) }
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
      const draftOwner = me.value?.id
      // The rich region is turned into ordered blocks, so an image and the text around
      // it travel together in the order they were written.
      syncDraftFromDom()
      const blocks = media ? [] : composerBlocks()
      const content = draft.value.trim()
      const sentDraft = captureConversationDraft()
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
        conversationDrafts.clearSent(draftOwner, id, sentDraft)
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
      if (event.isComposing || isComposerComposing() || event.keyCode === 229) return
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
    // Apply mutations to existing messages: the sync cursor only fetches new sequences.
    function applyMessageUpdate(updated) {
      if (!updated?.id || !updated.conversation_id) return
      function updateMessage(message) {
        if (!message) return message
        let result = message.id === updated.id ? updated : message
        if (updated.recalled_at && result.reply?.id === updated.id) {
          result = { ...result, reply: { ...result.reply, recalled: true, excerpt: '', media_type: '' } }
        }
        if (result.thread?.preview?.length) {
          result = { ...result, thread: { ...result.thread, preview: result.thread.preview.map(updateMessage) } }
        }
        return result
      }
      const id = updated.conversation_id
      if (cache.has(id)) cache.set(id, cache.get(id).map(updateMessage))
      cacheVersion.value++
      if (active.value === id) {
        threadRoot.value = updateMessage(threadRoot.value)
        threadReplies.value = threadReplies.value.map(updateMessage)
        searchResults.value = searchResults.value.filter(message => !updated.recalled_at || message.id !== updated.id).map(updateMessage)
      }
      globalResults.value = globalResults.value
        .filter(hit => !updated.recalled_at || hit.message.id !== updated.id)
        .map(hit => hit.message.conversation_id === id ? { ...hit, message: updateMessage(hit.message) } : hit)
      showMessages(id)
    }
    // ---- message reactions ----
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
      typingTimer = setInterval(() => {
        const now = Date.now()
        let activeChanged = false
        for (const [id, typists] of Object.entries(typingUsers.value)) {
          for (const [userID, expiry] of Object.entries(typists)) {
            if (expiry <= now) { delete typists[userID]; if (id === active.value) activeChanged = true }
          }
          if (!Object.keys(typists).length) delete typingUsers.value[id]
        }
        if (activeChanged) typingTick.value++
      }, 1000)
    })
    onUpdated(() => {
      if (draftArea.value) { installEmojiPicker(draftArea.value, emojiPickerOptions); installComposerUndo(draftArea.value) }
    })
    const removeDesktopDownloads = installDesktopDownloads(showError)
    onUnmounted(() => { voice.dispose(); cancelFileUpload(); screenshot.dispose(); removeDesktopDownloads(); document.getElementById('app')?.removeEventListener('click', onImageLinkClick); resetSearch(); resetGlobalSearch(); clearTimeout(highlightTimer); releaseAttachment(); disposed = true; epoch++; requests?.close(); socket?.close(); clearTimeout(reconnectTimer); clearInterval(refreshTimer); clearInterval(typingTimer); clearTimeout(errorTimer); document.removeEventListener('visibilitychange', onVisible); window.removeEventListener('focus', onVisible); document.removeEventListener('pointerdown', onConversationMenuOutside) })
    return { messagesLoading, messagesError, loadCurrentMessages, loadAnnouncements, messagePresentation, showFavorites, messageNotices, notificationPermission, openNotice, dismissNotice, enableSystemNotifications, showAppearance, historyMemberQuery, historyMemberOptions, voiceEnabled, voiceState, voiceSeconds, toggleVoice, cancelVoice: voice.cancel, fileInput, openFilePicker, cancelFileUpload, uploadName, searchSender, searchType, searchStart, searchEnd, clearSearchFilters, historyMessagePreview, startScreenshot: screenshot.start, profileAvatarBusy, avatarInput, uploadProfileAvatar, removeProfileAvatar, onGroupJoined, openGroupDirectory, groupDirectory, isDesktop, openServerSettings, sidebarMoreOpen, chatMoreOpen, toggleSidebarMore, toggleChatMore, sendKeyMode, sendKeyHint, setSendKey, sendMenuOpen, closeConversation, memberSearchOpen, toggleMemberSearch, preferenceBusy, memberByID, conversationAvatar, headMember, chatInfoOpen, dayDivider, dayLabel, pokeMember, startDirectRemark, clearLocalHistory, UserAvatar, chatHeadMeta, memberMenuID, toggleMemberMenu, messageActions, REACTION_CHOICES, reactionMine, toggleReaction, reactionBusy, typingLabel, mutedHere, dismissedHere, readTicks, showContacts, contactBusy, remarkEditingID, remarkDraft, blockedUsers, isBlocked, openContacts, startRemark, saveRemark, removeFriend, blockUser, unblockUser, conversationMenu, conversationMenuElement, openConversationMenu, closeConversationMenu, runConversationMenu, deleteConversation, joinRequests, joinRequestBusy, decideJoinRequest, updateGroupSettings, groupSettingsBusy, canDismissGroup, dismissGroup, setMemberAlias, setMemberMute, memberMuted, memberCanBeMuted, memberAliasEditable, groupToolBusy, showMembers, memberQuery, currentMembers, memberCount, memberBusy, currentRole, isGroupOwner, canInviteMembers, canManageGroupMembers, canEditGroupAnnouncement, canLeaveGroup, memberCanBeRemoved, memberCanChangeRole, canTransferTo, roleLabel, groupAnnouncements, announcementsBusy, announcementsError, announcementPanelOpen, announcementEditorOpen, announcementEditingID, announcementDraftText, announcementSaving, announcementCount, latestAnnouncement, canManageAnnouncements, loadAnnouncements, openAnnouncementPanel, closeAnnouncementPanel, startCreateAnnouncement, startEditAnnouncement, cancelAnnouncementEditor, saveAnnouncementText, deleteAnnouncement, groupBusy, removeMember, changeRole, transferOwnership, leaveGroup, pinnedConversations, showsBadge, threadOpen, threadRoot, threadReplies, threadHasMorePages, hasThread, threadPreview, threadLabel, hiddenReplyCount, threadBusy, threadError, threadReplyCount, threadStream, openThread, closeThread, loadMoreThread, threadDraft, threadSending, threadDraftArea, sendInThread, refreshThread, onThreadEnter, hasThread, threadLabel, threadPreview, hiddenReplyCount, setPreference, togglePreference, currentPinned, currentMuted, openMembers, closeMembers, inviteFromMembers, globalQuery, globalResults, globalBusy, globalError, globalHasMore, globalStarted, showGlobalSearch, globalInput, openGlobalSearch, closeGlobalSearch, searchAll, openGlobalResult, anySender, isGenericFile, directoryQuery, directoryInput, directoryPlaceholder, directoryCount, filteredFriends, filteredGroups, filteredRecent, clearDirectorySearch, showSearch, searchInput, searchQuery, searchedQuery, searchResults, searchBusy, searchError, searchHasMore, searchStarted, locatingMessage, highlightedMessage, openSearch, closeSearch, changeSearch, searchMessages, locateMessage, highlightMatches, searchDate, me, mode, email, password, name, error, users, friends, recentConversations, groupConversations, friendCandidates, sortedFriends, friendQuery, conversationFilter, active, messages, pendingSend, draft, groupName, inviteUsers, inviteCandidates, inviteQuery, inviteInitial, inviteVisible, inviteGroups, inviteInitials, inviteSummary, inviteSelectedCount, inviteSelectedPeople, chooseInviteInitial, toggleInvite, selectAllInvites, clearInvites, resetInviteFilters, OTHER_INITIAL, conversationBusy, friendBusy, showCreate, showAddUser, showInvite, showSettings, showProfile, showFriendRequests, showRequestComposer, friendRequests, pendingIncomingRequests, friendRequestFor, friendRequestStatus, friendRequestBusy, requestMessage, requestTarget, profileEditing, profileBusy, profileName, profileEmail, passwordEditing, passwordBusy, currentPassword, newPassword, confirmPassword, startPasswordChange, changePassword, connected, loading, sending, stream, current, title, sender, time, directConversation, preview, contactPreview, unreadLabel, hasUnread, authenticate, logout, openCreate, openAddUser, openFriendRequests, openProfile, updateProfile, addFriend, sendFriendRequest, decideFriendRequest, openDirect, openInvite, addMembers, createGroup, select, send, loadOlder, onEnter, discardPending, mediaInput, uploadBusy, attachment, previewFile, mediaURL, formatBytes, openMediaPicker, chooseMedia, clearAttachment, composerImages, composerHasContent, removeComposerImage, clearComposerImages, syncDraftFromDom, onComposerInput, onComposerKeydown, onComposerClick, onComposerBlur, restorePending, dragging, onComposerPaste, onDragOver, onDragLeave, onDrop, draftArea, startReply, replyPreview, replyAuthor, bodyOf, segmentsOf, usesBlocks, recalledText, canRecallMessage, recallMessage, showMentions, mentionList, mentionIndex, mentionQuery, chooseMention, mentionKeydown, updateMentionState }
  },
  template,
}).mount('#app')
