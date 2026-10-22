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
globalThis.WebSocket = class { constructor() { this.readyState = 0 } close() {} }
globalThis.URL.createObjectURL = () => 'blob:preview'
globalThis.URL.revokeObjectURL = () => {}
globalThis.confirm = () => true
const copied = []
Object.defineProperty(dom.window.navigator, 'clipboard', { value: { writeText: async text => copied.push(text) }, configurable: true })

const me = { id: 'me', name: '我', email: 'me@test' }
const group = { id: 'source', kind: 'group', name: '来源群', members: [{ ...me, role: 'owner' }], last_seq: 0, last_read_seq: 0 }
const target = { ...group, id: 'target', name: '目标群' }
const calls = []
let failMessage = ''
let threadRoot
globalThis.fetch = async (url, options = {}) => {
  const path = String(url)
  calls.push({ path, method: options.method || 'GET', body: options.body })
  if (path.includes('/api/v1/media/photo')) return { ok: true, blob: async () => new Blob(['image'], { type: 'image/png' }) }
  let data = []
  if (path.endsWith('/me')) data = me
  else if (path.endsWith('/conversations')) data = [group, target]
  else if (path.includes('/friends/requests')) data = { incoming: [], outgoing: [] }
  else if (path.includes('/sync')) data = { messages: [], last_seq: 0, next_seq: 0, has_more: false }
  else if (path.includes('/thread?')) data = { root: threadRoot, replies: [], reply_count: 0, has_more: false }
  else if (path.endsWith('/messages') && options.method === 'POST') {
    const input = JSON.parse(options.body)
    if (failMessage && input.content === failMessage) throw new Error('网络暂时不可用')
    data = { id: 'forwarded', seq: 1, conversation_id: 'target', ...input }
  }
  return { ok: true, status: 200, json: async () => ({ data }) }
}

const timers = new Set()
const realSetInterval = globalThis.setInterval
const realSetTimeout = globalThis.setTimeout
globalThis.setInterval = (handler, delay, ...args) => { const handle = realSetInterval(handler, delay, ...args); timers.add(handle); return handle }
globalThis.setTimeout = (handler, delay, ...args) => { const handle = realSetTimeout(handler, delay, ...args); timers.add(handle); return handle }

await import('./main.js')
const { nextTick } = await import('vue/dist/vue.esm-bundler.js')
const settle = async () => { await nextTick(); await new Promise(resolve => realSetTimeout(resolve, 20)); await nextTick() }
await settle()
const app = document.querySelector('#app').__vue_app__
const vm = app._instance.setupState
const menu = () => document.querySelector('.message-context-menu')
const fresh = (id = 'one', extra = {}) => ({ id, seq: id === 'two' ? 2 : 1, conversation_id: 'source', kind: 'user', sender_id: 'me', created_at: new Date().toISOString(), content: id, ...extra })

async function reset(items = [fresh()]) {
  vm.messageActions.closeMenu()
  vm.messageActions.closeForward()
  vm.messageActions.cancelSelection()
  vm.closeThread()
  vm.active = 'source'
  await settle()
  vm.messages = items
  calls.length = 0
  copied.length = 0
  failMessage = ''
  threadRoot = items[0]
  await settle()
}

async function rightClick(seq = 1) {
  const row = document.querySelector(`[data-message-seq="${seq}"]`)
  const event = new dom.window.MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 10000, clientY: 10000 })
  row.dispatchEvent(event)
  await settle()
  assert.equal(event.defaultPrevented, true)
}

async function choose(label) {
  const button = [...menu().querySelectorAll('button')].find(item => item.textContent === label)
  assert.ok(button, `missing menu item: ${label}`)
  button.click()
  await settle()
}

test('messages have no inline replies and right click opens an in-viewport menu', async () => {
  await reset([fresh('one', { thread: { reply_count: 1, preview: [{ id: 'reply', content: 'reply text', sender_id: 'me' }] } })])
  assert.equal(document.querySelector('.message-actions'), null)
  assert.equal(document.querySelector('.thread-summary'), null)
  assert.equal(menu(), null)
  await rightClick()
  assert.deepEqual([...menu().querySelectorAll('button')].map(button => button.textContent), ['复制', '回复', '转发', '多选', '撤回'])
  assert.ok(parseFloat(menu().style.left) <= dom.window.innerWidth - 8)
  assert.ok(parseFloat(menu().style.top) <= dom.window.innerHeight - 8)
  assert.equal(document.activeElement.textContent, '复制')
  document.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
  assert.equal(document.activeElement.textContent, '回复')
})

