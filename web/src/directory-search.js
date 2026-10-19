export function filterDirectory(items, query, fields) {
  const keyword = query.trim().toLowerCase()
  if (!keyword) return items
  return items.filter(item => fields(item).some(value => String(value ?? '').toLowerCase().includes(keyword)))
}

export function friendSearchFields(person) {
  return [person.name, person.email]
}

export function conversationSearchFields(conversation, selfID) {
  if (conversation.kind === 'group') return [conversation.name]
  return conversation.members.filter(member => member.id !== selfID).flatMap(friendSearchFields)
}
