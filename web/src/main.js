import { createApp, ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue/dist/vue.esm-bundler.js'
import { Search, X, ArrowRight, RotateCw, Users, ImagePlus, Paperclip, Download } from 'lucide-vue-next'
import { highlightMatches } from './message-search.js'
import { filterMembers } from './member-list.js'
import { filterDirectory, friendSearchFields, conversationSearchFields } from './directory-search.js'
import { toggleConversationSelection } from './conversation-selection.js'
import { appendPage, PendingSends, Requests, SyncTasks } from './realtime.js'
import { canChangeRole, canEditAnnouncement, canInvite, canLeave, canManageMembers, canRemoveMember, canTransferOwnership, roleLabel, roleOf } from './group.js'
import { activeMentionToken, applyMention, canRecall, mentionCandidates, messageBody, recalledLabel, replyExcerpt, replyLabel } from './reply.js'
import { formatBytes, mediaPreview, mediaURL, validateMediaFile } from './media.js'
import { api, uploadMedia } from './api.js'
import './style.css'

createApp({
  components: { Search, X, ArrowRight, RotateCw, Users, ImagePlus, Paperclip, Download },
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
    const conversationFilter = ref('current')
    const directoryQuery = ref('')
    const directoryInput = ref(null)
    const showCreate = ref(false)
    const showAddUser = ref(false)
    const showInvite = ref(false)
    const showMembers = ref(false)
    const memberQuery = ref('')
    const memberBusy = ref(false)
    const announcementDraft = ref('')
    const announcementEditing = ref(false)
    const groupBusy = ref(false)
    const draftArea = ref(null)
    const replyTarget = ref(null)
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
    const profileEditing = ref(false)
    const profileBusy = ref(false)
    const profileName = ref('')
    const profileEmail = ref('')
    const conversationBusy = ref(false)
    const friendBusy = ref(false)
    const connected = ref(false)
    const loading = ref(false)
    const stream = ref(null)
    const sending = ref(false)
    const mediaInput = ref(null)
    const uploadBusy = ref(false)
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
    const deviceID = sessionStorage.getItem('im.device') || crypto.randomUUID()
    sessionStorage.setItem('im.device', deviceID)
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
    const hasAnnouncement = computed(() => Boolean(current.value?.kind === 'group' && current.value?.announcement))
    function startAnnouncementEdit() {
      announcementDraft.value = current.value?.announcement || ''
      announcementEditing.value = true
    }
    async function saveAnnouncement() {
      if (groupBusy.value || !current.value) return
      const id = active.value
      const version = epoch
      groupBusy.value = true
      try {
        const result = await api(`/conversations/${id}/announcement`, { method: 'PATCH', body: JSON.stringify({ announcement: announcementDraft.value }) })
        if (version !== epoch) return
        const conversation = conversations.value.find(item => item.id === id)
        if (conversation) conversation.announcement = result.announcement
        announcementEditing.value = false
      } catch (cause) { if (version === epoch) showError(cause) }
      finally { if (version === epoch) groupBusy.value = false }
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
    function openMembers() {
      memberQuery.value = ''
      showMembers.value = true
    }
    function closeMembers() {
      showMembers.value = false
      memberQuery.value = ''
    }
    function inviteFromMembers() {
      closeMembers()
      openInvite()
    }
    const title = item => item.kind === 'direct'
      ? item.members.find(member => member.id !== me.value?.id)?.name || '私聊'
      : item.name
    const sender = id => current.value?.members.find(member => member.id === id)?.name || '成员'
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
    const seenKey = id => `im.seen.${me.value?.id}.${id}`
    const seenSeq = id => Number(localStorage.getItem(seenKey(id)) || 0)
    const markSeen = (id, seq) => {
      if (!me.value || !seq) return
      const previous = seenSeq(id)
      if (seq > previous) localStorage.setItem(seenKey(id), String(seq))
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
    // Reply, mention and recall helpers.
    const replyPreview = item => replyExcerpt(item)
    const replyAuthor = item => replyLabel(item, sender)
    const bodyOf = item => messageBody(item)
    const recalledText = item => recalledLabel(item, me.value?.id)
    const canRecallMessage = item => canRecall(item, me.value?.id, current.value)
    // Mention list is derived from the active "@" token in the composer.
    const showMentions = computed(() => mentionQuery.value !== null && current.value?.kind === 'group')
    function startReply(item) {
      replyTarget.value = item
      nextTick(() => draftArea.value?.focus())
    }
    function clearReply() {
      replyTarget.value = null
    }
    function updateMentionState() {
      const area = draftArea.value
      if (!area || current.value?.kind !== 'group') { mentionQuery.value = null; mentionList.value = []; return }
      const token = activeMentionToken(draft.value, area.selectionStart)
      if (!token) { mentionQuery.value = null; mentionList.value = []; return }
      mentionQuery.value = token
      mentionList.value = mentionCandidates(current.value?.members, me.value?.id, token.query).slice(0, 8)
      mentionIndex.value = 0
    }
    function chooseMention(person) {
      const token = mentionQuery.value
      if (!token || !person) return
      const applied = applyMention(draft.value, token, person.name)
      draft.value = applied.text
      mentionQuery.value = null
      mentionList.value = []
      nextTick(() => {
        const area = draftArea.value
        if (!area) return
        area.focus()
        area.setSelectionRange(applied.caret, applied.caret)
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
        if (replyTarget.value?.id === item.id) replyTarget.value = null
        await syncConversation(id)
      } catch (cause) { if (version === epoch) showError(cause) }
      finally { if (version === epoch) sending.value = false }
    }
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
      clearAttachment()
      clearReply()
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
      markSeen(id, cache.get(id)?.at(-1)?.seq || 0)
      pendingSend.value = pendingStore?.get(id) || null
      visibleCount.value = 100
      showMessages(id)
      try {
        await syncConversation(id)
        if (active.value === id && chatVisible.value) await scroll()
      } catch (cause) { showError(cause) }
    }
    const onVisible = () => { if (me.value) reconcile().catch(showError) }
    function connect() {
      if (disposed || !me.value) return
      const connection = new WebSocket(`${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/ws?device_id=${deviceID}`)
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
        pendingStore = new PendingSends(sessionStorage, me.value.id)
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
      showCreate.value = false; showAddUser.value = false; showInvite.value = false; showSettings.value = false; showProfile.value = false; showFriendRequests.value = false; showRequestComposer.value = false; requestTarget.value = null; friendRequests.value = { incoming: [], outgoing: [] }; directoryQuery.value = ''; conversationFilter.value = 'current'
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
      const content = draft.value.trim()
      const id = active.value
      const version = epoch
      const media = attachment.value
      if ((!content && !media) || !id || sending.value || uploadBusy.value) return
      sending.value = true
      try {
        const input = pendingStore.save(id, content, media?.id || '')
        pendingSend.value = input
        const payload = { client_msg_id: input.client_msg_id, content, media_id: media?.id || '', reply_to_id: replyTarget.value?.id || '', mentions: resolveMentions() }
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
        if (active.value === id && draft.value.trim() === content) draft.value = ''
        if (active.value === id) clearAttachment()
        if (active.value === id) clearReply()
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
      event.preventDefault()
      send()
    }
    onMounted(async () => {
      try { me.value = await api('/me'); pendingStore = new PendingSends(sessionStorage, me.value.id); await refresh(); if (active.value) await select(active.value) }
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
    })
    onUnmounted(() => { resetSearch(); clearTimeout(highlightTimer); releaseAttachment(); disposed = true; epoch++; requests?.close(); socket?.close(); clearTimeout(reconnectTimer); clearInterval(refreshTimer); clearTimeout(errorTimer); document.removeEventListener('visibilitychange', onVisible); window.removeEventListener('focus', onVisible) })
    return { showMembers, memberQuery, currentMembers, memberCount, memberBusy, currentRole, isGroupOwner, canInviteMembers, canManageGroupMembers, canEditGroupAnnouncement, canLeaveGroup, memberCanBeRemoved, memberCanChangeRole, canTransferTo, roleLabel, announcementDraft, announcementEditing, startAnnouncementEdit, saveAnnouncement, hasAnnouncement, groupBusy, removeMember, changeRole, transferOwnership, leaveGroup, openMembers, closeMembers, inviteFromMembers, globalQuery, globalResults, globalBusy, globalError, globalHasMore, globalStarted, showGlobalSearch, globalInput, openGlobalSearch, closeGlobalSearch, searchAll, openGlobalResult, anySender, isGenericFile, directoryQuery, directoryInput, directoryPlaceholder, directoryCount, filteredFriends, filteredGroups, filteredRecent, clearDirectorySearch, showSearch, searchInput, searchQuery, searchedQuery, searchResults, searchBusy, searchError, searchHasMore, searchStarted, locatingMessage, highlightedMessage, openSearch, closeSearch, changeSearch, searchMessages, locateMessage, highlightMatches, searchDate, me, mode, email, password, name, error, users, friends, recentConversations, groupConversations, friendCandidates, sortedFriends, friendQuery, conversationFilter, active, messages, pendingSend, draft, groupName, inviteUsers, inviteCandidates, conversationBusy, friendBusy, showCreate, showAddUser, showInvite, showSettings, showProfile, showFriendRequests, showRequestComposer, friendRequests, pendingIncomingRequests, friendRequestFor, friendRequestStatus, friendRequestBusy, requestMessage, requestTarget, profileEditing, profileBusy, profileName, profileEmail, connected, loading, sending, stream, current, title, sender, time, directConversation, preview, contactPreview, unreadLabel, hasUnread, authenticate, logout, openCreate, openAddUser, openFriendRequests, openProfile, updateProfile, addFriend, sendFriendRequest, decideFriendRequest, openDirect, openInvite, addMembers, createGroup, select, send, loadOlder, onEnter, mediaInput, uploadBusy, attachment, previewFile, mediaURL, formatBytes, openMediaPicker, chooseMedia, clearAttachment, draftArea, replyTarget, startReply, clearReply, replyPreview, replyAuthor, bodyOf, recalledText, canRecallMessage, recallMessage, showMentions, mentionList, mentionIndex, mentionQuery, chooseMention, mentionKeydown, updateMentionState }
  },
  template: `
    <div v-if="!me" class="auth-layout">
      <div class="auth-card">
        <div class="brand-mark">✦</div><p class="eyebrow">OPEN SOURCE · REAL-TIME</p>
        <h1>让沟通，自然发生。</h1><p class="subtle">websocket_chat_go · 开源即时通讯初版</p>
        <form @submit.prevent="authenticate">
          <label v-if="mode === 'register'">昵称<input v-model="name" required maxlength="64" placeholder="你的称呼" /></label>
          <label>邮箱<input v-model="email" type="email" required maxlength="254" placeholder="you@example.com" /></label>
          <label>密码<input v-model="password" type="password" required minlength="8" maxlength="72" placeholder="至少 8 位" /></label>
          <button class="primary" :disabled="loading">{{ mode === 'login' ? '登录' : '创建账号' }}</button>
        </form>
        <button class="text-button" @click="mode = mode === 'login' ? 'register' : 'login'">{{ mode === 'login' ? '还没有账号？立即注册' : '已有账号？返回登录' }}</button>
      </div>
    </div>
    <div v-else class="layout">
      <aside class="sidebar">
        <header class="sidebar-head"><span class="brand-mark small">✦</span><div><strong>websocket_chat_go</strong><small>让团队更靠近</small></div></header>
          <div class="side-title"><span>{{ conversationFilter === 'current' ? '当前会话' : conversationFilter === 'friends' ? '好友' : '群聊' }}</span><div class="side-actions"><button @click="openGlobalSearch" title="全局搜索">搜索</button><button @click="openAddUser" title="添加用户">添加</button><button @click="openFriendRequests" title="好友申请">申请<span v-if="pendingIncomingRequests.length" class="request-count">{{ pendingIncomingRequests.length > 99 ? '99+' : pendingIncomingRequests.length }}</span></button><button @click="openCreate" title="创建群聊">建群</button></div></div>
          <div class="conversation-tabs"><button :class="{ active: conversationFilter === 'current' }" @click="conversationFilter = 'current'">当前</button><button :class="{ active: conversationFilter === 'friends' }" @click="conversationFilter = 'friends'">好友</button><button :class="{ active: conversationFilter === 'group' }" @click="conversationFilter = 'group'">群聊</button></div>
          <div class="directory-search" role="search" aria-label="查找好友和群聊"><Search :size="17" aria-hidden="true" /><input ref="directoryInput" v-model="directoryQuery" type="search" maxlength="100" :placeholder="directoryPlaceholder" :aria-label="directoryPlaceholder" @keydown.esc.prevent="clearDirectorySearch" /><button v-if="directoryQuery" type="button" class="icon-button" title="清除搜索" aria-label="清除列表搜索" @click="clearDirectorySearch"><X :size="16" aria-hidden="true" /></button></div>
          <div class="conversation-list">
             <p v-if="directoryQuery.trim() && !directoryCount" class="empty-side" role="status">{{ conversationFilter === 'friends' ? '没有匹配的好友' : conversationFilter === 'group' ? '没有匹配的群聊' : '没有匹配的当前会话' }}</p>
            <template v-if="conversationFilter === 'friends'">
              <button v-for="person in filteredFriends" :key="person.id" class="conversation contact" @click="openDirect(person.id)">
                <span class="avatar">{{ person.name.slice(0, 1) }}</span><span class="conversation-text"><strong>{{ person.name }}</strong><small>{{ contactPreview(person) }}</small></span><span v-if="directConversation(person.id) && hasUnread(directConversation(person.id).id)" class="unread-badge">{{ unreadLabel(directConversation(person.id).id) }}</span>
              </button>
            </template>
            <template v-else-if="conversationFilter === 'group'">
              <button v-for="item in filteredGroups" :key="item.id" class="conversation" :class="{ active: active === item.id }" @click="select(item.id)">
                <span class="avatar">{{ title(item).slice(0, 1) }}</span><span class="conversation-text"><strong>{{ title(item) }}</strong><small>{{ preview(item) }}</small></span><span v-if="hasUnread(item.id)" class="unread-badge">{{ unreadLabel(item.id) }}</span>
              </button>
            </template>
            <template v-else>
              <button v-for="item in filteredRecent" :key="item.id" class="conversation" :class="{ active: active === item.id }" @click="select(item.id)">
                <span class="avatar">{{ title(item).slice(0, 1) }}</span><span class="conversation-text"><strong>{{ title(item) }}</strong><small>{{ preview(item) }}</small></span><span v-if="hasUnread(item.id)" class="unread-badge">{{ unreadLabel(item.id) }}</span>
              </button>
            </template>
            <p v-if="conversationFilter === 'friends' && !friends.length && !directoryQuery.trim()" class="empty-side">还没有好友，点击“添加”开始。</p>
            <p v-else-if="conversationFilter === 'group' && !groupConversations.length && !directoryQuery.trim()" class="empty-side">还没有加入群聊。</p>
            <p v-else-if="conversationFilter === 'current' && !recentConversations.length && !directoryQuery.trim()" class="empty-side">还没有最近会话。</p>
         </div>
        <footer class="sidebar-footer"><span class="avatar self">{{ me.name.slice(0,1) }}</span><span class="identity"><strong>{{ me.name }}</strong><small>{{ connected ? '实时连接已建立' : '正在重连 · 历史可用' }}</small></span><button @click="showSettings = !showSettings" title="设置" aria-haspopup="menu" :aria-expanded="showSettings">设置</button><div v-if="showSettings" class="settings-menu" role="menu"><button type="button" role="menuitem" @click="openProfile">个人</button><button type="button" role="menuitem" @click="logout">退出</button></div></footer>
      </aside>
      <main class="chat">
        <template v-if="current">
          <header class="chat-head"><span class="avatar">{{ title(current).slice(0,1) }}</span><div><h2>{{ title(current) }}</h2><p v-if="current.kind === 'group'"><button type="button" class="member-count-button" @click="openMembers" :aria-haspopup="dialog" aria-label="查看群成员">{{ current.members.length }} 位成员</button> · {{ current.members.slice(0,5).map(member => member.name).join('、') }}{{ current.members.length > 5 ? ' 等 ' + current.members.length + ' 人' : '' }}</p><p v-else>私聊 · {{ current.members.slice(0,5).map(member => member.name).join('、') }}</p></div><button type="button" class="icon-button search-toggle" title="搜索聊天记录" aria-label="搜索聊天记录" :aria-expanded="showSearch" aria-controls="message-search-panel" @click="showSearch ? closeSearch() : openSearch()"><Search :size="20" aria-hidden="true" /></button><button v-if="current.kind === 'group'" type="button" class="icon-button member-toggle" title="群成员列表" aria-label="群成员列表" :aria-expanded="showMembers" @click="openMembers"><Users :size="20" aria-hidden="true" /></button><button v-if="canInviteMembers" class="outline-button" @click="openInvite">邀请成员</button><button v-else-if="current.kind === 'group'" class="outline-button leave-button" :disabled="groupBusy" @click="leaveGroup">退出群聊</button></header>            <section v-if="hasAnnouncement && !announcementEditing" class="announcement" role="status" aria-label="群公告"><strong>群公告</strong><p>{{ current.announcement }}</p><button v-if="canEditGroupAnnouncement" type="button" class="icon-button" title="编辑群公告" aria-label="编辑群公告" @click="startAnnouncementEdit">编辑</button></section>
            <section v-if="announcementEditing" class="announcement editing" aria-label="编辑群公告"><strong>群公告</strong><textarea v-model="announcementDraft" maxlength="2000" placeholder="填写群公告，成员都会在会话顶部看到"></textarea><div class="announcement-actions"><button type="button" class="outline-button" :disabled="groupBusy" @click="announcementEditing = false">取消</button><button type="button" class="primary" :disabled="groupBusy" @click="saveAnnouncement">{{ groupBusy ? '保存中…' : '保存公告' }}</button></div></section>
            <section v-else-if="canEditGroupAnnouncement && !hasAnnouncement" class="announcement empty"><button type="button" class="text-button" @click="startAnnouncementEdit">+ 添加群公告</button></section>
           <section v-if="showSearch" id="message-search-panel" class="message-search" aria-label="搜索聊天记录" @keydown.esc.stop="closeSearch">
             <form class="message-search-form" role="search" @submit.prevent="searchMessages()">
               <div class="message-search-field"><Search :size="18" aria-hidden="true" /><input ref="searchInput" v-model="searchQuery" type="search" maxlength="100" placeholder="搜索当前会话的聊天记录" aria-label="消息关键词" @input="changeSearch" /></div>
               <button type="submit" class="icon-button search-submit" title="搜索" aria-label="搜索" :disabled="!searchQuery.trim() || searchBusy"><Search :size="19" aria-hidden="true" /></button>
               <button type="button" class="icon-button" title="关闭搜索" aria-label="关闭搜索" @click="closeSearch"><X :size="20" aria-hidden="true" /></button>
             </form>
             <div class="message-search-status" role="status" aria-live="polite"><span v-if="searchBusy">正在搜索…</span><span v-else-if="searchError" class="search-error">{{ searchError }}</span><span v-else-if="!searchStarted">搜索聊天记录</span><span v-else-if="!searchResults.length">没有找到匹配的消息</span><span v-else>已找到 {{ searchResults.length }} 条{{ searchHasMore ? '以上' : '' }}匹配消息</span><button v-if="searchError" type="button" class="icon-button" title="重试搜索" aria-label="重试搜索" @click="searchMessages(searchResults.length > 0)"><RotateCw :size="17" aria-hidden="true" /></button></div>
             <div class="message-search-results" :aria-busy="searchBusy">
               <button v-for="item in searchResults" :key="item.id" type="button" class="message-search-result" :disabled="locatingMessage !== null" :aria-label="'定位消息：' + item.content" @click="locateMessage(item)">
                 <span class="search-result-meta"><strong>{{ item.kind === 'system' ? '系统消息' : sender(item.sender_id) }}</strong><time :datetime="item.created_at">{{ searchDate(item.created_at) }}</time></span>
                 <span class="search-result-content"><template v-for="(part, index) in highlightMatches(item.content, searchedQuery)" :key="index"><mark v-if="part.match">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template></span>
                 <span class="search-result-action">{{ locatingMessage === item.seq ? '定位中…' : '查看原消息' }}<ArrowRight :size="15" aria-hidden="true" /></span>
               </button>
               <button v-if="searchHasMore && !searchError" type="button" class="older search-more" :disabled="searchBusy" @click="searchMessages(true)">{{ searchBusy ? '加载中…' : '加载更多结果' }}</button>
             </div>
           </section>
           <section v-show="!showSearch" ref="stream" class="stream" aria-label="聊天记录">
            <button v-if="messages[0]?.seq > 1" class="older" @click="loadOlder">加载更早的消息</button>
            <div v-if="!messages.length" class="empty-chat">说声你好，开始这段对话。</div>
            <div v-for="item in messages" :key="item.id" class="message-row" :data-message-seq="item.seq" :class="{ mine: item.sender_id === me.id, system: item.kind === 'system', 'search-target': highlightedMessage === item.seq }">
              <template v-if="item.kind === 'system'"><span class="system-notice">{{ item.content }}</span></template>
              <template v-else><span v-if="item.sender_id !== me.id" class="avatar tiny">{{ sender(item.sender_id).slice(0,1) }}</span>
              <div><div class="message-meta">{{ sender(item.sender_id) }} · {{ time(item.created_at) }}</div><div v-if="item.reply" class="reply-quote"><strong>{{ replyAuthor(item.reply) }}</strong><span>{{ replyPreview(item.reply) }}</span></div><div class="bubble" :class="{ media: item.kind === 'image' || item.kind === 'video', recalled: item.recalled_at }"><template v-if="item.recalled_at"><span class="recalled">{{ recalledText(item) }}</span></template><template v-else-if="item.kind === 'image'"><a class="media-item" :href="mediaURL(item.media_id)" target="_blank" rel="noopener" :aria-label="'查看图片 ' + item.media.file_name"><img class="media-image" :src="mediaURL(item.media_id)" :alt="item.media.file_name" loading="lazy" /></a><p v-if="bodyOf(item)" class="media-caption">{{ bodyOf(item) }}</p></template><template v-else-if="item.kind === 'video'"><video class="media-video" :src="mediaURL(item.media_id)" controls preload="metadata" :aria-label="'播放视频 ' + item.media.file_name"></video><p v-if="bodyOf(item)" class="media-caption">{{ bodyOf(item) }}</p></template><template v-else-if="item.kind === 'file'"><a class="media-file" :href="mediaURL(item.media_id)" :download="item.media.file_name"><Paperclip :size="18" aria-hidden="true" /><span class="media-file-text"><strong>{{ item.media.file_name }}</strong><small>{{ formatBytes(item.media.byte_size) }}</small></span><Download :size="16" aria-hidden="true" /></a><p v-if="bodyOf(item)" class="media-caption">{{ bodyOf(item) }}</p></template><template v-else>{{ bodyOf(item) }}</template></div><div v-if="canRecallMessage(item) || (item.kind !== 'system' && !item.recalled_at)" class="message-actions"><button type="button" class="text-button" @click="startReply(item)">回复</button><button v-if="canRecallMessage(item)" type="button" class="text-button danger" :disabled="sending" @click="recallMessage(item)">撤回</button></div></div></template>
            </div>
          </section>
          <form v-show="!showSearch" class="composer" @submit.prevent="send"><p v-if="pendingSend?.conversation_id === active" class="pending-notice">发送结果未确认，消息可能已保存。<button type="button" @click="draft = pendingSend.content">恢复原文安全重试</button><button type="button" @click="discardPending">放弃确认</button></p><div v-if="replyTarget" class="reply-banner"><span class="reply-banner-text"><strong>回复 {{ sender(replyTarget.sender_id) }}</strong><small>{{ replyPreview(replyTarget) }}</small></span><button type="button" class="icon-button" title="取消回复" aria-label="取消回复" @click="clearReply"><X :size="16" aria-hidden="true" /></button></div><div v-if="uploadBusy" class="attachment-status" role="status">正在上传…</div><div v-else-if="attachment" class="attachment-preview"><img v-if="attachment.media_type === 'image'" :src="previewFile" :alt="attachment.file_name" /><video v-else-if="attachment.media_type === 'video'" :src="previewFile" muted playsinline preload="metadata"></video><span v-else class="attachment-file-icon"><Paperclip :size="22" aria-hidden="true" /></span><span class="attachment-info"><strong>{{ attachment.file_name }}</strong><small>{{ attachment.media_type === 'video' ? '视频' : attachment.media_type === 'image' ? '图片' : '文件' }} · {{ formatBytes(attachment.byte_size) }}</small></span><button type="button" class="icon-button" title="移除附件" aria-label="移除附件" :disabled="sending" @click="clearAttachment"><X :size="16" aria-hidden="true" /></button></div><textarea ref="draftArea" v-model="draft" maxlength="4000" :placeholder="attachment ? '添加说明（可选）…' : current.kind === 'group' ? '输入消息，@ 可提及成员…' : '输入消息…'" @keydown.enter.exact="onEnter" @keydown="mentionKeydown($event) && $event.preventDefault()" @keyup="updateMentionState" @click="updateMentionState" @input="updateMentionState"></textarea><div v-if="showMentions && mentionList.length" class="mention-list" role="listbox" aria-label="提及成员"><button v-for="(person, index) in mentionList" :key="person.id" type="button" role="option" :aria-selected="index === mentionIndex" :class="{ active: index === mentionIndex }" @mousedown.prevent="chooseMention(person)"><span class="avatar tiny">{{ person.name.slice(0,1) }}</span><span>{{ person.name }}<small>{{ person.email }}</small></span></button><p v-if="!mentionList.length" class="empty-mention">没有匹配的成员</p></div><div><span>Enter 发送 · Shift + Enter 换行</span><input ref="mediaInput" class="media-input" type="file" @change="chooseMedia" /><button type="button" class="icon-button media-button" title="发送图片、视频或文件" aria-label="发送图片、视频或文件" :disabled="uploadBusy || sending" @click="openMediaPicker"><Paperclip :size="19" aria-hidden="true" /></button><button class="primary" :disabled="(!draft.trim() && !attachment) || sending || uploadBusy">{{ sending ? '发送中…' : '发送 ↗' }}</button></div></form>        </template>
         <div v-else class="welcome"><span class="brand-mark">✦</span><h2>开启一段新对话</h2><p>从左侧联系人选择用户即可开始私聊，也可以创建群聊。</p><div class="welcome-actions"><button class="outline-button" @click="openCreate()">创建群聊</button></div></div>
      </main>
      <div v-if="showCreate" class="modal-backdrop" @click.self="showCreate = false">
        <div class="modal">
           <div class="modal-head"><h2>创建群聊</h2><button @click="showCreate = false">×</button></div>
           <p class="subtle">创建后群内只有你自己，之后可在群内按需邀请其他用户。含自己最多 1000 人。</p>
           <input v-model="groupName" maxlength="100" placeholder="群聊名称" />
           <button class="primary" :disabled="conversationBusy || !groupName.trim()" @click="createGroup">{{ conversationBusy ? '处理中…' : '创建群聊' }}</button>
         </div>
       </div>
      <div v-if="showAddUser" class="modal-backdrop" @click.self="showAddUser = false"><div class="modal"><div class="modal-head"><h2>添加用户</h2><button @click="showAddUser = false">×</button></div><p class="subtle">通过用户名或用户账号搜索，点击发送申请后再填写申请信息。</p><input v-model="friendQuery" type="search" placeholder="搜索用户名或账号" aria-label="搜索用户名或账号" /><div class="user-list"><label v-for="person in friendCandidates" :key="person.id"><span class="avatar tiny">{{ person.name.slice(0,1) }}</span><span>{{ person.name }}<small>账号：{{ person.email }}</small></span><button type="button" class="small-button" :disabled="friendBusy" @click="addFriend(person.id)">{{ friendRequestFor(person.id) ? '已发送申请' : friendBusy ? '处理中…' : '发送申请' }}</button></label><p v-if="!friendCandidates.length">没有匹配的用户。</p></div></div></div>
<div v-if="showRequestComposer" class="modal-backdrop" @click.self="showRequestComposer = false"><div class="modal request-composer"><div class="modal-head"><h2>发送好友申请</h2><button @click="showRequestComposer = false">×</button></div><div class="request-recipient"><span class="avatar tiny">{{ requestTarget?.name?.slice(0,1) }}</span><div><strong>{{ requestTarget?.name }}</strong><small>账号：{{ requestTarget?.email }}</small></div></div><p class="subtle">申请信息可选，填写后点击发送才会提交好友申请。</p><textarea v-model="requestMessage" class="request-message" maxlength="500" placeholder="请输入申请信息（可选）"></textarea><button class="primary" :disabled="friendBusy" @click="sendFriendRequest">{{ friendBusy ? '发送中…' : '发送好友申请' }}</button></div></div>
      <div v-if="showFriendRequests" class="modal-backdrop" @click.self="showFriendRequests = false"><div class="modal friend-requests-modal"><div class="modal-head"><h2>好友申请</h2><button @click="showFriendRequests = false">×</button></div><section><h3 class="request-section-title">收到的申请</h3><div class="request-list"><div v-for="item in friendRequests.incoming" :key="item.id" class="request-item"><div class="request-main"><strong>{{ item.requester.name }}</strong><small>{{ item.message || '请求添加你为好友' }}</small><time>{{ time(item.created_at) }}</time></div><div class="request-actions"><button class="small-button" :disabled="friendRequestBusy" @click="decideFriendRequest(item.id, 'accept')">同意</button><button class="small-button reject-button" :disabled="friendRequestBusy" @click="decideFriendRequest(item.id, 'reject')">拒绝</button></div></div><p v-if="!friendRequests.incoming.length" class="empty-request">暂无新的好友申请。</p></div></section><section><h3 class="request-section-title">发出的申请</h3><div class="request-list"><div v-for="item in friendRequests.outgoing" :key="item.id" class="request-item"><div class="request-main"><strong>{{ item.recipient.name }}</strong><small>{{ item.message || '未填写申请信息' }}</small><time>{{ time(item.updated_at) }}</time></div><span class="request-status" :class="'status-' + item.status">{{ friendRequestStatus(item.status) }}</span></div><p v-if="!friendRequests.outgoing.length" class="empty-request">暂无发出的申请。</p></div></section></div></div>
            <div v-if="showMembers" class="modal-backdrop" @click.self="closeMembers"><div class="modal member-modal" role="dialog" aria-modal="true" aria-label="群成员列表"><div class="modal-head"><h2>群成员<span class="member-total">{{ memberCount }} 人</span></h2><button @click="closeMembers" aria-label="关闭">×</button></div><p class="subtle">{{ title(current) }} · 你的身份：{{ roleLabel(currentRole) }} · 群成员上限 1000 人</p><div class="member-search"><Search :size="17" aria-hidden="true" /><input v-model="memberQuery" type="search" maxlength="100" placeholder="搜索群成员昵称或账号" aria-label="搜索群成员" /></div><div class="member-list" role="list"><div v-for="person in currentMembers" :key="person.id" class="member-item" role="listitem"><span class="avatar tiny">{{ person.name.slice(0,1) }}</span><span class="member-text"><strong>{{ person.name }}<span v-if="person.id === me.id" class="member-self">我</span><span v-if="person.role !== 'member'" class="role-badge" :class="'role-' + person.role">{{ roleLabel(person.role) }}</span></strong><small>账号：{{ person.email }}</small></span><span v-if="memberCanChangeRole(person) || canTransferTo(person) || memberCanBeRemoved(person)" class="member-row-actions"><button v-if="memberCanChangeRole(person)" type="button" class="small-button" :disabled="memberBusy" @click="changeRole(person, person.role === 'admin' ? 'member' : 'admin')">{{ person.role === 'admin' ? '取消管理员' : '设为管理员' }}</button><button v-if="canTransferTo(person)" type="button" class="small-button" :disabled="memberBusy" @click="transferOwnership(person)">转让群主</button><button v-if="memberCanBeRemoved(person)" type="button" class="small-button reject-button" :disabled="memberBusy" @click="removeMember(person)">移出</button></span></div><p v-if="!currentMembers.length" class="empty-member">{{ memberQuery.trim() ? '没有匹配的成员' : '暂无群成员' }}</p></div><div class="member-actions"><button class="outline-button" type="button" @click="closeMembers">关闭</button><button v-if="canLeaveGroup" class="outline-button leave-button" type="button" :disabled="groupBusy" @click="leaveGroup">退出群聊</button><button v-if="canInviteMembers" class="primary" type="button" :disabled="memberCount >= 1000" @click="inviteFromMembers">{{ memberCount >= 1000 ? '成员已满' : '邀请成员' }}</button></div></div></div>
      <div v-if="showGlobalSearch" class="modal-backdrop" @click.self="closeGlobalSearch"><div class="modal global-search-modal" role="dialog" aria-modal="true" aria-label="全局搜索"><div class="modal-head"><h2>全局搜索</h2><button @click="closeGlobalSearch" aria-label="关闭">×</button></div><p class="subtle">搜索你参与的全部会话，包含系统提示，不包含已撤回的消息。</p><form class="message-search-form" role="search" @submit.prevent="searchAll()"><div class="message-search-field"><Search :size="18" aria-hidden="true" /><input ref="globalInput" v-model="globalQuery" type="search" maxlength="100" placeholder="搜索所有会话的消息" aria-label="全局搜索关键词" /></div><button type="submit" class="primary" :disabled="!globalQuery.trim() || globalBusy">{{ globalBusy ? '搜索中…' : '搜索' }}</button></form><div class="message-search-status" role="status" aria-live="polite"><span v-if="globalBusy">正在搜索…</span><span v-else-if="globalError" class="search-error">{{ globalError }}</span><span v-else-if="!globalStarted">输入关键词后开始搜索。</span><span v-else-if="!globalResults.length">没有找到匹配的消息。</span><span v-else>已找到 {{ globalResults.length }} 条{{ globalHasMore ? '以上' : '' }}匹配消息</span></div><div class="message-search-results" :aria-busy="globalBusy"><button v-for="hit in globalResults" :key="hit.message.id" type="button" class="message-search-result" @click="openGlobalResult(hit)"><span class="search-result-meta"><strong>{{ hit.conversation_name || (hit.conversation_kind === 'direct' ? '私聊' : '群聊') }}</strong><span class="search-result-conversation">{{ hit.conversation_kind === 'group' ? '群聊 · ' + hit.member_count + ' 人' : '私聊' }} · {{ anySender(hit.message.sender_id) }}</span><time :datetime="hit.message.created_at">{{ searchDate(hit.message.created_at) }}</time></span><span class="search-result-content"><template v-for="(part, index) in highlightMatches(hit.message.content, globalQuery)" :key="index"><mark v-if="part.match">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template></span><span class="search-result-action">查看原消息<ArrowRight :size="15" aria-hidden="true" /></span></button><button v-if="globalHasMore && !globalError" type="button" class="older search-more" :disabled="globalBusy" @click="searchAll(true)">{{ globalBusy ? '加载中…' : '加载更多结果' }}</button></div></div></div>
      <div v-if="showInvite" class="modal-backdrop" @click.self="showInvite = false"><div class="modal"><div class="modal-head"><h2>邀请成员</h2><button @click="showInvite = false">×</button></div><p class="subtle">选择要加入“{{ current?.name }}”的用户，群聊最多容纳 1000 人，当前 {{ current?.members.length }} 人。</p><div class="user-list"><label v-for="person in inviteCandidates" :key="person.id"><input v-model="inviteUsers" type="checkbox" :value="person.id" :disabled="(current?.members.length || 0) + inviteUsers.length >= 1000 && !inviteUsers.includes(person.id)" /><span class="avatar tiny">{{ person.name.slice(0,1) }}</span><span>{{ person.name }}<small>{{ person.email }}</small></span></label><p v-if="!inviteCandidates.length">暂无可邀请的用户。</p></div><button class="primary" :disabled="conversationBusy || !inviteUsers.length || (current?.members.length || 0) + inviteUsers.length > 1000" @click="addMembers">{{ conversationBusy ? '邀请中…' : '邀请加入群聊' }}</button></div></div>
      <div v-if="showProfile" class="modal-backdrop" @click.self="showProfile = false"><div class="modal profile-modal" role="dialog" aria-modal="true" aria-label="个人信息"><div class="modal-head"><h2>个人信息</h2><button @click="showProfile = false" aria-label="关闭">×</button></div><div class="profile-header"><span class="avatar profile-avatar self">{{ (profileEditing ? profileName : me.name).slice(0,1) }}</span><div><h3>{{ me.name }}</h3><p class="subtle">个人资料</p></div></div><template v-if="profileEditing"><div class="profile-form"><label>用户名<input v-model="profileName" maxlength="64" autocomplete="name" /></label><label>用户账号<input v-model="profileEmail" type="email" maxlength="254" autocomplete="email" /></label></div><div class="profile-actions"><button class="outline-button" type="button" :disabled="profileBusy" @click="profileEditing = false">取消</button><button class="primary" type="button" :disabled="profileBusy || !profileName.trim() || !profileEmail.trim()" @click="updateProfile">{{ profileBusy ? "保存中…" : "保存修改" }}</button></div></template><template v-else><dl class="profile-details"><div><dt>用户名</dt><dd>{{ me.name }}</dd></div><div><dt>用户账号</dt><dd>{{ me.email }}</dd></div><div><dt>用户 ID</dt><dd>{{ me.id }}</dd></div></dl><button class="primary profile-edit-button" type="button" @click="profileEditing = true">编辑个人信息</button></template></div></div>
    </div>
    <div v-if="error" class="toast" role="alert">{{ error }}</div>
  `,
}).mount('#app')
