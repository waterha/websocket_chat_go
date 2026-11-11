import test from 'node:test'
import assert from 'node:assert/strict'
import { performance } from 'node:perf_hooks'
import { buildConversationIndex, latestVisibleMessages, sortConversationsByLatest } from './conversation-index.js'

test('conversation index preserves first direct match, first member name and non-thread previews without copying history', () => {
  const conversations = [{ id: 'one', kind: 'direct', members: [{ id: 'me', name: 'Me' }, { id: 'peer', name: 'Original' }] }, { id: 'two', kind: 'direct', members: [{ id: 'peer', name: 'Later' }] }, { id: 'group', kind: 'group', members: [] }]
  const root = { id: 'root', created_at: '2026-01-01T00:00:00Z' }
  const history = Object.freeze([root, { id: 'reply', thread_root_id: 'root', created_at: '2026-01-02T00:00:00Z' }])
  const index = buildConversationIndex(conversations, 'me', new Map([['one', history]]))
  assert.equal(index.byID.get('one'), conversations[0])
  assert.equal(index.directByUser.get('peer'), conversations[0])
  assert.equal(index.directByUser.has('me'), false)
  assert.equal(index.memberNames.get('peer'), 'Original')
  assert.equal(index.latestByID.get('one'), root)
  assert.equal(index.latestByID.has('group'), false)
  assert.equal(history[1].id, 'reply')
})

test('visible message windows stop after the requested non-thread rows and retain chronology', () => {
  const root = { id: 'visible' }, reply = { id: 'hidden', thread_root_id: 'root' }
  const history = [root, reply, { id: 'latest' }]
  assert.deepEqual(latestVisibleMessages(history, 2), [root, history[2]])
  assert.deepEqual(latestVisibleMessages(history, 0), [])
  let reads = 0
  const longHistory = new Proxy(Array.from({ length: 10000 }, (_, id) => ({ id })), { get(target, key) { if (/^\d+$/.test(String(key))) reads++; return target[key] } })
  const visible = latestVisibleMessages(longHistory, 100)
  assert.equal(reads, 100)
  assert.equal(visible[0].id, 9900)
  assert.equal(visible.at(-1).id, 9999)
})

test('many-conversation sorting scans history once, not from every comparison or rendered row', t => {
  const count = 5000, conversations = [], cache = new Map()
  for (let i = 0; i < count; i++) {
    conversations.push({ id: `c-${i}`, kind: i % 5 ? 'direct' : 'group', name: `群${i}`, members: [{ id: 'me', name: 'Me' }, { id: `u-${i}`, name: `User${i}` }] })
    const history = Array.from({ length: 100 }, (_, n) => ({ id: `${i}-${n}`, created_at: new Date(1700000000000 + i * 1000 + n).toISOString() }))
    cache.set(`c-${i}`, history)
  }
  let historyReads = 0
  const countedCache = { get(id) { historyReads++; return cache.get(id) } }
  const started = performance.now(), index = buildConversationIndex(conversations, 'me', countedCache)
  const actual = sortConversationsByLatest([...conversations], index)
  for (let i = 0; i < 1000; i++) { const item = index.directByUser.get(`u-${i}`); if (i % 5) assert.equal(item.id, `c-${i}`) }
  assert.equal(historyReads, count)
  assert.equal(actual[0].id, `c-${count - 1}`)
  assert.equal(actual.at(-1).id, 'c-0')
  t.diagnostic(`5000 conversations / 500000 messages: build+sort+1000 lookups ${(performance.now() - started).toFixed(2)}ms, ${historyReads} cache reads, no history copies`)
  const oldStarted = performance.now()
  const old = [...conversations].sort((a, b) => Date.parse([...cache.get(b.id)].reverse().find(m => !m.thread_root_id)?.created_at || '') - Date.parse([...cache.get(a.id)].reverse().find(m => !m.thread_root_id)?.created_at || ''))
  for (let i = 0; i < 1000; i++) conversations.find(item => item.kind === 'direct' && item.members.some(member => member.id === `u-${i}`))
  t.diagnostic(`same legacy work ${(performance.now() - oldStarted).toFixed(2)}ms; timing is diagnostic, not a CI threshold`)
  assert.deepEqual(actual.map(item => item.id), old.map(item => item.id))
})
