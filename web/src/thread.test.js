import test from 'node:test'
import assert from 'node:assert/strict'
import { hasThread, hiddenReplyCount, replyTargetForWindow, threadHasMore, threadLabel, threadPreview, threadRootID, THREAD_PREVIEW_LIMIT } from './thread.js'

const withThread = (replyCount, previewLength = THREAD_PREVIEW_LIMIT) => ({
  id: 'root',
  thread: { reply_count: replyCount, preview: Array.from({ length: previewLength }, (_, index) => ({ id: 'r' + index, content: 'reply ' + index })) },
})

test('a message offers a window only when it has replies', () => {
  assert.equal(hasThread(withThread(3, 3)), true)
  assert.equal(hasThread({ id: 'plain' }), false)
  assert.equal(hasThread({ id: 'zero', thread: { reply_count: 0, preview: [] } }), false)
  assert.equal(hasThread(null), false)
})

test('the inline preview is bounded by the server payload', () => {
  assert.equal(threadPreview(withThread(9)).length, THREAD_PREVIEW_LIMIT)
  assert.deepEqual(threadPreview({ id: 'plain' }), [])
  assert.deepEqual(threadPreview(null), [])
})

test('the label names how many replies are hidden', () => {
  assert.equal(threadLabel(withThread(9)), '查看全部 9 条回复')
  assert.equal(threadLabel(withThread(3)), '查看全部 3 条回复')
  assert.equal(threadLabel({ id: 'plain' }), '')
  assert.equal(threadLabel(null), '')
})

test('hidden count never goes negative when the preview is complete', () => {
  assert.equal(hiddenReplyCount(withThread(9, 3)), 6)
  assert.equal(hiddenReplyCount(withThread(3, 3)), 0)
  assert.equal(hiddenReplyCount(withThread(2, 3)), 0)
  assert.equal(hiddenReplyCount(null), 0)
})

test('a window opened from a reply requests the same window as the root', () => {
  // The server files a reply under its root, and the client mirrors that so it
  // never asks for a nested window.
  assert.equal(threadRootID({ id: 'reply', thread_root_id: 'root' }), 'root')
  assert.equal(threadRootID({ id: 'root' }), 'root')
  assert.equal(threadRootID(null), '')
})

test('paging and reply targeting read from the window page', () => {
  assert.equal(threadHasMore({ has_more: true }), true)
  assert.equal(threadHasMore({ has_more: false }), false)
  assert.equal(threadHasMore(null), false)
  const page = { root: { id: 'root', content: '你好' }, replies: [] }
  assert.equal(replyTargetForWindow(page).id, 'root')
  assert.equal(replyTargetForWindow(null), null)
})
