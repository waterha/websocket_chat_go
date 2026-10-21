// Invite-picker helpers: search, initial-letter grouping and selection rules.
//
// The picker can face a long user list, so it needs a bounded scroll area, a
// search box and an A–Z index. Kept separate from the component so the grouping
// and selection rules can be unit tested.

// The synthetic bucket for names that have no Latin initial, such as Chinese
// names. Grouping Chinese names by their actual pinyin would require a full
// dictionary; guessing would silently file people under the wrong letter, so
// they are grouped explicitly under "其他" and remain reachable by search.
export const OTHER_INITIAL = '#'

// Latin letters are the only initials the index offers. A name is grouped by its
// first character when that character is a Latin letter, ignoring case and
// diacritics; everything else falls into OTHER_INITIAL.
export function initialOf(name) {
  const text = String(name || '').trim()
  if (!text) return OTHER_INITIAL
  // Decompose so an accented letter still yields its base letter.
  const first = text[0].normalize('NFD').replace(/[\u0300-\u036f]/g, '')
  const upper = first.toUpperCase()
  if (upper >= 'A' && upper <= 'Z') return upper
  return OTHER_INITIAL
}

// groupByInitial orders people A–Z with the non-Latin bucket last, so the index
// reads like a contacts list and Chinese names are not scattered by locale rules.
export function groupByInitial(people) {
  const groups = new Map()
  for (const person of people || []) {
    const initial = initialOf(person?.name)
    if (!groups.has(initial)) groups.set(initial, [])
    groups.get(initial).push(person)
  }
  const letters = [...groups.keys()].filter(initial => initial !== OTHER_INITIAL).sort()
  if (groups.has(OTHER_INITIAL)) letters.push(OTHER_INITIAL)
  return letters.map(initial => ({
    initial,
    people: groups.get(initial).sort((left, right) => (left.name || '').localeCompare(right.name || '', 'zh-CN') || (left.id || '').localeCompare(right.id || '')),
  }))
}

// The initials actually present, for rendering the index rail.
export function availableInitials(groups) {
  return (groups || []).map(group => group.initial)
}

// filterCandidates applies the search box and the chosen initial together. The
// initial "#" means the non-Latin bucket, so an explicit choice is honoured
// rather than treated as "no filter".
export function filterCandidates(people, query = '', initial = '') {
  const keyword = String(query || '').trim().toLowerCase()
  return (people || []).filter(person => {
    if (initial && initialOf(person?.name) !== initial) return false
    if (!keyword) return true
    return [person?.name, person?.email].some(value => String(value ?? '').toLowerCase().includes(keyword))
  })
}

// toggleSelection adds or removes one id, returning a new array so callers never
// mutate the reactive array in place.
export function toggleSelection(selected, id) {
  const current = selected || []
  return current.includes(id) ? current.filter(value => value !== id) : [...current, id]
}

// keepSelectable drops selections that are no longer offered, which matters when
// someone becomes a member between opening the dialog and sending the invite.
// Selections hidden by a filter are kept: the user only narrowed the view.
export function keepSelectable(selected, candidates) {
  const allowed = new Set((candidates || []).map(person => person.id))
  return (selected || []).filter(id => allowed.has(id))
}

// selectAll adds every candidate that still fits under the member cap.
export function selectAllCandidates(selected, visible, memberCount, cap) {
  const current = selected || []
  const room = Math.max(0, Number(cap) - Number(memberCount) - current.length)
  const additions = (visible || []).filter(person => !current.includes(person.id)).slice(0, room).map(person => person.id)
  return [...current, ...additions]
}

// selectionSummary drives the button label and the disabled state, so the user
// always sees how many people will be invited and why the button is unavailable.
export function selectionSummary(selected, memberCount, cap) {
  const count = (selected || []).length
  const remaining = Math.max(0, Number(cap) - Number(memberCount))
  if (!count) return { label: '邀请加入群聊', disabled: true, count, remaining }
  if (count > remaining) return { label: `最多还能邀请 ${remaining} 人`, disabled: true, count, remaining }
  return { label: `邀请 ${count} 人加入群聊`, disabled: false, count, remaining }
}
