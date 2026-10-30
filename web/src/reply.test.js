import test from 'node:test'
import assert from 'node:assert/strict'
import { activeMentionToken, applyMention, canRecall, mentionCandidates, messageBody, recalledLabel, replyExcerpt, replyLabel } from './reply.js'

const group = (roles) => ({ kind: 'group', members: Object.entries(roles).map(([id, role]) => ({ id, role })) })

test('reply excerpts describe recalled and media originals', () => {
  assert.equal(replyExcerpt(null), '')
  assert.equal(replyExcerpt({ excerpt: 'hello' }), 'hello')
  assert.equal(replyExcerpt({ excerpt: 'hello', recalled: true }), '')
  assert.equal(replyExcerpt({ media_type: 'image', excerpt: '' }), '[图片]')
  assert.equal(replyExcerpt({ media_type: 'video', excerpt: '' }), '[视频]')
  assert.equal(replyLabel({ sender_id: 'a', recalled: true }, () => '小明'), '小明 撤回了一条消息')
  assert.equal(replyLabel({ sender_id: 'a' }, () => '小明'), '小明')
  assert.equal(replyLabel({ sender_id: 'a' }, undefined), '成员')
})

test('recall is limited to the sender window or a group moderator', () => {
  const now = Date.parse('2026-10-20T10:00:00Z')
  const fresh = { sender_id: 'me', created_at: '2026-10-20T09:59:30Z', kind: 'user' }
  const stale = { sender_id: 'me', created_at: '2026-10-20T09:50:00Z', kind: 'user' }
  assert.equal(canRecall(fresh, 'me', group({ me: 'member' }), now), true)
  assert.equal(canRecall(stale, 'me', group({ me: 'member' }), now), false)
  assert.equal(canRecall(stale, 'me', group({ me: 'owner' }), now), true)
  assert.equal(canRecall(stale, 'me', group({ me: 'admin' }), now), true)
  const ancient = { ...stale, created_at: '2020-01-01T00:00:00Z' }
  assert.equal(canRecall(ancient, 'boss', group({ boss: 'owner', me: 'member' }), now), true)
  assert.equal(canRecall(ancient, 'admin', group({ admin: 'admin', me: 'member' }), now), true)
  assert.equal(canRecall(stale, 'me', { kind: 'direct', members: [{ id: 'me', role: 'owner' }] }, now), false)
  // Moderators may recall others at any age.
  assert.equal(canRecall(stale, 'boss', group({ boss: 'owner', me: 'member' }), now), true)
  assert.equal(canRecall(stale, 'admin', group({ admin: 'admin', me: 'member' }), now), true)
  assert.equal(canRecall(stale, 'peer', group({ peer: 'member', me: 'member' }), now), false)
  // Outside a group even a stale other-authored message is not recallable.
  assert.equal(canRecall(stale, 'boss', { kind: 'direct', members: [] }, now), false)
  // Already recalled, system messages and missing messages are never recallable.
  assert.equal(canRecall({ ...stale, recalled_at: '2026-10-20T10:00:00Z' }, 'boss', group({ boss: 'owner' }), now), false)
  assert.equal(canRecall({ sender_id: 'me', kind: 'system', created_at: '2026-10-20T09:59:30Z' }, 'me', group({ me: 'member' }), now), false)
  assert.equal(canRecall(null, 'me', group({ me: 'member' }), now), false)
})

test('recalled messages hide their body and are labelled per viewer', () => {
  assert.equal(messageBody({ content: 'hi' }), 'hi')
  assert.equal(messageBody({ content: 'hi', recalled_at: 'x' }), '')
  assert.equal(messageBody(null), '')
  assert.equal(recalledLabel({ recalled_at: 'x', sender_id: 'me' }, 'me'), '你撤回了一条消息')
  assert.equal(recalledLabel({ recalled_at: 'x', sender_id: 'other' }, 'me'), '消息已撤回')
  assert.equal(recalledLabel({}, 'me'), '')
})

test('moderator recalls name the actual operator for every viewer', () => {
  const recalled = { recalled_at: 'x', sender_id: 'member', recalled_by: 'admin', recalled_by_name: '小明' }
  assert.equal(recalledLabel(recalled, 'member'), '小明撤回了一条成员消息')
  assert.equal(recalledLabel(recalled, 'admin'), '小明撤回了一条成员消息')
  assert.equal(recalledLabel({ ...recalled, recalled_by_name: '' }, 'peer', () => '群主'), '群主撤回了一条成员消息')
  assert.equal(recalledLabel({ ...recalled, recalled_by_name: '' }, 'peer'), '群管理员撤回了一条成员消息')
  assert.equal(recalledLabel({ ...recalled, recalled_by: 'member' }, 'member'), '你撤回了一条消息')
})

test('mention candidates exclude self and filter by name or account', () => {
  const members = [
    { id: 'me', name: '我', email: 'me@test' },
    { id: 'b', name: 'Bob', email: 'bob@test' },
    { id: 'c', name: 'Cindy', email: 'cindy@test' },
  ]
  assert.deepEqual(mentionCandidates(members, 'me').map(m => m.id), ['b', 'c'])
  assert.deepEqual(mentionCandidates(members, 'me', 'bob').map(m => m.id), ['b'])
  assert.deepEqual(mentionCandidates(members, 'me', 'CINDY@').map(m => m.id), ['c'])
  assert.deepEqual(mentionCandidates(members, 'me', 'zzz'), [])
  assert.deepEqual(mentionCandidates(null, 'me'), [])
})

test('the active mention token is the text after the caret-last @', () => {
  assert.deepEqual(activeMentionToken('hello @bo', 10), { start: 6, query: 'bo' })
  assert.deepEqual(activeMentionToken('@', 1), { start: 0, query: '' })
  assert.equal(activeMentionToken('hi there', 8), null)
  assert.equal(activeMentionToken('@bo done', 9), null)
  assert.equal(activeMentionToken('', 0), null)
})

test('applying a mention replaces the token and leaves a trailing space', () => {
  assert.deepEqual(applyMention('hello @bo', { start: 6, query: 'bo' }, 'Bob'), { text: 'hello @Bob ', caret: 11 })
  assert.deepEqual(applyMention('@bo tail', { start: 0, query: 'bo' }, 'Bob'), { text: '@Bob  tail', caret: 5 })
  assert.deepEqual(applyMention('plain', null, 'Bob'), { text: 'plain@Bob ', caret: 10 })
})
