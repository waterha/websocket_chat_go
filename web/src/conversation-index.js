// Build once per conversation/cache change, not inside each sort comparator or
// rendered row. Values are existing objects, so no message history is copied.
export function buildConversationIndex(conversations, userID, cache) {
  const byID = new Map(), directByUser = new Map(), latestByID = new Map(), memberNames = new Map()
  for (const conversation of conversations) {
    byID.set(conversation.id, conversation)
    for (const member of conversation.members || []) {
      if (!memberNames.has(member.id)) memberNames.set(member.id, member.name)
      if (conversation.kind === 'direct' && member.id !== userID && !directByUser.has(member.id)) directByUser.set(member.id, conversation)
    }
    const history = cache.get(conversation.id) || []
    for (let index = history.length - 1; index >= 0; index--) {
      if (!history[index].thread_root_id) { latestByID.set(conversation.id, history[index]); break }
    }
  }
  return { byID, directByUser, latestByID, memberNames }
}

export function latestVisibleMessages(history, count) {
  const result = []
  for (let index = history.length - 1; index >= 0 && result.length < count; index--) {
    const message = history[index]
    if (!message.thread_root_id) result.push(message)
  }
  return result.reverse()
}

export function sortConversationsByLatest(items, index) {
  const times = new Map(items.map(item => [item.id, Date.parse(index.latestByID.get(item.id)?.created_at || '') || 0]))
  return items.sort((left, right) => times.get(right.id) - times.get(left.id) || (left.name || '').localeCompare(right.name || '', 'zh-CN'))
}
