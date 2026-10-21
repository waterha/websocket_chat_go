import test from 'node:test'
import assert from 'node:assert/strict'
import { register } from 'node:module'
import { JSDOM } from 'jsdom'

// Proves the composer sends an attachment and the typed text as ONE message. The
// chat shows them stacked in a single bubble, so a split into two messages would
// break the expected "image plus caption" behaviour without failing any unit test.
register('../css-hook.mjs', import.meta.url)

const dom = new JSDOM('<!doctype html><html><body><div id="app"></div></body></html>', { url: 'http://127.0.0.1:4173' })
for (const key of ['window', 'document', 'location', 'HTMLElement', 'Element', 'Node', 'SVGElement', 'Text', 'Comment', 'DocumentFragment', 'Document', 'ShadowRoot', 'MutationObserver', 'File', 'Blob']) {
  globalThis[key] = key === 'window' ? dom.window : key === 'document' ? dom.window.document : key === 'location' ? dom.window.location : dom.window[key]
}
globalThis.getComputedStyle = dom.window.getComputedStyle.bind(dom.window)
globalThis.localStorage = dom.window.localStorage
globalThis.sessionStorage = dom.window.sessionStorage
globalThis.WebSocket = class { constructor() { this.readyState = 0 } close() {} }
globalThis.URL.createObjectURL = () => 'blob:preview'
globalThis.URL.revokeObjectURL = () => {}

const me = { id: 'me', email: 'me@test', name: 'Me' }
const group = {
  id: 'conv-1', name: 'Team', kind: 'group', pinned: false, muted: false,
  announcement_count: 0, last_seq: 0, last_read_seq: 0,
  members: [{ id: 'me', name: 'Me', email: 'me@test', role: 'owner', online: true }],
}

const calls = []
globalThis.fetch = async (url, options = {}) => {
  const path = String(url)
  calls.push({ path, method: options.method || 'GET', body: options.body })
  let data = null
  if (path.includes('/media')) data = { id: 'media-9', file_name: 'pasted-9.png', media_type: 'image', content_type: 'image/png', byte_size: 8 }
  else if (path.endsWith('/messages') && options.method === 'POST') data = { id: 'msg-9', conversation_id: 'conv-1', seq: 1, kind: 'image', media_id: 'media-9' }
  else if (path.includes('/me')) data = me
  else if (path.includes('/friends/requests')) data = { incoming: [], outgoing: [] }
  else if (path.includes('/conversations') && !path.includes('/messages')) data = [group]
  else if (path.includes('/announcements')) data = []
  else if (path.includes('/sync')) data = { messages: [], last_seq: 0, next_seq: 0, has_more: false }
  else if (path.includes('/friends') || path.includes('/users') || path.includes('/devices')) data = []
  return { ok: true, status: 200, json: async () => ({ data }) }
}

const timers = new Set()
const realSetInterval = globalThis.setInterval
const realSetTimeout = globalThis.setTimeout
globalThis.setInterval = (handler, delay, ...args) => { const handle = realSetInterval(handler, delay, ...args); timers.add(handle); return handle }
globalThis.setTimeout = (handler, delay, ...args) => { const handle = realSetTimeout(handler, delay, ...args); timers.add(handle); return handle }

await import('./main.js')
await new Promise(resolve => realSetTimeout(resolve, 800))

const vm = document.querySelector('#app').__vue_app__._instance.setupState
vm.active = 'conv-1'
await new Promise(resolve => realSetTimeout(resolve, 400))

function paste(file) {
  const textarea = document.querySelector('.composer textarea')
  assert.ok(textarea, 'the composer textarea was not rendered')
  const event = new dom.window.Event('paste', { bubbles: true, cancelable: true })
  Object.defineProperty(event, 'clipboardData', {
    value: { items: [{ kind: 'file', type: file.type, getAsFile: () => file }], files: [file], types: ['Files'] },
    configurable: true,
  })
  textarea.dispatchEvent(event)
  return event
}

test('pasting an image attaches it without sending anything', async () => {
  calls.length = 0
  const event = paste(new File([new Uint8Array(8)], 'image.png', { type: 'image/png' }))
  await new Promise(resolve => realSetTimeout(resolve, 500))

  assert.equal(event.defaultPrevented, true, 'the paste must be intercepted')
  assert.ok(calls.some(call => call.path.includes('/media')), 'the image was not uploaded')
  assert.equal(calls.filter(call => call.method === 'POST' && call.path.endsWith('/messages')).length, 0, 'pasting must not post a message')
  assert.equal(vm.attachment?.id, 'media-9', 'the image was not held as a pending attachment')
})

test('the attachment and the typed text leave as a single message', async () => {
  calls.length = 0
  vm.draft = '会议纪要见附件'
  await new Promise(resolve => realSetTimeout(resolve, 50))
  await vm.send()
  await new Promise(resolve => realSetTimeout(resolve, 300))

  const posted = calls.filter(call => call.method === 'POST' && call.path.endsWith('/messages'))
  assert.equal(posted.length, 1, `expected one message, got ${posted.length}`)
  const payload = JSON.parse(String(posted[0].body))
  assert.equal(payload.content, '会议纪要见附件', 'the text did not travel with the image')
  assert.equal(payload.media_id, 'media-9', 'the image did not travel with the text')
  // The composer is cleared once the combined message is sent.
  assert.equal(vm.attachment, null, 'the attachment was not released after sending')
  assert.equal(vm.draft, '', 'the draft was not cleared after sending')
})

test('a second image replaces the pending one instead of posting it', async () => {
  calls.length = 0
  paste(new File([new Uint8Array(8)], 'one.png', { type: 'image/png' }))
  await new Promise(resolve => realSetTimeout(resolve, 400))
  paste(new File([new Uint8Array(8)], 'two.png', { type: 'image/png' }))
  await new Promise(resolve => realSetTimeout(resolve, 400))
  assert.equal(calls.filter(call => call.method === 'POST' && call.path.endsWith('/messages')).length, 0, 'neither paste may post a message')
  assert.ok(vm.attachment, 'an image should still be pending')
})

test.after(() => {
  for (const handle of timers) { clearInterval(handle); clearTimeout(handle) }
  globalThis.setInterval = realSetInterval
  globalThis.setTimeout = realSetTimeout
  dom.window.close()
})
