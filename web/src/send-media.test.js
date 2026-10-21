import test from 'node:test'
import assert from 'node:assert/strict'
import { register } from 'node:module'
import { JSDOM } from 'jsdom'

// Proves the composer sends images and the typed text as ONE message, in the order
// they appear in the draft. The chat renders them stacked in a single bubble, so a
// split into several messages would break the expected behaviour silently.
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

let uploadCount = 0
const calls = []
globalThis.fetch = async (url, options = {}) => {
  const path = String(url)
  calls.push({ path, method: options.method || 'GET', body: options.body })
  let data = null
  if (path.includes('/media')) {
    uploadCount++
    data = { id: 'media-' + uploadCount, file_name: 'pasted-' + uploadCount + '.png', media_type: 'image', content_type: 'image/png', byte_size: 8 }
  } else if (path.endsWith('/messages') && options.method === 'POST') {
    data = { id: 'msg-1', conversation_id: 'conv-1', seq: 1, kind: 'user' }
  } else if (path.includes('/me')) data = me
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

const posted = () => calls.filter(call => call.method === 'POST' && call.path.endsWith('/messages'))

test('pasting an image puts a token in the draft without sending', async () => {
  calls.length = 0
  const event = paste(new File([new Uint8Array(8)], 'image.png', { type: 'image/png' }))
  await new Promise(resolve => realSetTimeout(resolve, 500))

  assert.equal(event.defaultPrevented, true, 'the paste must be intercepted')
  assert.ok(calls.some(call => call.path.includes('/media')), 'the image was not uploaded')
  assert.equal(posted().length, 0, 'pasting must not post a message')
  // The image becomes a token inside the draft, which is what keeps its position.
  assert.match(vm.draft, /〔图1〕/, 'the draft does not reference the pasted image')
  assert.equal(vm.composerImages.length, 1, 'the image is not held by the composer')
})

test('text, image, text, image leave as one ordered message', async () => {
  calls.length = 0
  // Compose exactly what the user described: explain, paste, explain again, paste.
  vm.draft = ''
  vm.clearComposerImages()
  await new Promise(resolve => realSetTimeout(resolve, 50))

  paste(new File([new Uint8Array(8)], 'one.png', { type: 'image/png' }))
  await new Promise(resolve => realSetTimeout(resolve, 400))
  // Append rather than assign: assigning would overwrite the token the paste placed,
  // which is exactly the position this test is checking.
  vm.draft = vm.draft + '\n第一张是登录页'
  await new Promise(resolve => realSetTimeout(resolve, 50))

  // Place the second image at the caret, which sits at the end of the text.
  const textarea = document.querySelector('.composer textarea')
  textarea.selectionStart = vm.draft.length
  textarea.selectionEnd = vm.draft.length
  paste(new File([new Uint8Array(8)], 'two.png', { type: 'image/png' }))
  await new Promise(resolve => realSetTimeout(resolve, 400))
  vm.draft = vm.draft + '\n第二张是报错信息'
  await new Promise(resolve => realSetTimeout(resolve, 50))

  await vm.send()
  await new Promise(resolve => realSetTimeout(resolve, 400))

  const messages = posted()
  assert.equal(messages.length, 1, `expected one message, got ${messages.length}`)
  const payload = JSON.parse(String(messages[0].body))
  const blocks = payload.blocks || []
  const types = blocks.map(block => block.type)
  // The text the user typed before each image stays before it.
  assert.deepEqual(types, ['image', 'text', 'image', 'text'], `unexpected block order: ${JSON.stringify(types)}`)
  assert.equal(blocks[1].text, '第一张是登录页')
  assert.equal(blocks[3].text, '第二张是报错信息')
  assert.ok(blocks[0].media_id && blocks[2].media_id, 'both images must be referenced')
  assert.notEqual(blocks[0].media_id, blocks[2].media_id, 'the two images must be distinct')
  // The composer is cleared once the combined message is sent.
  assert.equal(vm.composerImages.length, 0, 'the images were not released after sending')
  assert.equal(vm.draft, '', 'the draft was not cleared after sending')
})

test('an image alone is a valid message', async () => {
  calls.length = 0
  vm.draft = ''
  vm.clearComposerImages()
  await new Promise(resolve => realSetTimeout(resolve, 50))
  paste(new File([new Uint8Array(8)], 'solo.png', { type: 'image/png' }))
  await new Promise(resolve => realSetTimeout(resolve, 400))

  await vm.send()
  await new Promise(resolve => realSetTimeout(resolve, 400))
  const messages = posted()
  assert.equal(messages.length, 1)
  const payload = JSON.parse(String(messages[0].body))
  assert.equal(payload.blocks.length, 1)
  assert.equal(payload.blocks[0].type, 'image')
})

test('removing an image drops both its strip entry and its token', async () => {
  vm.draft = ''
  vm.clearComposerImages()
  await new Promise(resolve => realSetTimeout(resolve, 50))
  paste(new File([new Uint8Array(8)], 'gone.png', { type: 'image/png' }))
  await new Promise(resolve => realSetTimeout(resolve, 400))
  assert.equal(vm.composerImages.length, 1)

  vm.removeComposerImage(vm.composerImages[0])
  await new Promise(resolve => realSetTimeout(resolve, 100))
  assert.equal(vm.composerImages.length, 0, 'the strip entry was not removed')
  assert.doesNotMatch(vm.draft, /〔图\d+〕/, 'the token was left behind in the draft')
})

test.after(() => {
  for (const handle of timers) { clearInterval(handle); clearTimeout(handle) }
  globalThis.setInterval = realSetInterval
  globalThis.setTimeout = realSetTimeout
  dom.window.close()
})
