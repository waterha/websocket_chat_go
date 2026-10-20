// Thread presentation rules: the main chat view shows a bounded preview under a
// root message, and the window shows every reply. Kept separate from the
// component so the counting and ordering rules can be unit tested.

// How many replies the chat view shows before the user opens the window. The
// server caps its preview at the same number, so the two agree.
export const THREAD_PREVIEW_LIMIT = 3

// Whether a message should offer a way into its window. A message with no
// replies has no window, so the affordance stays hidden.
export function hasThread(message) {
  return Boolean(message?.thread && message.thread.reply_count > 0)
}

// The replies to show inline, already bounded by the server.
export function threadPreview(message) {
  return message?.thread?.preview || []
}

// The label for the expand affordance. It names the remainder so the user knows
// what opening the window will reveal.
export function threadLabel(message) {
  if (!hasThread(message)) return ''
  const count = message.thread.reply_count
  const shown = threadPreview(message).length
  if (count > shown) return `查看全部 ${count} 条回复`
  return `查看 ${count} 条回复`
}

// Replies that exist beyond the inline preview.
export function hiddenReplyCount(message) {
  if (!hasThread(message)) return 0
  return Math.max(0, message.thread.reply_count - threadPreview(message).length)
}

// The window decides whether more replies exist beyond what was fetched.
export function threadHasMore(page) {
  return Boolean(page?.has_more)
}

// Opening a window from a reply must show the same window as opening it from the
// root, because the server resolves a reply to its root. This picks the id to
// request so the client does not depend on the server doing that.
export function threadRootID(message) {
  return message?.thread_root_id || message?.id || ''
}

// A reply inside a window addresses the window's root, not the intermediate
// reply, so the thread stays flat.
export function replyTargetForWindow(page) {
  return page?.root || null
}
