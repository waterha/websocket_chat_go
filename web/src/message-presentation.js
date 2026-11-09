import './message-presentation.css'

import { isEmojiOnly } from './emoji-content.js'
export { isEmojiOnly } from './emoji-content.js'
export function messagePresentation(message) {
  const normal = { 'borderless-image': false, 'borderless-emoji': false }
  if (!message || message.recalled_at || message.kind === 'system') return normal
  const blocks = Array.isArray(message.blocks) && message.blocks.length ? message.blocks : null
  if (blocks) {
    const meaningful = blocks.filter(block => block.type !== 'text' || String(block.text || '').trim())
    return {
      'borderless-image': meaningful.length > 0 && meaningful.every(block => block.type === 'image'),
      'borderless-emoji': meaningful.length > 0 && meaningful.every(block => block.type === 'text') && isEmojiOnly(meaningful.map(block => block.text).join('')),
    }
  }
  if (message.kind === 'image') return { ...normal, 'borderless-image': !String(message.content || '').trim() }
  if (message.media_id || ['file', 'video', 'audio'].includes(message.kind)) return normal
  return { ...normal, 'borderless-emoji': isEmojiOnly(message.content) }
}
