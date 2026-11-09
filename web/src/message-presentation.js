import './message-presentation.css'

// Match complete emoji sequences, not ordinary digits, punctuation or mixed prose.
const emojiSequence = /^(?:\p{Extended_Pictographic}(?:\uFE0F|\p{Emoji_Modifier})?(?:\u200D\p{Extended_Pictographic}(?:\uFE0F|\p{Emoji_Modifier})?)*|\p{Regional_Indicator}{2}|[0-9#*]\uFE0F?\u20E3)+$/u
export function isEmojiOnly(text) {
  const compact = String(text || '').replace(/\s/gu, '')
  return compact.length > 0 && emojiSequence.test(compact)
}
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
