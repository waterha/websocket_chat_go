import test from 'node:test'
import assert from 'node:assert/strict'
import { appendPage, Requests } from './realtime.js'

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
