// Conversation preference helpers: pin, mute and archive are private to each
// member, so the sidebar ordering and badge rules live here and are unit tested
// without rendering the chat component.

// Pinned conversations come first, ordered by most recent pin; the rest keep the
// server's recency order. Archived conversations are filtered out unless the
// user explicitly asks to see them.
export function orderConversations(conversations) {
  return (conversations || []).filter(item => !item.archived)
}

export function groupByPin(conversations) {
  const items = orderConversations(conversations)
  return {
    pinned: items.filter(item => item.pinned),
    recent: items.filter(item => !item.pinned),
  }
}

export function archivedConversations(conversations) {
  return (conversations || []).filter(item => item.archived)
}

// A muted conversation never raises an unread badge, but it still counts as
// unread internally so the user can find it when they choose to.
export function showsUnreadBadge(conversation, count) {
  if (!count || count < 1) return false
  return !conversation?.muted
}

export function unreadLabel(count) {
  return count > 99 ? '99+' : String(count)
}

// The mute label tells the user what muting actually does, so it is not mistaken
// for leaving or blocking.
export const MUTE_HINT = '免打扰只影响本机提示，消息仍会正常送达并计入未读。'

export function preferenceSummary(conversation) {
  const labels = []
  if (conversation?.pinned) labels.push('已置顶')
  if (conversation?.muted) labels.push('免打扰')
  if (conversation?.archived) labels.push('已归档')
  return labels
}
