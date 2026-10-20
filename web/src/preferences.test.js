import test from 'node:test'
import assert from 'node:assert/strict'
import { archivedConversations, groupByPin, MUTE_HINT, orderConversations, preferenceSummary, showsUnreadBadge, unreadLabel } from './preferences.js'

const item = (id, extra = {}) => ({ id, ...extra })

test('archived conversations are hidden from the normal list', () => {
  const list = [item('a'), item('b', { archived: true }), item('c')]
  assert.deepEqual(orderConversations(list).map(entry => entry.id), ['a', 'c'])
  assert.deepEqual(archivedConversations(list).map(entry => entry.id), ['b'])
  assert.deepEqual(orderConversations(null), [])
  assert.deepEqual(archivedConversations(undefined), [])
})

test('pinned and unpinned conversations are split without reordering', () => {
  const list = [item('a', { pinned: true }), item('b'), item('c', { pinned: true }), item('d', { archived: true })]
  const groups = groupByPin(list)
  assert.deepEqual(groups.pinned.map(entry => entry.id), ['a', 'c'])
  assert.deepEqual(groups.recent.map(entry => entry.id), ['b'])
  // The archived entry appears in neither group.
  assert.equal(groups.pinned.length + groups.recent.length, 3)
})

test('muted conversations suppress the unread badge but keep the count', () => {
  assert.equal(showsUnreadBadge(item('a'), 5), true)
  assert.equal(showsUnreadBadge(item('a', { muted: true }), 5), false)
  assert.equal(showsUnreadBadge(item('a', { muted: true }), 0), false)
  assert.equal(showsUnreadBadge(item('a'), 0), false)
  assert.equal(showsUnreadBadge(null, 3), true)
  assert.equal(unreadLabel(5), '5')
  assert.equal(unreadLabel(150), '99+')
})

test('preference summaries list active preferences only', () => {
  assert.deepEqual(preferenceSummary({ pinned: true, muted: true, archived: true }), ['已置顶', '免打扰', '已归档'])
  assert.deepEqual(preferenceSummary({}), [])
  assert.deepEqual(preferenceSummary(null), [])
  assert.equal(MUTE_HINT.includes('仍会正常送达'), true)
})
