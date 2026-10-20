import test from 'node:test'
import assert from 'node:assert/strict'
import { appendPage, PendingSends, Requests, SyncTasks } from './realtime.js'

const message = seq => ({ id: String(seq), conversation_id: 'chat', seq })

test('sync advances only contiguous pages for the correct conversation', () => {
  const previous = [message(1)]
  assert.deepEqual(appendPage(previous, { messages: [message(2)], next_seq: 2, has_more: true }, 'chat'), [message(1), message(2)])
  assert.throws(() => appendPage(previous, { messages: [message(3)], next_seq: 3 }, 'chat'))
  assert.throws(() => appendPage(previous, { messages: [message(2)], next_seq: 2 }, 'other'))
  assert.throws(() => appendPage(previous, { messages: [], next_seq: 1, has_more: true }, 'chat'))
  assert.throws(() => appendPage(previous, { messages: [message(2)], next_seq: 3 }, 'chat'))
  assert.equal(previous.length, 1)
})

test('ACK and errors correlate with requests; close rejects outstanding sends', async () => {
  const sent = []
  const requests = new Requests({ readyState: 1, send: payload => sent.push(JSON.parse(payload)) })
  const response = requests.send('message.send', { client_msg_id: 'stable-id' })
  assert.equal(sent[0].data.client_msg_id, 'stable-id')
  assert.equal(requests.receive({ event: 'message.created' }), false)
  requests.receive({ request_id: sent[0].request_id, data: message(1) })
  assert.deepEqual(await response, message(1))
  const rejected = requests.send('message.send', {})
  const failure = assert.rejects(rejected, error => error.application && error.message === 'not_found')
  requests.receive({ request_id: sent[1].request_id, error: { message: 'not_found' } })
  await failure
  const outstanding = requests.send('ping', {})
  const closed = assert.rejects(outstanding, /连接断开/)
  requests.close()
  await closed
  assert.equal(requests.pending.size, 0)
})

test('transport timeout allows retry and removes pending request', async () => {
  const requests = new Requests({ readyState: 1, send() {} }, 5)
  await assert.rejects(requests.send('ping', {}), /超时/)
  assert.equal(requests.pending.size, 0)
})

test('a newer push during an in-flight sync cannot lose the final message', async () => {
  const tasks = new SyncTasks()
  let release
  let calls = 0
  const first = tasks.run('chat', 1, async () => {
    calls++
    if (calls === 1) return new Promise(resolve => { release = resolve })
    return 2
  })
  const second = tasks.run('chat', 2, () => { throw new Error('must share the original task') })
  assert.equal(first, second)
  release(1)
  await second
  assert.equal(calls, 2)
  assert.equal(tasks.pending.size, 0)
})

test('sync failure is retryable; clearing tasks preserves a new session task', async () => {
  const tasks = new SyncTasks()
  await assert.rejects(tasks.run('chat', 1, async () => { throw new Error('network failed') }), /network failed/)
  await tasks.run('chat', 1, async () => 1)
  let finishOld, finishNew
  const old = tasks.run('chat', 1, () => new Promise(resolve => { finishOld = resolve }))
  tasks.clear()
  const current = tasks.run('chat', 2, () => new Promise(resolve => { finishNew = resolve }))
  finishOld(undefined)
  await old
  assert.equal(tasks.pending.size, 1)
  finishNew(2)
  await current
  assert.equal(tasks.pending.size, 0)
})

test('uncertain send retains its id across reload and cannot silently change content', () => {
  const values = new Map()
  const storage = {
    getItem: key => values.get(key) || null,
    setItem: (key, value) => values.set(key, value),
    removeItem: key => values.delete(key),
  }
  const firstTab = new PendingSends(storage, 'alice')
  const pending = firstTab.save('chat', 'important')
  const reloaded = new PendingSends(storage, 'alice')
  assert.deepEqual(reloaded.get('chat'), pending)
  assert.deepEqual(reloaded.save('chat', 'important'), pending)
  assert.throws(() => reloaded.save('chat', 'changed'), /未确认/)
  assert.deepEqual(reloaded.get('chat'), pending)
  assert.equal(new PendingSends(storage, 'bob').get('chat'), null)
  assert.equal(reloaded.get('other'), null)
  reloaded.clear('chat', 'stale-id')
  assert.deepEqual(reloaded.get('chat'), pending)
  reloaded.clear('chat', pending.client_msg_id)
  assert.equal(reloaded.get('chat'), null)
  const next = reloaded.save('other', 'other message')
  assert.notEqual(next.client_msg_id, pending.client_msg_id)
  reloaded.forget()
  assert.equal(reloaded.get('other'), null)
})

test('an attachment resend keeps the same id and refuses a different media', () => {
  const values = new Map()
  const storage = {
    getItem: key => values.get(key) || null,
    setItem: (key, value) => values.set(key, value),
    removeItem: key => values.delete(key),
  }
  const sends = new PendingSends(storage, 'alice')
  const pending = sends.save('chat', 'caption', 'media-1')
  assert.equal(pending.media_id, 'media-1')
  assert.deepEqual(sends.save('chat', 'caption', 'media-1'), pending)
  assert.throws(() => sends.save('chat', 'caption', 'media-2'), /未确认/)
  assert.throws(() => sends.save('chat', 'changed', 'media-1'), /未确认/)
  const textOnly = sends.save('other', 'plain')
  assert.equal(textOnly.media_id, '')
})

// The record is sent verbatim over WebSocket, so every field the server reads
// must be stored on it. Reply and mention fields were previously dropped here,
// which silently lost quoting and mentions on the real send path while the HTTP
// fallback still worked.
test('a pending send carries the reply and mention fields the server reads', () => {
  const values = new Map()
  const storage = {
    getItem: key => values.get(key) || null,
    setItem: (key, value) => values.set(key, value),
    removeItem: key => values.delete(key),
  }
  const sends = new PendingSends(storage, 'alice')
  const pending = sends.save('chat', 'agree', '', 'root-message-id', ['bob-id'])
  assert.equal(pending.reply_to_id, 'root-message-id')
  assert.deepEqual(pending.mentions, ['bob-id'])
  // The payload sent over the socket is this stored object, so it must also carry
  // the conversation, which the server needs to route the send.
  assert.equal(pending.conversation_id, 'chat')
  assert.ok(pending.client_msg_id.length >= 8)
})

test('a reply resend keeps its target and refuses a changed one', () => {
  const values = new Map()
  const storage = {
    getItem: key => values.get(key) || null,
    setItem: (key, value) => values.set(key, value),
    removeItem: key => values.delete(key),
  }
  const sends = new PendingSends(storage, 'alice')
  const pending = sends.save('chat', 'agree', '', 'root-a', [])
  assert.deepEqual(sends.save('chat', 'agree', '', 'root-a', []), pending)
  assert.throws(() => sends.save('chat', 'agree', '', 'root-b', []), /未确认/)
  assert.throws(() => sends.save('chat', 'agree', '', 'root-a', ['bob']), /未确认/)
})
