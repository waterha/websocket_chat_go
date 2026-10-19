import { createApp, ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue/dist/vue.esm-bundler.js'
import { Search, X, ArrowRight, RotateCw } from 'lucide-vue-next'
import { highlightMatches } from './message-search.js'
import { filterDirectory, friendSearchFields, conversationSearchFields } from './directory-search.js'
import { appendPage, PendingSends, Requests, SyncTasks } from './realtime.js'
import { api } from './api.js'
import './style.css'

createApp({
  components: { Search, X, ArrowRight, RotateCw },
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
    const title = item => item.kind === 'direct'
      ? item.members.find(member => member.id !== me.value?.id)?.name || '私聊'
      : item.name
    const sender = id => current.value?.members.find(member => member.id === id)?.name || '成员'
    const time = value => new Date(value).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
    const preview = item => latestMessage(item)?.content || '暂无消息'
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
      active.value = id
      markSeen(id, cache.get(id)?.at(-1)?.seq || 0)
      pendingSend.value = pendingStore?.get(id) || null
      visibleCount.value = 100
      showMessages(id)
      try {
        await syncConversation(id)
        if (active.value === id) await scroll()
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
          if (envelope.event === 'message.sync_required') await catchUp(envelope.data.cursors)
          if (envelope.event === 'conversation.created') await refresh()
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
      if (!content || !id || sending.value) return
      sending.value = true
      try {
        const input = pendingStore.save(id, content)
        pendingSend.value = input
        if (connected.value) {
          try { await requests.send('message.send', input) }
          catch (cause) {
            if (version !== epoch) return
            if (cause.application) throw cause
            await api(`/conversations/${id}/messages`, { method: 'POST', body: JSON.stringify({ client_msg_id: input.client_msg_id, content }) })
          }
        } else await api(`/conversations/${id}/messages`, { method: 'POST', body: JSON.stringify({ client_msg_id: input.client_msg_id, content }) })
        if (version !== epoch) return
        pendingStore.clear(id, input.client_msg_id)
        if (pendingSend.value?.client_msg_id === input.client_msg_id) pendingSend.value = null
        if (active.value === id && draft.value.trim() === content) draft.value = ''
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
    onUnmounted(() => { resetSearch(); clearTimeout(highlightTimer); disposed = true; epoch++; requests?.close(); socket?.close(); clearTimeout(reconnectTimer); clearInterval(refreshTimer); clearTimeout(errorTimer); document.removeEventListener('visibilitychange', onVisible); window.removeEventListener('focus', onVisible) })
    return { directoryQuery, directoryInput, directoryPlaceholder, directoryCount, filteredFriends, filteredGroups, filteredRecent, clearDirectorySearch, showSearch, searchInput, searchQuery, searchedQuery, searchResults, searchBusy, searchError, searchHasMore, searchStarted, locatingMessage, highlightedMessage, openSearch, closeSearch, changeSearch, searchMessages, locateMessage, highlightMatches, searchDate, me, mode, email, password, name, error, users, friends, recentConversations, groupConversations, friendCandidates, sortedFriends, friendQuery, conversationFilter, active, messages, pendingSend, draft, groupName, inviteUsers, inviteCandidates, conversationBusy, friendBusy, showCreate, showAddUser, showInvite, showSettings, showProfile, showFriendRequests, showRequestComposer, friendRequests, pendingIncomingRequests, friendRequestFor, friendRequestStatus, friendRequestBusy, requestMessage, requestTarget, profileEditing, profileBusy, profileName, profileEmail, connected, loading, sending, stream, current, title, sender, time, directConversation, preview, contactPreview, unreadLabel, hasUnread, authenticate, logout, openCreate, openAddUser, openFriendRequests, openProfile, updateProfile, addFriend, sendFriendRequest, decideFriendRequest, openDirect, openInvite, addMembers, createGroup, select, send, loadOlder, onEnter }
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
          <div class="side-title"><span>{{ conversationFilter === 'current' ? '当前会话' : conversationFilter === 'friends' ? '好友' : '群聊' }}</span><div class="side-actions"><button @click="openAddUser" title="添加用户">添加</button><button @click="openFriendRequests" title="好友申请">申请<span v-if="pendingIncomingRequests.length" class="request-count">{{ pendingIncomingRequests.length > 99 ? '99+' : pendingIncomingRequests.length }}</span></button><button @click="openCreate" title="创建群聊">建群</button></div></div>
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
          <header class="chat-head"><span class="avatar">{{ title(current).slice(0,1) }}</span><div><h2>{{ title(current) }}</h2><p>{{ current.kind === 'group' ? current.members.length + ' 位成员' : '私聊' }} · {{ current.members.slice(0,5).map(member => member.name).join('、') }}{{ current.members.length > 5 ? ' 等 ' + current.members.length + ' 人' : '' }}</p></div><button type="button" class="icon-button search-toggle" title="搜索聊天记录" aria-label="搜索聊天记录" :aria-expanded="showSearch" aria-controls="message-search-panel" @click="showSearch ? closeSearch() : openSearch()"><Search :size="20" aria-hidden="true" /></button><button v-if="current.kind === 'group'" class="outline-button" @click="openInvite">邀请成员</button></header>
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
              <div><div class="message-meta">{{ sender(item.sender_id) }} · {{ time(item.created_at) }}</div><div class="bubble">{{ item.content }}</div></div></template>
            </div>
          </section>
          <form v-show="!showSearch" class="composer" @submit.prevent="send"><p v-if="pendingSend?.conversation_id === active" class="pending-notice">发送结果未确认，消息可能已保存。<button type="button" @click="draft = pendingSend.content">恢复原文安全重试</button><button type="button" @click="discardPending">放弃确认</button></p><textarea v-model="draft" maxlength="4000" placeholder="输入消息…" @keydown.enter.exact="onEnter"></textarea><div><span>Enter 发送 · Shift + Enter 换行</span><button class="primary" :disabled="!draft.trim() || sending">{{ sending ? '发送中…' : '发送 ↗' }}</button></div></form>
        </template>
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
            <div v-if="showInvite" class="modal-backdrop" @click.self="showInvite = false"><div class="modal"><div class="modal-head"><h2>邀请成员</h2><button @click="showInvite = false">×</button></div><p class="subtle">选择要加入“{{ current?.name }}”的用户，群聊最多容纳 1000 人，当前 {{ current?.members.length }} 人。</p><div class="user-list"><label v-for="person in inviteCandidates" :key="person.id"><input v-model="inviteUsers" type="checkbox" :value="person.id" :disabled="(current?.members.length || 0) + inviteUsers.length >= 1000 && !inviteUsers.includes(person.id)" /><span class="avatar tiny">{{ person.name.slice(0,1) }}</span><span>{{ person.name }}<small>{{ person.email }}</small></span></label><p v-if="!inviteCandidates.length">暂无可邀请的用户。</p></div><button class="primary" :disabled="conversationBusy || !inviteUsers.length || (current?.members.length || 0) + inviteUsers.length > 1000" @click="addMembers">{{ conversationBusy ? '邀请中…' : '邀请加入群聊' }}</button></div></div>
      <div v-if="showProfile" class="modal-backdrop" @click.self="showProfile = false"><div class="modal profile-modal" role="dialog" aria-modal="true" aria-label="个人信息"><div class="modal-head"><h2>个人信息</h2><button @click="showProfile = false" aria-label="关闭">×</button></div><div class="profile-header"><span class="avatar profile-avatar self">{{ (profileEditing ? profileName : me.name).slice(0,1) }}</span><div><h3>{{ me.name }}</h3><p class="subtle">个人资料</p></div></div><template v-if="profileEditing"><div class="profile-form"><label>用户名<input v-model="profileName" maxlength="64" autocomplete="name" /></label><label>用户账号<input v-model="profileEmail" type="email" maxlength="254" autocomplete="email" /></label></div><div class="profile-actions"><button class="outline-button" type="button" :disabled="profileBusy" @click="profileEditing = false">取消</button><button class="primary" type="button" :disabled="profileBusy || !profileName.trim() || !profileEmail.trim()" @click="updateProfile">{{ profileBusy ? "保存中…" : "保存修改" }}</button></div></template><template v-else><dl class="profile-details"><div><dt>用户名</dt><dd>{{ me.name }}</dd></div><div><dt>用户账号</dt><dd>{{ me.email }}</dd></div><div><dt>用户 ID</dt><dd>{{ me.id }}</dd></div></dl><button class="primary profile-edit-button" type="button" @click="profileEditing = true">编辑个人信息</button></template></div></div>
    </div>
    <div v-if="error" class="toast" role="alert">{{ error }}</div>
  `,
}).mount('#app')
