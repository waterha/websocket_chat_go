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
    const editing = ref(''), draftTags = ref(''), draftNote = ref(''), saving = ref(false), editError = ref('')
    let cursor = '', generation = 0, request, editRequest, editGeneration = 0, previousFocus, retryMore = false
    const matches = item => {
      const needle = query.value.trim().toLocaleLowerCase()
      return !needle || [item.sender_name, item.conversation_name, item.message?.content, item.message?.reply?.excerpt, item.message?.media?.file_name, ...(item.message?.blocks || []).map(block => block.text || block.file_name), ...(item.tags || []), item.note].join('\n').toLocaleLowerCase().includes(needle)
    }
    const filtered = computed(() => entries.value.filter(item => item.id === editing.value || matches(item)))
    const date = value => { const timestamp = new Date(value); return Number.isNaN(timestamp.getTime()) ? '时间未知' : timestamp.toLocaleString('zh-CN') }
    async function load(more = false) {
      if (!props.open || !props.userID || busy.value || editing.value || removeTarget.value || (more && !hasMore.value)) return
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
    function clearEditor() { editGeneration++; editRequest?.abort(); editing.value = ''; draftTags.value = ''; draftNote.value = ''; saving.value = false; editError.value = '' }
    function focusEdit(id) { nextTick(() => { if (props.open) [...(panel.value?.querySelectorAll('.favorite-entry') || [])].find(entry => entry.dataset.favoriteId === id)?.querySelector('.favorite-edit')?.focus() }) }
    function cancelEdit() { if (saving.value) return; const id = editing.value; clearEditor(); focusEdit(id) }
    async function startEdit(item) {
      if (editing.value || removeTarget.value || busy.value) return
      editing.value = item.id; draftTags.value = (item.tags || []).join(', '); draftNote.value = item.note || ''; editError.value = ''
      await nextTick(); panel.value?.querySelector('.favorite-editor input')?.focus()
    }
    async function saveEdit() {
      if (!editing.value || saving.value) return
      const tags = [...new Set(draftTags.value.split(/[,，]/u).map(tag => tag.trim()).filter(Boolean))]
      if (tags.length > 10) { editError.value = '标签最多 10 项'; return }
      if (tags.some(tag => [...tag].length > 20)) { editError.value = '每个标签最多 20 个字符'; return }
      if ([...draftNote.value].length > 2000) { editError.value = '个人备注最多 2000 个字符'; return }
      const id = editing.value, version = generation, editVersion = ++editGeneration, identity = props.userID
      editRequest = new AbortController(); saving.value = true; editError.value = ''
      const current = () => version === generation && editVersion === editGeneration && props.open && identity === props.userID
      try {
        const updated = await api(`/favorites/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify({ tags, note: draftNote.value }), signal: editRequest.signal })
        if (!current()) return
        entries.value = entries.value.map(item => item.id === id ? { ...item, tags: updated.tags || [], note: updated.note || '' } : item)
        if (!matches(entries.value.find(item => item.id === id))) query.value = ''
        clearEditor(); focusEdit(id)
      } catch (cause) { if (current() && cause.name !== 'AbortError') editError.value = '标签与个人备注保存失败，请重试' }
      finally { if (current()) saving.value = false }
    }
    function reset() { generation++; request?.abort(); clearEditor(); busy.value = false; entries.value = []; error.value = ''; hasMore.value = false; cursor = ''; removeTarget.value = null; removing.value = ''; removeError.value = '' }
    function close() { reset(); emit('update:open', false) }
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
    async function askRemove(item) { if (editing.value || removing.value || busy.value) return; removeTarget.value = item; removeError.value = ''; await nextTick(); panel.value?.querySelector('.favorites-cancel-remove')?.focus() }
    function keyboard(event) {
      if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); if (editing.value) { if (!saving.value) cancelEdit() } else if (removeTarget.value) { if (!removing.value) cancelRemove() } else close(); return }
      if (event.key !== 'Tab') return
      const focusScope = removeTarget.value ? panel.value.querySelector('.favorites-confirm') : panel.value
      const controls = [...focusScope.querySelectorAll('button:not(:disabled),input:not(:disabled),textarea:not(:disabled),a[href],audio[controls],video[controls]')]
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
    return { panel, query, entries, filtered, busy, error, hasMore, removing, removeTarget, removeError, editing, draftTags, draftNote, saving, editError, startEdit, saveEdit, cancelEdit, date, load, retry: () => load(retryMore), close, confirmRemove, askRemove, cancelRemove, keyboard }
  },
  template: `<Teleport to="body"><div v-if="open" class="modal-backdrop favorites-backdrop" @click.self="close" @keydown="keyboard"><section ref="panel" class="modal favorites-panel" role="dialog" aria-modal="true" aria-labelledby="favorites-title"><header class="modal-head"><h2 id="favorites-title">收藏</h2><button type="button" class="icon-button" aria-label="关闭收藏" @click="close">×</button></header><div class="favorites-toolbar"><input v-model="query" type="search" aria-label="搜索收藏" placeholder="搜索已加载的消息、姓名、来源、文件名、标签或个人备注" :disabled="!!editing" /><button type="button" class="outline-button" :disabled="busy || !!editing || !!removeTarget" @click="load()">刷新</button></div><p class="subtle">保存的消息快照，原消息撤回或离开会话后仍可查看。搜索范围为已加载的收藏。</p><div v-if="removeTarget" class="favorites-confirm" role="alertdialog" aria-label="移除收藏确认"><p>确定移除这条收藏？移除后无法恢复。</p><p v-if="removeError" role="alert">{{ removeError }}</p><button type="button" class="outline-button favorites-cancel-remove" :disabled="!!removing" @click="cancelRemove">取消</button><button type="button" class="outline-button danger" :disabled="!!removing" @click="confirmRemove">{{ removing ? '移除中…' : '确认移除' }}</button></div><p v-if="error" class="favorites-error" role="alert">{{ error }} <button type="button" class="outline-button" @click="retry">重试</button></p><p v-if="busy" role="status">正在加载收藏…</p><p v-else-if="!error && !entries.length" role="status">暂无收藏。右键消息选择“收藏”即可保存。</p><p v-else-if="query && !filtered.length" role="status">已加载的收藏中没有匹配结果。</p><div class="favorites-list" :aria-busy="busy"><article v-for="item in filtered" :key="item.id" class="favorite-entry" :data-favorite-id="item.id"><header class="favorite-source"><strong>{{ item.conversation_name || '原会话' }}</strong><span>{{ item.conversation_kind === 'group' ? '群聊' : '私聊' }} · {{ item.is_own ? '我发送的消息' : '收到的消息' }}</span><button type="button" class="text-button" :disabled="!!removing || !!editing || busy" :aria-label="'移除收藏 ' + (item.sender_name || '')" @click="askRemove(item)">移除</button></header><div class="favorite-metadata"><div v-if="item.tags?.length" class="favorite-tags" aria-label="收藏标签"><span v-for="tag in item.tags" :key="tag" class="favorite-tag">{{ tag }}</span></div><div v-if="item.note" class="favorite-note"><strong>个人备注</strong><p>{{ item.note }}</p></div><button type="button" class="text-button favorite-edit" :disabled="!!editing || !!removeTarget || busy" @click="startEdit(item)">编辑标签与备注</button><form v-if="editing === item.id" class="favorite-editor" @submit.prevent="saveEdit"><label>标签<input v-model="draftTags" :disabled="saving" placeholder="用逗号或中文逗号分隔" /></label><p class="subtle">最多 10 个标签，每个最多 20 个字符；自动去除首尾空格和重复项。</p><label>个人备注<textarea v-model="draftNote" :disabled="saving" rows="4" placeholder="仅修改个人备注，不会更改原消息快照"></textarea></label><p class="subtle">最多 2000 个字符，保留换行。</p><p v-if="editError" class="favorites-error" role="alert">{{ editError }}</p><div class="favorite-editor-actions"><button type="submit" class="outline-button" :disabled="saving">{{ saving ? '保存中…' : '保存' }}</button><button type="button" class="outline-button favorite-cancel-edit" :disabled="saving" @click="cancelEdit">取消</button></div></form></div><div class="message-row" :class="{ mine: item.is_own }"><span class="avatar tiny" aria-hidden="true">{{ (item.sender_name || '成员').slice(0,1) }}</span><div><div class="message-meta">{{ item.sender_name || '成员' }} · {{ date(item.message?.created_at) }}</div><SavedMessageContent :favorite="item" /></div></div><footer class="favorite-saved-at">收藏于 {{ date(item.saved_at) }}</footer></article></div><button v-if="hasMore && !error" type="button" class="outline-button favorites-more" :disabled="busy" @click="load(true)">{{ busy ? '加载中…' : '加载更多收藏' }}</button></section></div></Teleport>`,
}
