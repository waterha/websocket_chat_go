import test from 'node:test'
import assert from 'node:assert/strict'
import { groupByPin, MUTE_HINT, preferenceSummary, showsUnreadBadge, unreadLabel } from './preferences.js'

const item = (id, extra = {}) => ({ id, ...extra })

test('pinned and unpinned conversations are split without reordering', () => {
  const list = [item('a', { pinned: true }), item('b'), item('c', { pinned: true })]
  const groups = groupByPin(list)
  assert.deepEqual(groups.pinned.map(entry => entry.id), ['a', 'c'])
  assert.deepEqual(groups.recent.map(entry => entry.id), ['b'])
  assert.equal(groups.pinned.length + groups.recent.length, list.length)
  assert.deepEqual(groupByPin(null), { pinned: [], recent: [] })
  assert.deepEqual(groupByPin(undefined), { pinned: [], recent: [] })
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
  assert.deepEqual(preferenceSummary({ pinned: true, muted: true }), ['已置顶', '免打扰'])
  assert.deepEqual(preferenceSummary({ pinned: true }), ['已置顶'])
  assert.deepEqual(preferenceSummary({}), [])
  assert.deepEqual(preferenceSummary(null), [])
  assert.equal(MUTE_HINT.includes('仍会正常送达'), true)
})
