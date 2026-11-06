import { ref, watch, nextTick, onScopeDispose } from 'vue/dist/vue.esm-bundler.js'
import { mediaURL } from './media.js'

const EMPTY = () => ({ blocks: [], images: [], attachment: null })
const validID = value => typeof value === 'string' && value.length > 0 && value.length <= 200

// Persist structured content, never editor HTML or temporary blob URLs.
export function normalizeDraft(value) {
  const images = (Array.isArray(value?.images) ? value.images : []).filter(image => validID(image?.mediaId)).slice(0, 20)
    .map((image, index) => ({ index: index + 1, mediaId: image.mediaId, fileName: String(image.fileName || '图片').slice(0, 500) }))
  const ids = new Set(images.map(image => image.mediaId))
  const blocks = (Array.isArray(value?.blocks) ? value.blocks : []).slice(0, 1000).flatMap(block => {
    if (block?.type === 'image' && ids.has(block.media_id)) return [{ type: 'image', media_id: block.media_id }]
    if (block?.type === 'text' && typeof block.text === 'string') return [{ type: 'text', text: block.text.slice(0, 100000) }]
    return []
  })
  const media = value?.attachment
  const attachment = validID(media?.id) ? { id: media.id, file_name: String(media.file_name || '文件').slice(0, 500), media_type: media.media_type, content_type: media.content_type, byte_size: media.byte_size } : null
  return { blocks, images, attachment }
}
export const draftFingerprint = value => JSON.stringify(normalizeDraft(value))
const hasContent = value => Boolean(value.attachment || value.blocks.some(block => block.type === 'image' || block.text.length))

export class ConversationDraftStore {
  constructor(storage, onError = () => {}) { this.storage = storage; this.onError = onError; this.memory = new Map(); this.warned = false }
  key(userID, conversationID) { return `im.draft.v1:${encodeURIComponent(userID)}:${encodeURIComponent(conversationID)}` }
  read(userID, conversationID) {
    if (!userID || !conversationID) return EMPTY()
    const key = this.key(userID, conversationID)
    if (this.memory.has(key)) return normalizeDraft(this.memory.get(key))
    try { return normalizeDraft(JSON.parse(this.storage.getItem(key) || 'null')) } catch { return EMPTY() }
  }
  write(userID, conversationID, snapshot) {
    if (!userID || !conversationID) return
    const key = this.key(userID, conversationID), clean = normalizeDraft(snapshot)
    this.memory.set(key, clean)
    try {
      if (hasContent(clean)) this.storage.setItem(key, JSON.stringify(clean))
      else this.storage.removeItem(key)
    } catch {
      if (!this.warned) { this.warned = true; this.onError(new Error('浏览器存储不可用，草稿暂时仅保留在本次窗口中')) }
    }
  }
  clearIfMatches(userID, conversationID, expected) {
    if (draftFingerprint(this.read(userID, conversationID)) === draftFingerprint(expected)) this.write(userID, conversationID, EMPTY())
  }
}

// The root owns editor rendering; this module owns persistence and account/conversation lifetimes.
export function useConversationDrafts({ active, userID, storage, sources, capture, clear, restore, showError }) {
  const revision = ref(0)
  const store = new ConversationDraftStore(storage, showError)
  let owner = null, restoring = false, generation = 0, timer
  function saveCurrent() {
    clearTimeout(timer)
    if (restoring || !owner) return
    store.write(owner.userID, owner.conversationID, capture())
    revision.value++
  }
  watch(sources, () => {
    if (!restoring && owner) { clearTimeout(timer); timer = setTimeout(saveCurrent, 180) }
  }, { deep: true, flush: 'post' })
  watch([active, userID], async ([conversationID, accountID]) => {
    saveCurrent()
    const version = ++generation
    owner = accountID && conversationID ? { userID: accountID, conversationID } : null
    restoring = true
    clear()
    const snapshot = owner ? store.read(accountID, conversationID) : EMPTY()
    await nextTick()
    if (version !== generation) return
    restore({ ...snapshot, images: snapshot.images.map(image => ({ ...image, previewURL: mediaURL(image.mediaId) })) })
    restoring = false
  }, { flush: 'sync', immediate: true })
  function clearSent(accountID, conversationID, expected) {
    store.clearIfMatches(accountID, conversationID, expected)
    revision.value++
  }
  function hasDraft(conversationID) {
    revision.value
    return Boolean(userID.value && hasContent(store.read(userID.value, conversationID)))
  }
  const onHidden = () => { if (document.visibilityState === 'hidden') saveCurrent() }
  window.addEventListener('pagehide', saveCurrent)
  document.addEventListener('visibilitychange', onHidden)
  onScopeDispose(() => {
    saveCurrent(); generation++; clearTimeout(timer)
    window.removeEventListener('pagehide', saveCurrent)
    document.removeEventListener('visibilitychange', onHidden)
  })
  function draftPreview(conversationID) {
    revision.value
    if (!userID.value) return ''
    const snapshot = store.read(userID.value, conversationID)
    if (!hasContent(snapshot)) return ''
    const text = snapshot.blocks.filter(block => block.type === 'text').map(block => block.text).join('').trim()
    return '[草稿] ' + (text || snapshot.attachment?.file_name || '[图片]')
  }
  return { saveCurrent, clearSent, hasDraft, draftPreview }
}
