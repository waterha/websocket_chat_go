import test from 'node:test'
import assert from 'node:assert/strict'
import { filterMembers, sortMembers } from './member-list.js'

const members = [
  { id: 'u3', name: 'zoe', email: 'zoe@example.com' },
  { id: 'u2', name: 'bo', email: 'bo@example.com' },
  { id: 'self', name: '王芳', email: 'wang@example.com' },
  { id: 'u4', name: 'anna', email: 'anna@example.com' },
]
const ids = items => items.map(member => member.id)

test('sorted members put the signed-in user first and do not mutate the source', () => {
  const source = members.map(member => ({ ...member }))
  const ordered = sortMembers(source, 'self')
  assert.equal(ordered[0].id, 'self')
  assert.deepEqual([...ids(ordered)].sort(), ['self', 'u2', 'u3', 'u4'])
  assert.deepEqual(ids(source), ['u3', 'u2', 'self', 'u4'])
})

test('members sharing a script are ordered by name', () => {
  const ordered = sortMembers(members.filter(member => member.id !== 'self'), 'self')
  assert.deepEqual(ids(ordered), ['u4', 'u2', 'u3'])
})

test('mixed-script ordering is stable and keeps every member', () => {
  const first = ids(sortMembers(members, null))
  const second = ids(sortMembers(members, null))
  assert.deepEqual(first, second)
  assert.deepEqual([...first].sort(), ['self', 'u2', 'u3', 'u4'])
})

test('filtering matches member names and accounts, ignoring case and whitespace', () => {
  assert.deepEqual(ids(filterMembers(members, ' ANNA ', 'self')), ['u4'])
  assert.deepEqual(ids(filterMembers(members, 'bo@example', 'self')), ['u2'])
  assert.deepEqual(ids(filterMembers(members, '王', 'self')), ['self'])
  assert.deepEqual(ids(filterMembers(members, 'wang@example', 'u2')), ['self'])
  assert.deepEqual(filterMembers(members, 'missing@example', 'self'), [])
})

test('an empty query keeps every member in sorted order and unknown text matches none', () => {
  const all = filterMembers(members, '   ', 'self')
  assert.deepEqual(ids(all), ids(sortMembers(members, 'self')))
  assert.equal(all.length, members.length)
  assert.deepEqual(filterMembers(members, 'nobody', 'self'), [])
  assert.deepEqual(filterMembers(undefined, '', 'self'), [])
  assert.deepEqual(filterMembers([], 'a', 'self'), [])
})