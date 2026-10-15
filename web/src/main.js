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
    const conversations = ref([])
    const active = ref(null)
    const messages = ref([])
    const deliveryStatuses = ref({})
    const pendingSend = ref(null)
    const draft = ref('')
    const groupName = ref('')
    const selectedUsers = ref([])
    const inviteUsers = ref([])
    const createMode = ref('direct')
    const conversationFilter = ref('all')
    const showCreate = ref(false)
    const showInvite = ref(false)
    const conversationBusy = ref(false)
    const connected = ref(false)
    const loading = ref(false)
    const stream = ref(null)
    const sending = ref(false)
    const visibleCount = ref(100)
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
    const visibleConversations = computed(() => conversationFilter.value === 'all'
      ? conversations.value
      : conversations.value.filter(item => item.kind === conversationFilter.value))
    const inviteCandidates = computed(() => {
      const memberIDs = new Set(current.value?.members.map(member => member.id) || [])
      return users.value.filter(person => !memberIDs.has(person.id))
    })
    const title = item => item.kind === 'direct'
      ? item.members.find(member => member.id !== me.value?.id)?.name || '私聊'
      : item.name
    const sender = id => current.value?.members.find(member => member.id === id)?.name || '成员'
    const time = value => new Date(value).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
    const scroll = async () => { await nextTick(); if (stream.value) stream.value.scrollTop = stream.value.scrollHeight }
    const showError = cause => { error.value = cause?.message || '操作失败'; clearTimeout(errorTimer); errorTimer = setTimeout(() => { error.value = '' }, 5000) }
    const atBottom = () => !stream.value || stream.value.scrollHeight - stream.value.scrollTop - stream.value.clientHeight < 64
    const showMessages = id => { if (id === active.value) messages.value = (cache.get(id) || []).slice(-visibleCount.value) }
    const readLabel = item => {
      const others = current.value?.members.filter(member => member.id !== me.value?.id) || []
      const count = others.filter(member => member.last_read_seq >= item.seq).length
      return current.value?.kind === 'direct' ? (count ? '已读' : '未读') : `${count}/${others.length} 已读`
    }
    async function checkDelivery(item) {
      const version = epoch
      try {
        const status = await api(`/conversations/${item.conversation_id}/messages/${item.seq}/delivery`)
        if (version === epoch) deliveryStatuses.value = { ...deliveryStatuses.value, [item.id]: status }
      } catch (cause) { if (version === epoch) showError(cause) }
    }
    const deliveryLabel = item => {
      const status = deliveryStatuses.value[item.id]
      return status ? `${status.confirmed_members}/${status.total_members} 成员设备确认接收（上次查询 ${time(status.as_of)}，点击刷新）` : '查看接收确认'
    }
    function discardPending() {
      if (!pendingSend.value || !confirm('放弃确认不等于撤回：消息可能已经保存在服务器。请先检查聊天记录，确定仍要放弃吗？')) return
      pendingStore.clear(pendingSend.value.conversation_id, pendingSend.value.client_msg_id)
      pendingSend.value = null
    }
    function applyReceipt(receipt) {
      const conversation = conversations.value.find(item => item.id === receipt.conversation_id)
      const member = conversation?.members.find(item => item.id === receipt.user_id)
      if (member && receipt.seq > member.last_read_seq) { member.last_read_seq = receipt.seq; member.read_at = receipt.read_at }
      if (conversation && receipt.user_id === me.value?.id) conversation.last_read_seq = Math.max(conversation.last_read_seq, receipt.seq)
    }

    async function refresh() {
      if (refreshTask) return refreshTask
      const version = epoch
      const task = (async () => {
        const [list, people] = await Promise.all([api('/conversations'), api('/users')])
        if (version !== epoch || !me.value) return
        conversations.value = list
        users.value = people
        if (!active.value && list.length) active.value = list[0].id
        if (active.value && !list.some(item => item.id === active.value)) active.value = null
        for (const item of list) {
          if (!me.value || version !== epoch) return
          await syncConversation(item.id)
        }
        await markRead()
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
          const stick = atBottom()
          const merged = appendPage(existing, page, id)
          cache.set(id, merged)
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
      if (version === epoch) await markRead()
    }
    async function reconcile() {
      const version = epoch
      const cursors = await api(`/devices/${deviceID}/cursors`)
      if (version === epoch && me.value) await catchUp(cursors)
    }
    async function select(id) {
      active.value = id
      pendingSend.value = pendingStore?.get(id) || null
      visibleCount.value = 100
      showMessages(id)
      try {
        await syncConversation(id)
        if (active.value === id) { await scroll(); await markRead() }
      } catch (cause) { showError(cause) }
    }
    async function markRead() {
      if (!me.value || !active.value || !messages.value.length || document.visibilityState !== 'visible' || !document.hasFocus() || !atBottom()) return
      const id = active.value
      const version = epoch
      const seq = messages.value.at(-1).seq
      if (seq <= (current.value?.last_read_seq || 0)) return
      const receipt = await api(`/conversations/${id}/read`, { method: 'POST', body: JSON.stringify({ seq }) })
      if (version === epoch) applyReceipt(receipt)
    }
    const onVisible = () => { if (me.value) reconcile().catch(showError) }
    const onScroll = () => { markRead().catch(showError) }
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
            await markRead()
          }
          if (envelope.event === 'message.sync_required') await catchUp(envelope.data.cursors)
          if (envelope.event === 'conversation.created') await refresh()
          if (envelope.event === 'conversation.read') applyReceipt(envelope.data)
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
      cache.clear(); syncing.clear(); refreshTask = null; pendingStore = null; pendingSend.value = null; deliveryStatuses.value = {}
      active.value = null; messages.value = []; conversations.value = []; users.value = []; draft.value = ''
      showCreate.value = false; showInvite.value = false; conversationFilter.value = 'all'
      try { await api('/auth/logout', { method: 'POST' }) } catch (cause) { showError(cause) }
    }
    function openCreate(kind) {
      createMode.value = kind
      selectedUsers.value = []
      groupName.value = ''
      showCreate.value = true
    }
    function chooseCreateUser(id) {
      selectedUsers.value = selectedUsers.value.includes(id) ? [] : [id]
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
    async function create(kind, memberIDs) {
      if (conversationBusy.value) return
      const version = epoch
      conversationBusy.value = true
      try {
        const created = await api('/conversations', { method: 'POST', body: JSON.stringify({ kind, name: kind === 'group' ? groupName.value : '', member_ids: memberIDs }) })
        if (version !== epoch) return
        showCreate.value = false; groupName.value = ''; selectedUsers.value = []
        conversationFilter.value = kind
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
        await syncConversation(id); await markRead()
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
    return { me, mode, email, password, name, error, users, conversations, visibleConversations, conversationFilter, active, messages, pendingSend, draft, groupName, selectedUsers, inviteUsers, inviteCandidates, createMode, conversationBusy, showCreate, showInvite, connected, loading, sending, stream, current, title, sender, time, authenticate, logout, openCreate, chooseCreateUser, openInvite, addMembers, create, select, send, loadOlder, readLabel, deliveryLabel, checkDelivery, discardPending, onScroll, onEnter }
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
        <div class="side-title"><span>消息</span><button @click="openCreate('direct')" title="发起聊天">＋</button></div>
        <div class="conversation-tabs"><button :class="{ active: conversationFilter === 'all' }" @click="conversationFilter = 'all'">全部</button><button :class="{ active: conversationFilter === 'direct' }" @click="conversationFilter = 'direct'">私聊</button><button :class="{ active: conversationFilter === 'group' }" @click="conversationFilter = 'group'">群聊</button></div>
        <div class="conversation-list">
          <button v-for="item in visibleConversations" :key="item.id" class="conversation" :class="{ active: active === item.id }" @click="select(item.id)">
            <span class="avatar">{{ title(item).slice(0, 1) }}</span><span class="conversation-text"><strong>{{ title(item) }}</strong><small>{{ item.kind === 'group' ? item.members.length + ' 位成员' : '私聊' }}</small></span><span v-if="item.last_seq > item.last_read_seq" class="dot"></span>
          </button>
          <p v-if="!visibleConversations.length" class="empty-side">还没有{{ conversationFilter === 'group' ? '群聊' : conversationFilter === 'direct' ? '私聊' : '会话' }}，点击＋开始。</p>
        </div>
        <footer class="sidebar-footer"><span class="avatar self">{{ me.name.slice(0,1) }}</span><span class="identity"><strong>{{ me.name }}</strong><small>{{ connected ? '实时连接已建立' : '正在重连 · 历史可用' }}</small></span><button @click="logout" title="退出登录">退出</button></footer>
      </aside>
      <main class="chat">
        <template v-if="current">
          <header class="chat-head"><span class="avatar">{{ title(current).slice(0,1) }}</span><div><h2>{{ title(current) }}</h2><p>{{ current.kind === 'group' ? current.members.length + ' 位成员' : '私聊' }} · {{ current.members.slice(0,5).map(member => member.name).join('、') }}{{ current.members.length > 5 ? ' 等 ' + current.members.length + ' 人' : '' }}</p></div><button v-if="current.kind === 'group'" class="outline-button" @click="openInvite">邀请成员</button></header>
          <section ref="stream" class="stream" aria-label="聊天记录" @scroll.passive="onScroll">
            <button v-if="messages[0]?.seq > 1" class="older" @click="loadOlder">加载更早的消息</button>
            <div v-if="!messages.length" class="empty-chat">说声你好，开始这段对话。</div>
            <div v-for="item in messages" :key="item.id" class="message-row" :class="{ mine: item.sender_id === me.id }">
              <span v-if="item.sender_id !== me.id" class="avatar tiny">{{ sender(item.sender_id).slice(0,1) }}</span>
              <div><div class="message-meta">{{ sender(item.sender_id) }} · {{ time(item.created_at) }}</div><div class="bubble">{{ item.content }}</div><small v-if="item.sender_id === me.id" class="message-meta">{{ readLabel(item) }} · <button type="button" class="delivery-check" @click="checkDelivery(item)">{{ deliveryLabel(item) }}</button></small></div>
            </div>
          </section>
          <form class="composer" @submit.prevent="send"><p v-if="pendingSend?.conversation_id === active" class="pending-notice">发送结果未确认，消息可能已保存。<button type="button" @click="draft = pendingSend.content">恢复原文安全重试</button><button type="button" @click="discardPending">放弃确认</button></p><textarea v-model="draft" maxlength="4000" placeholder="输入消息…" @keydown.enter.exact="onEnter"></textarea><div><span>Enter 发送 · Shift + Enter 换行</span><button class="primary" :disabled="!draft.trim() || sending">{{ sending ? '发送中…' : '发送 ↗' }}</button></div></form>
        </template>
        <div v-else class="welcome"><span class="brand-mark">✦</span><h2>开启一段新对话</h2><p>与同伴即时沟通，让想法流动起来。</p><div class="welcome-actions"><button class="primary" @click="openCreate('direct')">发起私聊</button><button class="outline-button" @click="openCreate('group')">创建群聊</button></div></div>
      </main>
      <div v-if="showCreate" class="modal-backdrop" @click.self="showCreate = false">
        <div class="modal">
          <div class="modal-head"><h2>{{ createMode === 'group' ? '创建群聊' : '发起私聊' }}</h2><button @click="showCreate = false">×</button></div>
          <div class="modal-tabs"><button :class="{ active: createMode === 'direct' }" @click="openCreate('direct')">私聊</button><button :class="{ active: createMode === 'group' }" @click="openCreate('group')">群聊</button></div>
          <p class="subtle">{{ createMode === 'group' ? '创建后群内只有你自己，之后可在群内按需邀请其他用户。含自己最多 1000 人。' : '选择一位用户开始私聊。' }}</p>
          <input v-if="createMode === 'group'" v-model="groupName" maxlength="100" placeholder="群聊名称" />
          <div v-else class="user-list">
            <label v-for="person in users" :key="person.id"><input :checked="selectedUsers.includes(person.id)" type="checkbox" @change="chooseCreateUser(person.id)" /><span class="avatar tiny">{{ person.name.slice(0,1) }}</span><span>{{ person.name }}<small>{{ person.email }}</small></span></label>
            <p v-if="!users.length">暂无其他用户，邀请朋友注册后再试。</p>
          </div>
          <button class="primary" :disabled="conversationBusy || (createMode === 'direct' ? selectedUsers.length !== 1 : !groupName.trim())" @click="create(createMode, createMode === 'group' ? [] : selectedUsers)">{{ conversationBusy ? '处理中…' : createMode === 'group' ? '创建群聊' : '开始私聊' }}</button>
        </div>
      </div>
      <div v-if="showInvite" class="modal-backdrop" @click.self="showInvite = false"><div class="modal"><div class="modal-head"><h2>邀请成员</h2><button @click="showInvite = false">×</button></div><p class="subtle">选择要加入“{{ current?.name }}”的用户，群聊最多容纳 1000 人，当前 {{ current?.members.length }} 人。</p><div class="user-list"><label v-for="person in inviteCandidates" :key="person.id"><input v-model="inviteUsers" type="checkbox" :value="person.id" :disabled="(current?.members.length || 0) + inviteUsers.length >= 1000 && !inviteUsers.includes(person.id)" /><span class="avatar tiny">{{ person.name.slice(0,1) }}</span><span>{{ person.name }}<small>{{ person.email }}</small></span></label><p v-if="!inviteCandidates.length">暂无可邀请的用户。</p></div><button class="primary" :disabled="conversationBusy || !inviteUsers.length || (current?.members.length || 0) + inviteUsers.length > 1000" @click="addMembers">{{ conversationBusy ? '邀请中…' : '邀请加入群聊' }}</button></div></div>
    </div>
    <div v-if="error" class="toast" role="alert">{{ error }}</div>
  `,
}).mount('#app')
