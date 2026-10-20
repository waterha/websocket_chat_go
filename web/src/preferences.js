// Conversation preference helpers: pin and mute are private to each member, so
// the sidebar badge rules live here and are unit tested without rendering the
// chat component.

// Pinned conversations are surfaced separately; the rest keep the server's
// recency order.
export function groupByPin(conversations) {
  const items = conversations || []
  return {
    pinned: items.filter(item => item.pinned),
    recent: items.filter(item => !item.pinned),
  }
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

// The hint tells the user what muting actually does, so it is not mistaken for
// leaving or blocking.
export const MUTE_HINT = '免打扰只影响本机提示，消息仍会正常送达并计入未读。'

export function preferenceSummary(conversation) {
  const labels = []
  if (conversation?.pinned) labels.push('已置顶')
  if (conversation?.muted) labels.push('免打扰')
  return labels
}
