import { api } from './api.js'
import { isDesktop } from './desktop.js'
import { isEmojiOnly } from './emoji-content.js'

export function stickerCandidate(message, selectedMediaID = '') {
  if (!message?.id || message.recalled_at || message.kind === 'system') return null
  const blocks = Array.isArray(message.blocks) && message.blocks.length ? message.blocks : null
  const images = blocks ? blocks.filter(block => block.type === 'image' && block.media_id).map(block => block.media_id)
    : message.kind === 'image' && message.media_id ? [message.media_id] : []
  if (selectedMediaID) return images.includes(selectedMediaID) ? { media_id: selectedMediaID } : null
  if (images.length === 1) return { media_id: images[0] }
  if (images.length) return null
  if (message.media_id || ['file', 'video', 'audio'].includes(message.kind)) return null
  if (blocks && blocks.some(block => block.type !== 'text')) return null
  const text = blocks ? blocks.map(block => block.text || '').join('') : message.content || ''
  return isEmojiOnly(text) ? { text } : null
}

const stickerErrors = {
  unsupported_media_type: '请选择 PNG、JPEG、GIF 或 WebP 图片',
  media_too_large: '表情图片不能超过 10 MB',
  invalid_upload: '图片上传失败，请重新选择有效图片',
  sticker_limit: '表情库已达到200项上限',
  invalid_sticker_emoji: '只能添加较短的纯表情消息（最多64个Unicode字符）',
  invalid_sticker_media: '该图片无法添加为表情，请选择其他图片',
  cannot_sticker: '该消息已撤回或不支持添加为表情',
  not_found: '表情或来源消息已不可用，请刷新重试',
  forbidden: '当前会话没有发送权限',
  member_muted: '你已被禁言，暂时无法发送表情',
  conversation_muted: '当前群聊已开启全员禁言',
  conversation_dismissed: '该群聊已解散，无法发送表情',
  blocked_by_user: '当前好友关系不允许发送消息',
}
async function stickerRequest(path, options) {
  try { return await api(path, options) }
  catch (cause) {
    if (stickerErrors[cause.code]) cause.message = stickerErrors[cause.code]
    throw cause
  }
}
export function useStickers() {
  const pending = new Map()
  async function list() { const page = await stickerRequest('/stickers'); return page.stickers || [] }
  function save(message, conversationID, mediaID) {
    const candidate = stickerCandidate(message, mediaID)
    if (!candidate) return Promise.reject(new Error('请右键图片或纯表情消息添加'))
    const key = `${conversationID}:${message.id}:${candidate.media_id || ''}`
    if (!pending.has(key)) pending.set(key, stickerRequest('/stickers', { method: 'POST', body: JSON.stringify({ conversation_id: conversationID, message_id: message.id, ...(candidate.media_id ? { media_id: candidate.media_id } : {}) }) }).finally(() => pending.delete(key)))
    return pending.get(key)
  }
  function prepare(sticker, conversationID) {
    return stickerRequest(`/stickers/${encodeURIComponent(sticker.id)}/prepare`, { method: 'POST', body: JSON.stringify({ conversation_id: conversationID }) })
  }
  async function upload(file) {
    if (!file || file.size <= 0 || file.size > 10 * 1024 * 1024) throw new Error('表情图片不能为空且不能超过 10 MB')
    if (!['image/png', 'image/jpeg', 'image/gif', 'image/webp'].includes(file.type)) throw new Error('请选择 PNG、JPEG、GIF 或 WebP 图片')
    return stickerRequest(`/stickers/upload?${new URLSearchParams({ file_name: file.name || 'sticker.png' })}`, {
      method: 'POST', body: file,
      headers: { 'Content-Type': file.type, ...(isDesktop() ? { 'X-Upload-Size': String(file.size) } : {}) },
    })
  }
  async function remove(sticker) {
    const token = document.cookie.match(/(?:^|;\s*)csrf=([^;]*)/)
    const response = await fetch(`/api/v1/stickers/${encodeURIComponent(sticker.id)}`, {
      method: 'DELETE', credentials: 'same-origin',
      headers: token ? { 'X-CSRF-Token': decodeURIComponent(token[1]) } : {},
    })
    if (!response.ok) {
      const body = await response.json().catch(() => ({}))
      throw new Error(stickerErrors[body.error?.code] || '删除表情失败，请重试')
    }
  }
  return { list, save, prepare, upload, remove }
}
