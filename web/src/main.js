import { createApp, ref, computed, onMounted, onUnmounted, nextTick } from 'vue/dist/vue.esm-bundler.js'
import { appendPage, PendingSends, Requests, SyncTasks } from './realtime.js'
import { api } from './api.js'
import './style.css'

createApp({
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
    const showCreate = ref(false)
    const showAddUser = ref(false)
    const showInvite = ref(false)
    const showSettings = ref(false)
    const showProfile = ref(false)
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
    const recentConversations = computed(() => {
      cacheVersion.value
      return conversations.value
        .filter(item => cache.get(item.id)?.length)
        .sort((left, right) => (cache.get(right.id)?.at(-1)?.seq || 0) - (cache.get(left.id)?.at(-1)?.seq || 0))
    })
    const groupConversations = computed(() => conversations.value.filter(item => item.kind === 'group'))
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
    const latestMessage = item => cache.get(item.id)?.at(-1)
    const preview = item => latestMessage(item)?.content || '暂无消息'
    const directConversation = userID => conversations.value.find(item => item.kind === 'direct' && item.members.some(member => member.id === userID))
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
        const [list, people, friendList] = await Promise.all([api('/conversations'), api('/users'), api('/friends')])
        if (version !== epoch || !me.value) return
        conversations.value = list
        users.value = people
        friends.value = friendList
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
           if (id === active.value && stick) await scroll()
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
      showCreate.value = false; showAddUser.value = false; showInvite.value = false; showSettings.value = false; showProfile.value = false; conversationFilter.value = 'current'
      try { await api('/auth/logout', { method: 'POST' }) } catch (cause) { showError(cause) }
    }
    function openCreate() {
      groupName.value = ''
      showCreate.value = true
    }
    function openAddUser() {
      friendQuery.value = ''
      showAddUser.value = true
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
    async function addFriend(userID) {
      if (friendBusy.value) return
      friendBusy.value = true
      try {
        await api('/friends', { method: 'POST', body: JSON.stringify({ user_id: userID }) })
        showAddUser.value = false
        await refresh()
      } catch (cause) { showError(cause) }
      finally { friendBusy.value = false }
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
          else await reconcile()
        } catch (cause) { showError(cause) }
      }, 5000)
      document.addEventListener('visibilitychange', onVisible)
      window.addEventListener('focus', onVisible)
    })
    onUnmounted(() => { disposed = true; epoch++; requests?.close(); socket?.close(); clearTimeout(reconnectTimer); clearInterval(refreshTimer); clearTimeout(errorTimer); document.removeEventListener('visibilitychange', onVisible); window.removeEventListener('focus', onVisible) })
    return { me, mode, email, password, name, error, users, friends, recentConversations, groupConversations, friendCandidates, friendQuery, conversationFilter, active, messages, pendingSend, draft, groupName, inviteUsers, inviteCandidates, conversationBusy, friendBusy, showCreate, showAddUser, showInvite, showSettings, showProfile, profileEditing, profileBusy, profileName, profileEmail, connected, loading, sending, stream, current, title, sender, time, directConversation, preview, contactPreview, unreadLabel, hasUnread, authenticate, logout, openCreate, openAddUser, openProfile, updateProfile, openDirect, openInvite, addFriend, addMembers, createGroup, select, send, loadOlder, onEnter }
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
          <div class="side-title"><span>{{ conversationFilter === 'current' ? '当前会话' : conversationFilter === 'friends' ? '好友' : '群聊' }}</span><div class="side-actions"><button @click="openAddUser" title="添加用户">添加</button><button @click="openCreate" title="创建群聊">建群</button></div></div>
          <div class="conversation-tabs"><button :class="{ active: conversationFilter === 'current' }" @click="conversationFilter = 'current'">当前</button><button :class="{ active: conversationFilter === 'friends' }" @click="conversationFilter = 'friends'">好友</button><button :class="{ active: conversationFilter === 'group' }" @click="conversationFilter = 'group'">群聊</button></div>
          <div class="conversation-list">
            <template v-if="conversationFilter === 'friends'">
              <button v-for="person in friends" :key="person.id" class="conversation contact" @click="openDirect(person.id)">
                <span class="avatar">{{ person.name.slice(0, 1) }}</span><span class="conversation-text"><strong>{{ person.name }}</strong><small>{{ contactPreview(person) }}</small></span><span v-if="directConversation(person.id) && hasUnread(directConversation(person.id).id)" class="unread-badge">{{ unreadLabel(directConversation(person.id).id) }}</span>
              </button>
            </template>
            <template v-else-if="conversationFilter === 'group'">
              <button v-for="item in groupConversations" :key="item.id" class="conversation" :class="{ active: active === item.id }" @click="select(item.id)">
                <span class="avatar">{{ title(item).slice(0, 1) }}</span><span class="conversation-text"><strong>{{ title(item) }}</strong><small>{{ preview(item) }}</small></span><span v-if="hasUnread(item.id)" class="unread-badge">{{ unreadLabel(item.id) }}</span>
              </button>
            </template>
            <template v-else>
              <button v-for="item in recentConversations" :key="item.id" class="conversation" :class="{ active: active === item.id }" @click="select(item.id)">
                <span class="avatar">{{ title(item).slice(0, 1) }}</span><span class="conversation-text"><strong>{{ title(item) }}</strong><small>{{ preview(item) }}</small></span><span v-if="hasUnread(item.id)" class="unread-badge">{{ unreadLabel(item.id) }}</span>
              </button>
            </template>
            <p v-if="conversationFilter === 'friends' && !friends.length" class="empty-side">还没有好友，点击“添加”开始。</p>
            <p v-else-if="conversationFilter === 'group' && !groupConversations.length" class="empty-side">还没有加入群聊。</p>
            <p v-else-if="conversationFilter === 'current' && !recentConversations.length" class="empty-side">还没有最近会话。</p>
         </div>
        <footer class="sidebar-footer"><span class="avatar self">{{ me.name.slice(0,1) }}</span><span class="identity"><strong>{{ me.name }}</strong><small>{{ connected ? '实时连接已建立' : '正在重连 · 历史可用' }}</small></span><button @click="showSettings = !showSettings" title="设置" aria-haspopup="menu" :aria-expanded="showSettings">设置</button><div v-if="showSettings" class="settings-menu" role="menu"><button type="button" role="menuitem" @click="openProfile">个人</button><button type="button" role="menuitem" @click="logout">退出</button></div></footer>
      </aside>
      <main class="chat">
        <template v-if="current">
          <header class="chat-head"><span class="avatar">{{ title(current).slice(0,1) }}</span><div><h2>{{ title(current) }}</h2><p>{{ current.kind === 'group' ? current.members.length + ' 位成员' : '私聊' }} · {{ current.members.slice(0,5).map(member => member.name).join('、') }}{{ current.members.length > 5 ? ' 等 ' + current.members.length + ' 人' : '' }}</p></div><button v-if="current.kind === 'group'" class="outline-button" @click="openInvite">邀请成员</button></header>
           <section ref="stream" class="stream" aria-label="聊天记录">
            <button v-if="messages[0]?.seq > 1" class="older" @click="loadOlder">加载更早的消息</button>
            <div v-if="!messages.length" class="empty-chat">说声你好，开始这段对话。</div>
            <div v-for="item in messages" :key="item.id" class="message-row" :class="{ mine: item.sender_id === me.id, system: item.kind === 'system' }">
              <template v-if="item.kind === 'system'"><span class="system-notice">{{ item.content }}</span></template>
              <template v-else><span v-if="item.sender_id !== me.id" class="avatar tiny">{{ sender(item.sender_id).slice(0,1) }}</span>
              <div><div class="message-meta">{{ sender(item.sender_id) }} · {{ time(item.created_at) }}</div><div class="bubble">{{ item.content }}</div></div></template>
            </div>
          </section>
          <form class="composer" @submit.prevent="send"><p v-if="pendingSend?.conversation_id === active" class="pending-notice">发送结果未确认，消息可能已保存。<button type="button" @click="draft = pendingSend.content">恢复原文安全重试</button><button type="button" @click="discardPending">放弃确认</button></p><textarea v-model="draft" maxlength="4000" placeholder="输入消息…" @keydown.enter.exact="onEnter"></textarea><div><span>Enter 发送 · Shift + Enter 换行</span><button class="primary" :disabled="!draft.trim() || sending">{{ sending ? '发送中…' : '发送 ↗' }}</button></div></form>
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
      <div v-if="showAddUser" class="modal-backdrop" @click.self="showAddUser = false"><div class="modal"><div class="modal-head"><h2>添加用户</h2><button @click="showAddUser = false">×</button></div><p class="subtle">通过用户名或用户账号搜索，选择用户添加为好友。</p><input v-model="friendQuery" type="search" placeholder="搜索用户名或账号" aria-label="搜索用户名或账号" /><div class="user-list"><label v-for="person in friendCandidates" :key="person.id"><span class="avatar tiny">{{ person.name.slice(0,1) }}</span><span>{{ person.name }}<small>账号：{{ person.email }}</small></span><button type="button" class="small-button" :disabled="friendBusy" @click="addFriend(person.id)">{{ friendBusy ? '处理中…' : '添加' }}</button></label><p v-if="!friendCandidates.length">没有匹配的用户。</p></div></div></div>
      <div v-if="showInvite" class="modal-backdrop" @click.self="showInvite = false"><div class="modal"><div class="modal-head"><h2>邀请成员</h2><button @click="showInvite = false">×</button></div><p class="subtle">选择要加入“{{ current?.name }}”的用户，群聊最多容纳 1000 人，当前 {{ current?.members.length }} 人。</p><div class="user-list"><label v-for="person in inviteCandidates" :key="person.id"><input v-model="inviteUsers" type="checkbox" :value="person.id" :disabled="(current?.members.length || 0) + inviteUsers.length >= 1000 && !inviteUsers.includes(person.id)" /><span class="avatar tiny">{{ person.name.slice(0,1) }}</span><span>{{ person.name }}<small>{{ person.email }}</small></span></label><p v-if="!inviteCandidates.length">暂无可邀请的用户。</p></div><button class="primary" :disabled="conversationBusy || !inviteUsers.length || (current?.members.length || 0) + inviteUsers.length > 1000" @click="addMembers">{{ conversationBusy ? '邀请中…' : '邀请加入群聊' }}</button></div></div>
      <div v-if="showProfile" class="modal-backdrop" @click.self="showProfile = false"><div class="modal profile-modal" role="dialog" aria-modal="true" aria-label="个人信息"><div class="modal-head"><h2>个人信息</h2><button @click="showProfile = false" aria-label="关闭">×</button></div><div class="profile-header"><span class="avatar profile-avatar self">{{ (profileEditing ? profileName : me.name).slice(0,1) }}</span><div><h3>{{ me.name }}</h3><p class="subtle">个人资料</p></div></div><template v-if="profileEditing"><div class="profile-form"><label>用户名<input v-model="profileName" maxlength="64" autocomplete="name" /></label><label>用户账号<input v-model="profileEmail" type="email" maxlength="254" autocomplete="email" /></label></div><div class="profile-actions"><button class="outline-button" type="button" :disabled="profileBusy" @click="profileEditing = false">取消</button><button class="primary" type="button" :disabled="profileBusy || !profileName.trim() || !profileEmail.trim()" @click="updateProfile">{{ profileBusy ? "保存中…" : "保存修改" }}</button></div></template><template v-else><dl class="profile-details"><div><dt>用户名</dt><dd>{{ me.name }}</dd></div><div><dt>用户账号</dt><dd>{{ me.email }}</dd></div><div><dt>用户 ID</dt><dd>{{ me.id }}</dd></div></dl><button class="primary profile-edit-button" type="button" @click="profileEditing = true">编辑个人信息</button></template></div></div>
    </div>
    <div v-if="error" class="toast" role="alert">{{ error }}</div>
  `,
}).mount('#app')
