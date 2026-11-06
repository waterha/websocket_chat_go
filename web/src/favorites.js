import { ref, computed, watch, nextTick, onUnmounted } from 'vue/dist/vue.esm-bundler.js'
import { api } from './api.js'
import { SavedMessageContent } from './saved-message-content.js'
import './favorites.css'

export function useFavorites() {
  const pending = new Map()
  function save(message, conversationID) {
    if (!message?.id || message.kind === 'system' || message.recalled_at) return Promise.reject(new Error('该消息无法收藏'))
    const key = `${conversationID}:${message.id}`
    if (!pending.has(key)) pending.set(key, api('/favorites', { method: 'POST', body: JSON.stringify({ conversation_id: conversationID, message_id: message.id }) }).finally(() => pending.delete(key)))
    return pending.get(key)
  }
  return { save }
}

export const FavoritesPanel = {
  components: { SavedMessageContent },
  props: { open: Boolean, userID: { type: String, required: true } },
  emits: ['update:open'],
  setup(props, { emit }) {
    const panel = ref(null), query = ref(''), entries = ref([]), busy = ref(false), error = ref(''), hasMore = ref(false), removing = ref(''), removeTarget = ref(null), removeError = ref('')
    let cursor = '', generation = 0, request, previousFocus, retryMore = false
    const filtered = computed(() => {
      const needle = query.value.trim().toLocaleLowerCase()
      return entries.value.filter(item => !needle || [item.sender_name, item.conversation_name, item.message?.content, item.message?.reply?.excerpt, item.message?.media?.file_name, ...(item.message?.blocks || []).map(block => block.text || block.file_name)].join('\n').toLocaleLowerCase().includes(needle))
    })
    const date = value => { const timestamp = new Date(value); return Number.isNaN(timestamp.getTime()) ? '时间未知' : timestamp.toLocaleString('zh-CN') }
    async function load(more = false) {
      if (!props.open || !props.userID || busy.value || (more && !hasMore.value)) return
      const version = ++generation, identity = props.userID
      request?.abort(); request = new AbortController()
      busy.value = true; error.value = ''; retryMore = more
      if (!more) { entries.value = []; cursor = ''; hasMore.value = false }
      const params = new URLSearchParams({ limit: '30' })
      if (more && cursor) params.set('before_id', cursor)
      try {
        const page = await api(`/favorites?${params}`, { signal: request.signal })
        if (version !== generation || !props.open || identity !== props.userID) return
        const known = new Set(entries.value.map(item => item.id))
        entries.value = [...entries.value, ...(page.favorites || []).filter(item => !known.has(item.id))]
        cursor = page.next_before_id || ''; hasMore.value = Boolean(page.has_more && cursor)
      } catch (cause) { if (version === generation && cause.name !== 'AbortError') error.value = cause.message || '收藏加载失败，请重试' }
      finally { if (version === generation) busy.value = false }
    }
    function reset() { generation++; request?.abort(); busy.value = false; entries.value = []; error.value = ''; hasMore.value = false; cursor = ''; removeTarget.value = null; removing.value = ''; removeError.value = '' }
    function close() { emit('update:open', false) }
    function cancelRemove() {
      if (removing.value) return
      const id = removeTarget.value?.id
      removeTarget.value = null
      nextTick(() => [...(panel.value?.querySelectorAll('.favorite-entry') || [])].find(entry => entry.dataset.favoriteId === id)?.querySelector('button')?.focus())
    }
    async function confirmRemove() {
      if (!removeTarget.value || removing.value) return
      const id = removeTarget.value.id, version = generation
      removing.value = id; removeError.value = ''
      try {
        await api(`/favorites/${encodeURIComponent(id)}`, { method: 'DELETE' })
        if (version !== generation) return
        entries.value = entries.value.filter(item => item.id !== id); removeTarget.value = null
        await nextTick(); panel.value?.querySelector('[aria-label="搜索收藏"]')?.focus()
      } catch (cause) { if (version === generation) removeError.value = cause.message || '移除失败，请重试' }
      finally { if (version === generation) removing.value = '' }
    }
    async function askRemove(item) { removeTarget.value = item; removeError.value = ''; await nextTick(); panel.value?.querySelector('.favorites-cancel-remove')?.focus() }
    function keyboard(event) {
      if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); if (removeTarget.value && !removing.value) cancelRemove(); else close(); return }
      if (event.key !== 'Tab') return
      const focusScope = removeTarget.value ? panel.value.querySelector('.favorites-confirm') : panel.value
      const controls = [...focusScope.querySelectorAll('button:not(:disabled),input,a[href],audio[controls],video[controls]')]
      const first = controls[0], last = controls.at(-1)
      if (event.shiftKey && (document.activeElement === first || !focusScope.contains(document.activeElement))) { event.preventDefault(); last?.focus() }
      if (!event.shiftKey && (document.activeElement === last || !focusScope.contains(document.activeElement))) { event.preventDefault(); first?.focus() }
    }
    watch(() => [props.open, props.userID], async ([open], old) => {
      reset(); query.value = ''
      if (open) { if (!old?.[0]) previousFocus = document.activeElement; load(); await nextTick(); panel.value?.querySelector('[aria-label="搜索收藏"]')?.focus() }
      else if (old?.[0]) nextTick(() => { const target = previousFocus?.isConnected ? previousFocus : document.querySelector('.sidebar-footer button[aria-haspopup="menu"]'); target?.focus({ preventScroll: true }) })
    }, { immediate: true })
    onUnmounted(reset)
    return { panel, query, entries, filtered, busy, error, hasMore, removing, removeTarget, removeError, date, load, retry: () => load(retryMore), close, confirmRemove, askRemove, cancelRemove, keyboard }
  },
  template: `<Teleport to="body"><div v-if="open" class="modal-backdrop favorites-backdrop" @click.self="close" @keydown="keyboard"><section ref="panel" class="modal favorites-panel" role="dialog" aria-modal="true" aria-labelledby="favorites-title"><header class="modal-head"><h2 id="favorites-title">收藏</h2><button type="button" class="icon-button" aria-label="关闭收藏" @click="close">×</button></header><div class="favorites-toolbar"><input v-model="query" type="search" aria-label="搜索收藏" placeholder="搜索已加载的消息、姓名、来源或文件名" /><button type="button" class="outline-button" :disabled="busy" @click="load()">刷新</button></div><p class="subtle">保存的消息快照，原消息撤回或离开会话后仍可查看。搜索范围为已加载的收藏。</p><div v-if="removeTarget" class="favorites-confirm" role="alertdialog" aria-label="移除收藏确认"><p>确定移除这条收藏？移除后无法恢复。</p><p v-if="removeError" role="alert">{{ removeError }}</p><button type="button" class="outline-button favorites-cancel-remove" :disabled="!!removing" @click="cancelRemove">取消</button><button type="button" class="outline-button danger" :disabled="!!removing" @click="confirmRemove">{{ removing ? '移除中…' : '确认移除' }}</button></div><p v-if="error" class="favorites-error" role="alert">{{ error }} <button type="button" class="outline-button" @click="retry">重试</button></p><p v-if="busy" role="status">正在加载收藏…</p><p v-else-if="!error && !entries.length" role="status">暂无收藏。右键消息选择“收藏”即可保存。</p><p v-else-if="query && !filtered.length" role="status">已加载的收藏中没有匹配结果。</p><div class="favorites-list" :aria-busy="busy"><article v-for="item in filtered" :key="item.id" class="favorite-entry" :data-favorite-id="item.id"><header class="favorite-source"><strong>{{ item.conversation_name || '原会话' }}</strong><span>{{ item.conversation_kind === 'group' ? '群聊' : '私聊' }} · {{ item.is_own ? '我发送的消息' : '收到的消息' }}</span><button type="button" class="text-button" :disabled="!!removing" :aria-label="'移除收藏 ' + (item.sender_name || '')" @click="askRemove(item)">移除</button></header><div class="message-row" :class="{ mine: item.is_own }"><span class="avatar tiny" aria-hidden="true">{{ (item.sender_name || '成员').slice(0,1) }}</span><div><div class="message-meta">{{ item.sender_name || '成员' }} · {{ date(item.message?.created_at) }}</div><SavedMessageContent :favorite="item" /></div></div><footer class="favorite-saved-at">收藏于 {{ date(item.saved_at) }}</footer></article></div><button v-if="hasMore && !error" type="button" class="outline-button favorites-more" :disabled="busy" @click="load(true)">{{ busy ? '加载中…' : '加载更多收藏' }}</button></section></div></Teleport>`,
}
