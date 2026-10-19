import { filterDirectory, friendSearchFields } from './directory-search.js'

// Members keep a stable order that does not depend on the server response order:
// the signed-in user first, then everyone else by name.
export function sortMembers(members, selfID) {
  return [...(members || [])].sort((left, right) => {
    if (left.id === selfID) return -1
    if (right.id === selfID) return 1
    return (left.name || '').localeCompare(right.name || '', 'zh-CN') || (left.id || '').localeCompare(right.id || '')
  })
}

export function filterMembers(members, query, selfID) {
  return filterDirectory(sortMembers(members, selfID), query, friendSearchFields)
}