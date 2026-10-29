import { ref, nextTick, onUnmounted } from 'vue/dist/vue.esm-bundler.js'
import { api } from './api.js'
import { formatBytes, mediaURL } from './media.js'
import './group-files.css'

export const GroupFiles = {
  props: { conversation: { type: Object, required: true } },
  setup(props) {
    const opened = ref(false), trigger = ref(null), panel = ref(null)
    const files = ref([]), busy = ref(false), error = ref(''), hasMore = ref(false)
    let cursor = 0, generation = 0, request
    const uploadedBy = id => props.conversation.members?.find(person => person.id === id)?.name || '已离群成员'
    const uploadedAt = value => new Date(value).toLocaleString('zh-CN')
    async function load(more = false) {
      if (!opened.value || (more && (busy.value || !hasMore.value))) return
      const version = ++generation
      request?.abort()
      const controller = new AbortController()
      request = controller
      const id = props.conversation.id
      busy.value = true; error.value = ''
      if (!more) { files.value = []; cursor = 0; hasMore.value = false }
      const params = new URLSearchParams({ type: 'files', limit: '30' })
      if (more) params.set('before_seq', String(cursor))
      try {
        const page = await api(`/conversations/${encodeURIComponent(id)}/messages/search?${params}`, { signal: controller.signal })
        if (version !== generation || !opened.value || id !== props.conversation.id) return
        const incoming = page.messages.filter(message => message.kind === 'file' && !message.recalled_at && message.media_id && message.media)
        const known = new Set(files.value.map(message => message.id))
        files.value = [...files.value, ...incoming.filter(message => !known.has(message.id))]
        cursor = page.next_before_seq
        hasMore.value = page.has_more
      } catch (cause) {
        if (version === generation && !controller.signal.aborted) error.value = cause.status === 404 ? '你已不在该群，无法查看群文件。' : '群文件加载失败，请重试。'
      } finally { if (version === generation) busy.value = false }
    }
    async function open() {
      opened.value = true
      load()
      await nextTick(); panel.value?.querySelector('button')?.focus()
    }
    function close() {
      generation++; request?.abort(); opened.value = false; busy.value = false
      nextTick(() => trigger.value?.focus())
    }
    function keyboard(event) {
      if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close(); return }
      if (event.key !== 'Tab') return
      const controls = [...panel.value.querySelectorAll('button:not(:disabled),a[href]')]
      const first = controls[0], last = controls.at(-1)
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
      if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
    }
    onUnmounted(() => { generation++; request?.abort() })
    return { opened, trigger, panel, files, busy, error, hasMore, uploadedBy, uploadedAt, formatBytes, mediaURL, open, close, load, keyboard }
  },
  template: `
    <button ref="trigger" type="button" class="small-button group-files-trigger" aria-haspopup="dialog" @click="open">群文件</button>
    <Teleport to="body"><div v-if="opened" class="modal-backdrop" @click.self="close" @keydown="keyboard">
      <section ref="panel" class="modal group-files-modal" role="dialog" aria-modal="true" aria-labelledby="group-files-title">
        <div class="modal-head"><h2 id="group-files-title">群文件</h2><button type="button" aria-label="关闭群文件" @click="close">×</button></div>
        <p class="subtle">群内已发送的文件，所有群成员均可查看和下载，包含加入群聊之前的文件。</p>
        <div class="group-files-toolbar"><span>{{ conversation.name }}</span><button type="button" class="small-button" :disabled="busy" @click="load()">刷新列表</button></div>
        <div role="status" aria-live="polite"><p v-if="error" class="search-error">{{ error }}<button type="button" class="small-button" @click="load(files.length > 0)">重试</button></p><p v-else-if="busy && !files.length" class="subtle">正在加载群文件…</p><p v-else-if="!files.length" class="subtle">暂无群文件。在聊天输入框点击“上传文件”，发送后会出现在这里。</p></div>
        <ul class="group-files-list" :aria-busy="busy">
          <li v-for="item in files" :key="item.id" class="group-file-row">
            <div class="group-file-info"><strong>{{ item.media.file_name }}</strong><small>{{ formatBytes(item.media.byte_size) }} · {{ uploadedBy(item.sender_id) }} · {{ uploadedAt(item.created_at) }}</small></div>
            <a class="outline-button group-file-download" :href="mediaURL(item.media_id)" :download="item.media.file_name" :aria-label="'下载 ' + item.media.file_name">下载</a>
          </li>
        </ul>
        <button v-if="hasMore && !error" type="button" class="outline-button group-files-more" :disabled="busy" @click="load(true)">{{ busy ? '加载中…' : '加载更多文件' }}</button>
      </section>
    </div></Teleport>
  `,
}
