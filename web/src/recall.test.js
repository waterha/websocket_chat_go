import test from 'node:test'
import assert from 'node:assert/strict'
import { register } from 'node:module'
import { JSDOM } from 'jsdom'

register('../css-hook.mjs', import.meta.url)
const dom = new JSDOM('<!doctype html><html><body><div id="app"></div></body></html>', { url: 'http://127.0.0.1:4173' })
for (const key of ['window', 'document', 'location', 'HTMLElement', 'Element', 'Node', 'SVGElement', 'Text', 'Comment', 'DocumentFragment', 'Document', 'ShadowRoot', 'MutationObserver', 'File', 'Blob', 'Range']) {
  globalThis[key] = key === 'window' ? dom.window : key === 'document' ? dom.window.document : key === 'location' ? dom.window.location : dom.window[key]
}
globalThis.getComputedStyle = dom.window.getComputedStyle.bind(dom.window)
globalThis.localStorage = dom.window.localStorage
globalThis.sessionStorage = dom.window.sessionStorage
globalThis.confirm = () => true
const sockets = []
globalThis.WebSocket = class {
  constructor() { this.readyState = 0; sockets.push(this) }
  close() {}
}
const me = { id: 'me', name: '我', email: 'me@test' }
const group = { id: 'source', kind: 'group', name: '测试群', members: [{ ...me, role: 'owner' }], last_seq: 0, last_read_seq: 0 }
const target = { ...group, id: 'target', name: '其他会话' }
const fresh = (id, seq, extra = {}) => ({ id, seq, conversation_id: group.id, kind: 'user', sender_id: me.id, created_at: new Date().toISOString(), content: `正文 ${id}`, ...extra })
let stored = []
let recallFails = false
const calls = []
const recalled = item => ({ ...item, recalled_at: new Date().toISOString(), content: '', media_id: '', media: null, blocks: [], mentions: [], reactions: [] })
globalThis.fetch = async (url, options = {}) => {
  const path = String(url)
  calls.push({ path, method: options.method || 'GET' })
  let data = []
  if (path.endsWith('/me') || path.endsWith('/auth/login')) data = me
  else if (path.endsWith('/conversations')) data = [group, target]
  else if (path.includes('/friends/requests')) data = { incoming: [], outgoing: [] }
  else if (path.includes('/sync?')) {
    const after = Number(new URL(path, dom.window.location).searchParams.get('after_seq'))
    const messages = path.includes('/source/') ? stored.filter(item => item.seq > after) : []
    data = { messages: structuredClone(messages), last_seq: path.includes('/source/') ? stored.at(-1)?.seq || 0 : 0, next_seq: messages.at(-1)?.seq || after, has_more: false }
  } else if (path.endsWith('/recall')) {
    if (recallFails) return { ok: false, status: 403, json: async () => ({ error: { code: 'recall_window_expired', message: 'recall_window_expired' } }) }
    const seq = Number(path.split('/').at(-2))
    const index = stored.findIndex(item => item.seq === seq)
    stored[index] = recalled(stored[index])
    data = structuredClone(stored[index])
  } else if (path.includes('/thread?')) {
    const rootID = path.split('/messages/')[1].split('/')[0]
    data = { root: structuredClone(stored.find(item => item.id === rootID)), replies: structuredClone(stored.filter(item => item.thread_root_id === rootID)), reply_count: stored.filter(item => item.thread_root_id === rootID).length, has_more: false }
  }
  return { ok: true, status: 200, json: async () => ({ data }) }
}
const timers = new Set()
const realSetTimeout = globalThis.setTimeout
const realSetInterval = globalThis.setInterval
globalThis.setTimeout = (handler, delay, ...args) => { const timer = realSetTimeout(handler, delay, ...args); timers.add(timer); return timer }
globalThis.setInterval = (handler, delay, ...args) => { const timer = realSetInterval(handler, delay, ...args); timers.add(timer); return timer }
await import('./main.js')
const { nextTick } = await import('vue/dist/vue.esm-bundler.js')
const settle = async () => { await nextTick(); await new Promise(resolve => realSetTimeout(resolve, 20)); await nextTick() }
await settle()
const app = document.querySelector('#app').__vue_app__
const vm = app._instance.setupState

async function reset(items) {
  await vm.logout()
  await settle()
  stored = structuredClone(items)
  recallFails = false
  await vm.authenticate()
  await settle()
  calls.length = 0
}
async function eventUpdate(item) {
  await sockets.at(-1).onmessage({ data: JSON.stringify({ event: 'message.recalled', data: item }) })
  await settle()
}

