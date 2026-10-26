import { ref, nextTick, onUnmounted } from 'vue/dist/vue.esm-bundler.js'
import { api } from './api.js'

const errors = {
  already_member: '你已加入该群，请刷新列表。',
  group_full: '该群人数已满。',
  invalid_members: '该群人数已满，暂时无法加入。',
  conversation_dismissed: '该群已解散，无法加入，请刷新列表。',
  not_found: '该群已不存在或已解散，请刷新列表。',
  invalid_join_request: '申请说明过长，请缩短后重试。',
}

export const GroupDirectory = {
  props: { showTrigger: { type: Boolean, default: true } },
  emits: ['joined'],
  setup(_, { emit }) {
    const opened = ref(false), panel = ref(null), trigger = ref(null)
    const query = ref(''), groups = ref([]), history = ref([])
    const busy = ref(false), actionBusy = ref(false), error = ref(''), notice = ref('')
    const target = ref(null), message = ref('')
    let generation = 0, request
    async function load() {
      request?.abort()
      const version = ++generation
      request = new AbortController()
      busy.value = true; error.value = ''
      try {
        const [entries, requests] = await Promise.all([
          api(`/groups/discover?${new URLSearchParams({ q: query.value.trim() })}`, { signal: request.signal }),
          api('/groups/join-requests', { signal: request.signal }),
        ])
        if (version !== generation) return
        groups.value = entries; history.value = requests
      } catch (cause) {
        if (version === generation && cause.name !== 'AbortError') error.value = '群列表加载失败，请重试。'
      } finally { if (version === generation) busy.value = false }
    }
    async function open() {
      opened.value = true; query.value = ''; target.value = null; message.value = ''; notice.value = ''
      groups.value = []; history.value = []; load()
      await nextTick(); panel.value?.querySelector('input[type="search"]')?.focus()
    }
    function close() {
      if (actionBusy.value) return
      generation++; request?.abort(); opened.value = false
      nextTick(() => (trigger.value || document.querySelector('.sidebar-more > button'))?.focus())
    }
    function choose(group) { target.value = group; message.value = ''; error.value = ''; notice.value = '' }
    async function join() {
      if (!target.value || actionBusy.value) return
      const group = target.value
      actionBusy.value = true; error.value = ''; notice.value = ''
      try {
        const result = await api(`/groups/${encodeURIComponent(group.id)}/join-requests`, { method: 'POST', body: JSON.stringify({ message: message.value.trim() }) })
        if (typeof result?.pending !== 'boolean') throw new Error('invalid_join_response')
        target.value = null; message.value = ''
        if (result.pending) { notice.value = '入群申请已提交，等待群主或管理员审批。'; await load() }
        else { actionBusy.value = false; close(); emit('joined', group.id) }
      } catch (cause) { error.value = errors[cause.code] || '加入群聊失败，请重试。' }
      finally { actionBusy.value = false }
    }
    async function withdraw(id) {
      if (actionBusy.value) return
      actionBusy.value = true; error.value = ''; notice.value = ''
      try {
        await api(`/groups/${encodeURIComponent(id)}/join-requests`, { method: 'DELETE' })
        notice.value = '已撤回入群申请。'; await load()
      } catch (cause) { error.value = '撤回失败，申请可能已经被处理，请刷新后查看。' }
      finally { actionBusy.value = false }
    }
    function keyboard(event) {
      if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close() }
      if (event.key !== 'Tab') return
      const items = [...panel.value.querySelectorAll('button,input,textarea')].filter(el => !el.disabled)
      const first = items[0], last = items.at(-1)
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
      if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
    }
    onUnmounted(() => { generation++; request?.abort() })
    return { opened, panel, trigger, query, groups, history, busy, actionBusy, error, notice, target, message, open, close, load, choose, join, withdraw, keyboard }
  },
  template: `
    <button v-if="showTrigger" ref="trigger" type="button" class="outline-button group-directory-trigger" aria-haspopup="dialog" @click="open">加入群聊</button>
    <Teleport to="body"><div v-if="opened" class="modal-backdrop" @click.self="close" @keydown="keyboard">
      <section ref="panel" class="modal group-directory-panel" role="dialog" aria-modal="true" aria-labelledby="group-directory-title">
        <div class="modal-head"><h2 id="group-directory-title">加入群聊</h2><button type="button" :disabled="actionBusy" aria-label="关闭加入群聊" @click="close">×</button></div>
        <p class="subtle">查找本服务器上的群聊。开启入群审批的群需等待群主或管理员同意。</p>
        <form class="message-search-form" role="search" @submit.prevent="load()">
          <input v-model="query" type="search" maxlength="100" placeholder="搜索群名称或群 ID" aria-label="搜索群名称或群 ID" :disabled="actionBusy" />
          <button class="primary" type="submit" :disabled="busy || actionBusy">{{ busy ? '搜索中…' : '搜索' }}</button>
        </form>
        <p v-if="error" class="search-error" role="alert">{{ error }} <button type="button" :disabled="busy || actionBusy" @click="load">刷新列表</button></p>
        <p v-if="notice" role="status">{{ notice }}</p>
        <form v-if="target" class="group-join-form" @submit.prevent="join">
          <strong>{{ target.name }}</strong>
          <label v-if="target.join_approval">申请说明<textarea v-model="message" maxlength="500" rows="3" placeholder="介绍一下自己（可选）" :disabled="actionBusy"></textarea></label>
          <p v-else class="subtle">该群无需审批，确认后即可加入。</p>
          <div class="request-actions"><button type="submit" class="primary" :disabled="actionBusy">{{ actionBusy ? '处理中…' : target.join_approval ? '提交入群申请' : '确认加入' }}</button><button type="button" class="outline-button" :disabled="actionBusy" @click="target = null">取消</button></div>
        </form>
        <div class="group-directory-results" :aria-busy="busy">
          <p v-if="busy" role="status">正在加载群聊…</p>
          <p v-else-if="!groups.length && !error" role="status">没有找到可加入的群聊，可更换关键词。</p>
          <ul class="group-directory-list"><li v-for="group in groups" :key="group.id">
            <span class="avatar" aria-hidden="true">{{ group.name.slice(0,1) }}</span>
            <div class="group-directory-info"><strong>{{ group.name }}</strong><small>{{ group.member_count }} 人 · {{ group.join_approval ? '需审批' : '可直接加入' }}</small><small>群 ID：{{ group.id }}</small></div>
            <button v-if="group.request_status === 'pending'" type="button" class="outline-button" :disabled="actionBusy" @click="withdraw(group.id)">撤回申请</button>
            <button v-else type="button" class="outline-button" :disabled="actionBusy || group.member_count >= 1000" @click="choose(group)">{{ group.member_count >= 1000 ? '人数已满' : group.join_approval ? '申请加入' : '加入群聊' }}</button>
          </li></ul>
          <p v-if="groups.length >= 100" class="subtle">最多显示 100 个群，请使用关键词缩小范围。</p>
        </div>
        <section v-if="history.length" aria-label="我的入群申请"><h3>我的入群申请</h3><button type="button" class="text-button" :disabled="busy || actionBusy" @click="load">刷新审批结果</button>
          <ul class="group-directory-list"><li v-for="item in history" :key="item.id"><div class="group-directory-info"><strong>{{ item.group_name || item.conversation_id }}</strong><small>{{ item.status === 'accepted' ? '已通过' : item.status === 'rejected' ? '已拒绝' : '待审批' }}</small></div><button v-if="item.status === 'pending'" type="button" class="outline-button" :disabled="actionBusy" @click="withdraw(item.conversation_id)">撤回申请</button></li></ul>
        </section>
      </section>
    </div></Teleport>`,
}
