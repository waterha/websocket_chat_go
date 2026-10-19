import test from 'node:test'
import assert from 'node:assert/strict'
import { conversationSearchFields, filterDirectory, friendSearchFields } from './directory-search.js'

const friends = [
  { id: '1', name: 'Alice Zhang', email: 'alice@example.com' },
  { id: '2', name: '张小明', email: 'ming@example.com' },
]

test('friend directory search matches names and accounts case-insensitively', () => {
  assert.deepEqual(filterDirectory(friends, ' ALICE ', friendSearchFields), [friends[0]])
  assert.deepEqual(filterDirectory(friends, 'MING@EXAMPLE', friendSearchFields), [friends[1]])
  assert.deepEqual(filterDirectory(friends, '张小', friendSearchFields), [friends[1]])
})

test('group directory search matches group name only and ignores surrounding whitespace', () => {
  const groups = [
    { id: 'g1', kind: 'group', name: '项目研发群', members: [{ id: '1', name: 'Ada', email: 'ada@example.com' }] },
    { id: 'g2', kind: 'group', name: '周末读书会', members: [] },
  ]
  assert.deepEqual(filterDirectory(groups, '  研发  ', item => conversationSearchFields(item, 'self')), [groups[0]])
  assert.deepEqual(filterDirectory(groups, 'ada', item => conversationSearchFields(item, 'self')), [])
})

test('direct conversation search matches the other friend, never the current user', () => {
  const direct = { id: 'd1', kind: 'direct', members: [
    { id: 'self', name: 'My Private Account', email: 'private@example.com' },
    friends[0],
  ] }
  const search = item => conversationSearchFields(item, 'self')
  assert.deepEqual(filterDirectory([direct], 'alice', search), [direct])
  assert.deepEqual(filterDirectory([direct], 'private@example.com', search), [])
})

test('empty query keeps order and unmatched query returns an empty list', () => {
  assert.deepEqual(filterDirectory(friends, '  ', friendSearchFields), friends)
  assert.deepEqual(filterDirectory(friends, 'not-found', friendSearchFields), [])
})