test('successful recall immediately replaces the cached image message without a new sequence', async () => {
  const image = fresh('image', 1, { kind: 'image', media_id: 'photo', media: { file_name: 'photo.png' }, blocks: [{ type: 'text', text: '保密内容' }, { type: 'image', media_id: 'photo', file_name: 'photo.png' }] })
  await reset([image, fresh('later', 2)])
  assert.ok(document.querySelector('[data-message-seq="1"] img'))
  await vm.recallMessage(vm.messages[0])
  await settle()
  assert.ok(vm.messages[0].recalled_at)
  assert.equal(vm.messages[0].content, '')
  assert.equal(vm.messages[0].media_id, '')
  assert.equal(vm.messages.length, 2)
  const row = document.querySelector('[data-message-seq="1"]')
  assert.match(row.textContent, /你撤回了一条消息/)
  assert.ok(row.classList.contains('recall-row'))
  assert.ok(row.querySelector('.recall-notice'))
  assert.equal(row.querySelector('.avatar'), null)
  assert.equal(row.querySelector('.bubble'), null)
  assert.equal(row.querySelector('.message-meta'), null)
  assert.equal(row.querySelector('.block-image'), null)
  assert.equal(row.querySelector('.media-image'), null)
})

test('a recall event updates an old message and removes its cached quote and thread preview', async () => {
  const root = fresh('root', 1, { sender_id: 'peer' })
  const reply = fresh('reply', 2, { sender_id: 'peer', thread_root_id: root.id, reply: { id: root.id, sender_id: root.sender_id, excerpt: root.content, recalled: false } })
  root.thread = { reply_count: 1, preview: [reply] }
  const quote = fresh('quote', 3, { reply: { id: root.id, sender_id: root.sender_id, excerpt: root.content, recalled: false } })
  await reset([root, reply, quote])
  await vm.openThread(vm.messages[0])
  await eventUpdate({ ...recalled(root), recalled_by: 'moderator', recalled_by_name: '小明' })
  assert.ok(vm.messages.find(item => item.id === root.id).recalled_at)
  assert.ok(vm.threadRoot.recalled_at)
  const recalledRow = document.querySelector('[data-message-seq="1"]')
  assert.ok(recalledRow.classList.contains('recall-row'))
  assert.equal(recalledRow.querySelector('.avatar'), null)
  assert.equal(recalledRow.querySelector('.bubble'), null)
  assert.equal(recalledRow.querySelector('.recall-notice').textContent, '小明撤回了一条成员消息')
  assert.equal(vm.messages.find(item => item.id === quote.id).reply.recalled, true)
  assert.equal(vm.messages.find(item => item.id === quote.id).reply.excerpt, '')
  assert.equal(vm.threadReplies[0].reply.excerpt, '')
  await eventUpdate(recalled(reply))
  assert.ok(vm.threadReplies[0].recalled_at)
  assert.ok(vm.messages.find(item => item.id === root.id).thread.preview[0].recalled_at)
})

test('recall events update inactive conversation caches without changing the visible conversation', async () => {
  const item = fresh('background', 1, { sender_id: 'peer' })
  await reset([item])
  await vm.select('target')
  await eventUpdate(recalled(item))
  assert.equal(vm.active, 'target')
  assert.equal(vm.messages.length, 0)
  await vm.select('source')
  await settle()
  assert.ok(vm.messages[0].recalled_at)
  assert.equal(vm.messages[0].content, '')
})

test('recall failures keep the original message and show the server error', async () => {
  await reset([fresh('denied', 1)])
  recallFails = true
  await vm.recallMessage(vm.messages[0])
  await settle()
  assert.equal(vm.messages[0].recalled_at, undefined)
  assert.equal(vm.messages[0].content, '正文 denied')
  assert.match(vm.error, /recall_window_expired/)
})

test('cancelling recall sends no request and preserves the message', async () => {
  await reset([fresh('cancelled', 1)])
  globalThis.confirm = () => false
  try {
    await vm.recallMessage(vm.messages[0])
    assert.equal(calls.some(call => call.path.endsWith('/recall')), false)
    assert.equal(vm.messages[0].recalled_at, undefined)
  } finally { globalThis.confirm = () => true }
})

test.after(() => {
  app.unmount()
  for (const timer of timers) { clearTimeout(timer); clearInterval(timer) }
  globalThis.setTimeout = realSetTimeout
  globalThis.setInterval = realSetInterval
  dom.window.close()
})
