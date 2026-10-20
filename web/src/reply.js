// Reply, mention and recall presentation rules, kept separate from the main
// component so they can be unit tested without rendering the chat.

// The excerpt a quoted message shows in a reply. A recalled original exposes
// nothing, and a media original is described instead of quoted.
export function replyExcerpt(message) {
  if (!message) return ''
  if (message.recalled) return ''
  if (message.media_type === 'image') return '[图片]'
  if (message.media_type === 'video') return '[视频]'
  return message.excerpt || ''
}

export function replyLabel(message, nameOf) {
  if (!message) return ''
  const name = nameOf?.(message.sender_id) || '成员'
  return message.recalled ? `${name} 撤回了一条消息` : name
}

// A message is recallable by its own sender within the window, or by a group
// owner/admin at any time. The window is advisory: the server is authoritative.
export const RECALL_WINDOW_MS = 2 * 60 * 1000

export function canRecall(message, userID, conversation, now = Date.now()) {
  if (!message || message.recalled_at) return false
  if (message.kind === 'system') return false
  if (message.sender_id === userID) {
    const sent = Date.parse(message.created_at || '')
    if (Number.isNaN(sent)) return true
    return now - sent <= RECALL_WINDOW_MS
  }
  if (conversation?.kind !== 'group') return false
  const role = (conversation.members || []).find(member => member.id === userID)?.role
  return role === 'owner' || role === 'admin'
}

// Recall removes the body but keeps the position, so history stays contiguous.
export function recalledLabel(message, userID) {
  if (!message?.recalled_at) return ''
  return message.sender_id === userID ? '你撤回了一条消息' : '消息已撤回'
}

export function messageBody(message) {
  if (message?.recalled_at) return ''
  return message?.content || ''
}

// Mention candidates are the other members, excluding the signed-in user.
export function mentionCandidates(members, selfID, query = '') {
  const needle = String(query || '').trim().toLowerCase()
  return (members || [])
    .filter(member => member.id !== selfID)
    .filter(member => !needle || `${member.name || ''} ${member.email || ''}`.toLowerCase().includes(needle))
    .sort((left, right) => (left.name || '').localeCompare(right.name || '', 'zh-CN'))
}

// The active "@..." token immediately before the caret, if any.
export function activeMentionToken(draft, caret) {
  const text = String(draft || '').slice(0, Number(caret) || 0)
  const at = text.lastIndexOf('@')
  if (at < 0) return null
  const token = text.slice(at + 1)
  if (/[\s\n]/.test(token)) return null
  return { start: at, query: token }
}

// Inserting a mention replaces the active token and leaves a trailing space.
export function applyMention(draft, token, name) {
  const text = String(draft || '')
  const caret = token ? token.start : text.length
  const before = text.slice(0, caret)
  const after = text.slice(caret)
  const remainder = after.replace(/^@[^\s\n]*/, '')
  return { text: `${before}@${name} ${remainder}`, caret: before.length + name.length + 2 }
}
