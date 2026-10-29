import test from 'node:test'
import assert from 'node:assert/strict'
import { register } from 'node:module'
import { JSDOM } from 'jsdom'
register('../css-hook.mjs', import.meta.url)
const dom = new JSDOM('<div id="app"></div>', { url: 'http://127.0.0.1:4173' })
for (const key of ['window', 'document', 'location', 'HTMLElement', 'Element', 'Node', 'SVGElement', 'Text', 'Comment', 'DocumentFragment', 'Document', 'DOMParser', 'ShadowRoot', 'MutationObserver', 'File', 'Blob', 'Range']) globalThis[key] = dom.window[key]
globalThis.getComputedStyle = dom.window.getComputedStyle.bind(dom.window)
globalThis.localStorage = dom.window.localStorage
globalThis.sessionStorage = dom.window.sessionStorage
globalThis.WebSocket = class { close() {} }
globalThis.URL.createObjectURL = () => 'blob:preview'
globalThis.URL.revokeObjectURL = () => {}
const me = { id: 'me', name: 'Me', email: 'me@test' }
const groups = ['one', 'two'].map(id => ({ id, name: id, kind: 'group', members: [{ ...me, role: 'owner' }], last_seq: 0 }))
const uploads = []
const sent = []
let failUpload = false
let failSend = false
let holdUpload = false
let finishUpload
let lastSignal
const uploadResult = (id, name) => ({ id, file_name: name, media_type: 'file', content_type: 'application/octet-stream', byte_size: 10 })
globalThis.fetch = async (url, options = {}) => {
  const path = String(url)
  let data = []
  if (path.includes('/media?')) {
    uploads.push({ path, options })
    lastSignal = options.signal
    if (failUpload) throw new Error('上传失败')
    if (holdUpload) return new Promise(resolve => { finishUpload = () => resolve({ ok: true, json: async () => ({ data: uploadResult('late', 'late.bin') }) }) })
    data = uploadResult('upload-' + uploads.length, options.body.name)
  } else if (path.endsWith('/messages') && options.method === 'POST') {
    if (failSend) return { ok: false, status: 500, json: async () => ({ error: { message: '发送失败' } }) }
    sent.push(JSON.parse(options.body))
    data = { id: 'sent', conversation_id: 'one', seq: 1 }
  } else if (path.endsWith('/me')) data = me
  else if (path.endsWith('/friends/requests')) data = { incoming: [], outgoing: [] }
  else if (path === '/api/v1/conversations') data = groups
  else if (path.includes('/sync')) data = { messages: [], last_seq: 0, next_seq: 0, has_more: false }
  return { ok: true, status: 200, json: async () => ({ data }) }
}
const realTimeout = globalThis.setTimeout
const realInterval = globalThis.setInterval
const timers = new Set()
globalThis.setTimeout = (...args) => { const id = realTimeout(...args); timers.add(id); return id }
globalThis.setInterval = (...args) => { const id = realInterval(...args); timers.add(id); return id }
const tick = () => new Promise(resolve => realTimeout(resolve, 100))
await import('./main.js')
await tick()
const app = document.querySelector('#app').__vue_app__
const vm = app._instance.setupState
vm.active = 'one'
await tick()
const choose = file => vm.chooseMedia({ target: { files: [file], value: 'chosen' } })
const file = () => new File(['archive'], 'archive.custom', { type: '' })
function type(text) {
  const input = document.querySelector('.rich-composer')
  input.textContent = text
  input.dispatchEvent(new dom.window.Event('input', { bubbles: true }))
}

test('file picker uploads unknown file types and sends file with caption without blocks', async () => {
  assert.ok(document.querySelector('button[aria-label="上传文件"]'))
  await choose(file())
  await tick()
  assert.equal(sent.length, 0)
  assert.equal(uploads.at(-1).options.headers['Content-Type'], 'application/octet-stream')
  assert.match(document.querySelector('.attachment-preview').textContent, /archive.custom/)
  type('这是附件说明')
  await vm.send()
  assert.equal(sent.at(-1).media_id, 'upload-1')
  assert.equal(sent.at(-1).content, '这是附件说明')
  assert.deepEqual(sent.at(-1).blocks, [])
  assert.equal(vm.attachment, null)
})

test('failed replacement preserves previous attachment and failed send keeps file for retry', async () => {
  await choose(file())
  const previous = vm.attachment.id
  failUpload = true
  await choose(new File(['bad'], 'failed.zip', { type: 'application/zip' }))
  assert.equal(vm.attachment.id, previous)
  failUpload = false
  failSend = true
  type('重试说明')
  await vm.send()
  assert.equal(vm.attachment.id, previous)
  failSend = false
  await vm.send()
  assert.equal(sent.at(-1).media_id, previous)
  assert.equal(vm.attachment, null)
})

test('cancel upload and conversation switch discard late file responses', async () => {
  holdUpload = true
  const uploading = choose(file())
  await tick()
  assert.equal(vm.uploadBusy, true)
  assert.match(document.querySelector('.attachment-status').textContent, /archive.custom/)
  vm.cancelFileUpload()
  assert.equal(lastSignal.aborted, true)
  finishUpload()
  await uploading
  assert.equal(vm.attachment, null)
  assert.equal(vm.uploadBusy, false)
  const switching = choose(file())
  await tick()
  vm.active = 'two'
  assert.equal(lastSignal.aborted, true)
  finishUpload()
  await switching
  assert.equal(vm.attachment, null)
  holdUpload = false
  vm.active = 'one'
  await tick()
})

test('a dropped or pasted document is uploaded but not automatically sent', async () => {
  const count = sent.length
  const transfer = { files: [file()], types: ['Files'], items: [] }
  let prevented = false
  vm.onDrop({ dataTransfer: transfer, preventDefault() { prevented = true } })
  await tick()
  assert.equal(prevented, true)
  assert.ok(vm.attachment)
  vm.clearAttachment()
  const input = document.querySelector('.rich-composer')
  const event = new dom.window.Event('paste', { bubbles: true, cancelable: true })
  Object.defineProperty(event, 'clipboardData', { value: { ...transfer, getData: () => '' } })
  input.dispatchEvent(event)
  await tick()
  assert.equal(event.defaultPrevented, true)
  assert.ok(vm.attachment)
  assert.equal(sent.length, count)
  vm.clearAttachment()
})

test.after(() => {
  app.unmount()
  for (const id of timers) { clearInterval(id); clearTimeout(id) }
  globalThis.setTimeout = realTimeout
  globalThis.setInterval = realInterval
  dom.window.close()
})