test('copy closes the menu and preserves message text', async () => {
  await reset([fresh('one', { content: '你好 😀' })])
  await rightClick()
  await choose('复制')
  assert.deepEqual(copied, ['你好 😀'])
  assert.equal(menu(), null)
})

test('copying a message with images writes embedded image data to the clipboard', async () => {
  await reset([fresh('one', { content: '前后', blocks: [
    { type: 'text', text: '前' },
    { type: 'image', media_id: 'photo', file_name: 'image.png' },
    { type: 'text', text: '后' },
  ] })])
  const writes = []
  const originalClipboard = dom.window.navigator.clipboard
  const originalClipboardItem = globalThis.ClipboardItem
  const originalWindowClipboardItem = dom.window.ClipboardItem
  class TestClipboardItem {
    constructor(parts) { this.parts = parts }
  }
  Object.defineProperty(dom.window.navigator, 'clipboard', { value: { write: async items => writes.push(items) }, configurable: true })
  globalThis.ClipboardItem = TestClipboardItem
  dom.window.ClipboardItem = TestClipboardItem
  try {
    await rightClick()
    await choose('复制')
    await new Promise(resolve => realSetTimeout(resolve, 50))
    assert.equal(writes.length, 1)
    const parts = writes[0][0].parts
    assert.ok(parts['text/html'])
    assert.match(await (await parts['text/html']).text(), /data:image\/png;base64,aW1hZ2U=/)
    const html = await (await parts['text/html']).text()
    assert.match(html, /<div class="block-stack"><p class="block-text">前<\/p><a class="block-image"><img[^>]+data:image\/png;base64,aW1hZ2U=[^>]*><\/a><p class="block-text">后<\/p><\/div>/)
    assert.equal(await (await parts['text/plain']).text(), '前\n[图片] image.png\n后')
  } finally {
    Object.defineProperty(dom.window.navigator, 'clipboard', { value: originalClipboard, configurable: true })
    globalThis.ClipboardItem = originalClipboardItem
    dom.window.ClipboardItem = originalWindowClipboardItem
  }
})

test('reply opens its conversation window only after choosing the menu action', async () => {
  await reset()
  assert.equal(document.querySelector('.thread-window'), null)
  await rightClick()
  await choose('回复')
  assert.ok(document.querySelector('.thread-window'))
  assert.ok(calls.some(call => call.path.includes('/messages/one/thread?')))
  assert.equal(menu(), null)
  document.querySelector('.thread-window-root').dispatchEvent(new dom.window.MouseEvent('contextmenu', { bubbles: true, cancelable: true }))
  await settle()
  assert.ok(menu())
})

test('outside pointer, Escape and scrolling close a message menu', async () => {
  await reset()
  await rightClick()
  document.body.dispatchEvent(new dom.window.MouseEvent('pointerdown', { bubbles: true }))
  await settle()
  assert.equal(menu(), null)
  await rightClick()
  document.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
  await settle()
  assert.equal(menu(), null)
  await rightClick()
  document.querySelector('.stream').dispatchEvent(new dom.window.Event('scroll'))
  await settle()
  assert.equal(menu(), null)
})

test('recall is offered only for own messages inside two minutes', async () => {
  for (const item of [fresh('one', { created_at: new Date(Date.now() - 120001).toISOString() }), fresh('one', { sender_id: 'peer' }), fresh('one', { created_at: 'invalid' })]) {
    await reset([item])
    await rightClick()
    assert.ok(!menu().textContent.includes('撤回'))
  }
  await reset()
  await rightClick()
  await choose('撤回')
  assert.ok(calls.some(call => call.path.endsWith('/messages/1/recall') && call.method === 'POST'))
})

test('recall expires while the menu is open', async () => {
  await reset([fresh('one', { created_at: new Date(Date.now() - 119500).toISOString() })])
  await rightClick()
  assert.ok(menu().textContent.includes('撤回'))
  await new Promise(resolve => realSetTimeout(resolve, 550))
  await settle()
  assert.ok(!menu().textContent.includes('撤回'))
})

