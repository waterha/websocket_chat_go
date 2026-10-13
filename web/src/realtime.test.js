import test from 'node:test'
import assert from 'node:assert/strict'
import { appendPage, Requests, SyncTasks } from './realtime.js'

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