test('system and recalled messages have no action menu', async () => {
  for (const item of [fresh('one', { kind: 'system' }), fresh('one', { recalled_at: new Date().toISOString() })]) {
    await reset([item])
    await rightClick()
    assert.equal(menu(), null)
  }
})

test('multi-select copies in chronological order and cancels cleanly', async () => {
  await reset([fresh('one'), fresh('two')])
  await rightClick(2)
  await choose('多选')
  assert.deepEqual([...vm.messageActions.selectedIDs], ['two'])
  document.querySelector('[data-message-seq="1"] .bubble').click()
  await settle()
  assert.equal(document.querySelectorAll('.message-selected').length, 2)
  const toolbar = document.querySelector('.message-selection-bar')
  assert.match(toolbar.textContent, /已选 2 条/)
  await vm.messageActions.copyMessages(vm.messageActions.selected)
  assert.deepEqual(copied, ['one\ntwo'])
  toolbar.querySelector('.text-button').click()
  await settle()
  assert.equal(document.querySelector('.message-selection-bar'), null)
  assert.equal(document.querySelectorAll('.message-selected').length, 0)
})

test('selection intercepts image clicks rather than opening the image viewer', async () => {
  await reset([fresh('one', { kind: 'image', media_id: 'photo', media: { file_name: 'photo.png' } })])
  await rightClick()
  await choose('多选')
  document.querySelector('.media-image').click()
  await settle()
  assert.equal(document.querySelector('.image-viewer'), null)
  assert.equal(vm.messageActions.selected.length, 0)
})

test('forward chooses a target and sends a new message without reply or mentions', async () => {
  await reset([fresh('one', { content: '转发文本', reply_to_id: 'original', mentions: ['peer'] })])
  await rightClick()
  await choose('转发')
  const select = document.querySelector('[aria-label="转发到会话"]')
  assert.ok(select)
  select.value = 'target'
  select.dispatchEvent(new dom.window.Event('change', { bubbles: true }))
  await settle()
  document.querySelector('.forward-modal').dispatchEvent(new dom.window.Event('submit', { bubbles: true, cancelable: true }))
  await settle()
  const posted = calls.filter(call => call.path === '/api/v1/conversations/target/messages')
  assert.equal(posted.length, 1)
  const payload = JSON.parse(posted[0].body)
  assert.equal(payload.content, '转发文本')
  assert.equal(payload.reply_to_id, '')
  assert.deepEqual(payload.mentions, [])
  assert.ok(payload.client_msg_id)
  assert.equal(document.querySelector('.forward-modal'), null)
})

test('retrying a partial batch keeps its idempotency key and does not resend successes', async () => {
  await reset([fresh('one'), fresh('two')])
  vm.messageActions.openForward(vm.messages)
  vm.messageActions.forwardTarget = 'target'
  failMessage = 'two'
  await vm.messageActions.confirmForward()
  assert.deepEqual(vm.messageActions.forwardQueue.map(item => item.id), ['two'])
  assert.equal(vm.messageActions.forwardLocked, true)
  assert.match(vm.messageActions.forwardError, /网络暂时不可用/)
  failMessage = ''
  await vm.messageActions.confirmForward()
  const posted = calls.filter(call => call.path.endsWith('/target/messages')).map(call => JSON.parse(call.body))
  assert.deepEqual(posted.map(item => item.content), ['one', 'two', 'two'])
  assert.equal(posted[1].client_msg_id, posted[2].client_msg_id)
  assert.equal(vm.messageActions.forwardQueue.length, 0)
})

test('switching conversations clears menus, selection and forwarding state', async () => {
  await reset()
  await rightClick()
  await choose('多选')
  vm.messageActions.openForward(vm.messages)
  await rightClick()
  vm.active = 'target'
  await settle()
  assert.equal(menu(), null)
  assert.equal(vm.messageActions.selectionMode, false)
  assert.equal(document.querySelector('.forward-modal'), null)
})

test.after(() => {
  app.unmount()
  for (const handle of timers) { clearInterval(handle); clearTimeout(handle) }
  globalThis.setInterval = realSetInterval
  globalThis.setTimeout = realSetTimeout
  dom.window.close()
})
