import test from 'node:test'
import assert from 'node:assert/strict'
import { register } from 'node:module'
import { JSDOM } from 'jsdom'

register('../css-hook.mjs', import.meta.url)
const dom = new JSDOM('<!doctype html><html><body><div id="app"></div></body></html>', { url: 'http://127.0.0.1:4173' })
for (const key of ['window', 'document', 'location', 'HTMLElement', 'Element', 'Node', 'SVGElement', 'Text', 'Comment', 'DocumentFragment', 'Document', 'DOMParser', 'ShadowRoot', 'MutationObserver', 'File', 'Blob', 'Range']) {
  globalThis[key] = key === 'window' ? dom.window : key === 'document' ? dom.window.document : key === 'location' ? dom.window.location : dom.window[key]
}
globalThis.getComputedStyle = dom.window.getComputedStyle.bind(dom.window)
globalThis.localStorage = dom.window.localStorage
globalThis.sessionStorage = dom.window.sessionStorage
globalThis.WebSocket = class { constructor() { this.readyState = 0 } close() {} }
globalThis.URL.createObjectURL = () => 'blob:restored-preview'
globalThis.URL.revokeObjectURL = () => {}

const alice = { id: 'alice', name: 'Alice', email: 'alice@example.test' }
const bob = { id: 'bob', name: 'Bob', email: 'bob@example.test' }
const conversations = [
  { id: 'room-a', name: '房间 A', kind: 'group', members: [{ ...alice, role: 'owner' }], last_seq: 0, last_read_seq: 0 },
  { id: 'room-b', name: '房间 B', kind: 'group', members: [{ ...alice, role: 'owner' }], last_seq: 0, last_read_seq: 0 },
]
const calls = []
let activeUser = null
let imageNumber = 0
let attachmentNumber = 0

globalThis.fetch = async (url, options = {}) => {
  const path = String(url)
  calls.push({ path, method: options.method || 'GET', body: options.body })
  let data = null
  if (path.endsWith('/auth/login') && options.method === 'POST') {
    const input = JSON.parse(options.body)
    activeUser = input.email === bob.email ? bob : alice
    data = activeUser
  } else if (path.endsWith('/auth/logout')) {
    activeUser = null
    data = {}
  } else if (path.endsWith('/me')) {
    if (!activeUser) return { ok: false, status: 401, json: async () => ({ error: { message: '未登录' } }) }
    data = activeUser
  } else if (path === '/api/v1/conversations') data = conversations.map(item => ({ ...item, members: item.members.map(member => ({ ...member, id: activeUser?.id || 'alice' })) }))
  else if (path.includes('/friends/requests')) data = { incoming: [], outgoing: [] }
  else if (path.includes('/friends') || path.includes('/users') || path.includes('/devices')) data = []
  else if (path.includes('/announcements')) data = []
  else if (path.includes('/sync')) data = { messages: [], last_seq: 0, next_seq: 0, has_more: false }
  else if (path.includes('/media?')) {
    const image = options.headers?.['Content-Type'] === 'image/png'
    data = image
      ? { id: `uploaded-image-${++imageNumber}`, file_name: 'paste.png', media_type: 'image', content_type: 'image/png', byte_size: 8 }
      : { id: `uploaded-file-${++attachmentNumber}`, file_name: 'notes.pdf', media_type: 'file', content_type: 'application/pdf', byte_size: 8 }
  }
  return { ok: true, status: 200, json: async () => ({ data }) }
}

const timers = new Set()
const realSetTimeout = globalThis.setTimeout
const realSetInterval = globalThis.setInterval
globalThis.setTimeout = (...args) => { const id = realSetTimeout(...args); timers.add(id); return id }
globalThis.setInterval = (...args) => { const id = realSetInterval(...args); timers.add(id); return id }
const wait = (ms = 100) => new Promise(resolve => realSetTimeout(resolve, ms))
const settle = async () => { await wait(220); await dom.window.__vue_app__?._instance?.appContext?.config?.globalProperties?.$nextTick }

await import('./main.js')
await wait(250)
let app = document.querySelector('#app').__vue_app__
let vm = app._instance.setupState
const cleanText = value => String(value).replace(/\u00a0/g, '')
const composer = () => document.querySelector('.rich-composer')
const type = text => {
  const region = composer()
  assert.ok(region, 'composer is not mounted')
  region.textContent = text
  region.dispatchEvent(new dom.window.Event('input', { bubbles: true }))
}
const pasteImage = async () => {
  const region = composer()
  const file = new File([new Uint8Array(8)], 'paste.png', { type: 'image/png' })
  const event = new dom.window.Event('paste', { bubbles: true, cancelable: true })
  Object.defineProperty(event, 'clipboardData', { value: { items: [{ kind: 'file', type: file.type, getAsFile: () => file }], files: [file], types: ['Files'] } })
  region.dispatchEvent(event)
  assert.equal(event.defaultPrevented, true)
  await wait(450)
}
async function login(user) {
  vm.email = user.email; vm.password = 'password123'
  await vm.authenticate()
  await wait(400)
  assert.equal(vm.me.id, user.id)
}
async function choose(id) { await vm.select(id); await wait(100); assert.equal(vm.active, id) }
async function setFileAttachment() {
  const stream = document.querySelector('.stream')
  assert.ok(stream, 'conversation stream is not mounted')
  const file = new File([new Uint8Array(8)], 'notes.pdf', { type: 'application/pdf' })
  const event = new dom.window.Event('drop', { bubbles: true, cancelable: true })
  Object.defineProperty(event, 'dataTransfer', { value: { files: [file], items: [], types: ['Files'] } })
  stream.dispatchEvent(event)
  assert.equal(event.defaultPrevented, true)
  await wait(120)
}

// All cases exercise the same mounted root instance and persistent localStorage.
test('composer drafts isolate rooms, survive same-user refreshes and logout, restore uploads, and remain account-private', async () => {
  await login(alice)
  await choose('room-a')
  type('A 文本草稿\n保留换行')
  await pasteImage()
  assert.equal(vm.composerImages.length, 1)
  const savedImageID = vm.composerImages[0].mediaId
  assert.ok(document.querySelector('.composer-image img'))

  await choose('room-b')
  assert.equal(cleanText(vm.draft), '')
  assert.equal(vm.composerImages.length, 0)
  type('B 私聊/图文草稿')
  await pasteImage()
  assert.equal(vm.composerImages.length, 1)

  await choose('room-a')
  assert.equal(cleanText(vm.draft), 'A 文本草稿\n保留换行')
  assert.equal(vm.composerImages[0].mediaId, savedImageID)
  assert.equal(vm.composerImages[0].previewURL, `/api/v1/media/${savedImageID}`)
  assert.ok(document.querySelector('.composer-image img'))

  const currentUser = vm.me
  vm.me = { ...currentUser, name: 'Alice 的资料已刷新', email: 'new-alias@example.test' }
  await wait(80)
  assert.equal(cleanText(vm.draft), 'A 文本草稿\n保留换行', 'replacing the profile object with the same ID must not reset its draft')
  assert.equal(vm.composerImages[0].mediaId, savedImageID)

  await choose('room-b')
  assert.equal(cleanText(vm.draft), 'B 私聊/图文草稿')
  assert.equal(vm.composerImages.length, 1)
  document.querySelector('.composer-image [data-remove-media]').click()
  await wait(220)
  assert.equal(vm.composerImages.length, 0)
  await setFileAttachment()
  assert.equal(vm.attachment.file_name, 'notes.pdf')
  assert.ok(document.querySelector('.attachment-preview'))
  const savedFileID = vm.attachment.id

  await vm.logout()
  await wait(100)
  assert.equal(vm.me, null)
  assert.equal(cleanText(vm.draft), '', 'logout clears the live editor')
  await login(bob)
  await choose('room-b')
  assert.equal(cleanText(vm.draft), '', 'a different account must not see the prior account draft')
  assert.equal(vm.composerImages.length, 0)
  assert.equal(vm.attachment, null)
  await vm.logout(); await wait(80)
  await login(alice)
  await choose('room-b')
  assert.equal(cleanText(vm.draft), 'B 私聊/图文草稿')
  assert.equal(vm.attachment.id, savedFileID, 'an uploaded attachment should be restored from its media id')
  assert.equal(document.querySelector('.attachment-info strong').textContent, 'notes.pdf')

  await choose('room-a')
  assert.equal(cleanText(vm.draft), 'A 文本草稿\n保留换行')
  assert.equal(vm.composerImages[0].mediaId, savedImageID)

  // Reload the application against the same localStorage to exercise disk restoration.
  app.unmount(); await wait(50)
  document.querySelector('#app').replaceChildren()
  await import('./main.js?draft-reload')
  await wait(350)
  app = document.querySelector('#app').__vue_app__
  vm = app._instance.setupState
  assert.equal(vm.me.id, 'alice')
  await choose('room-a')
  assert.equal(cleanText(vm.draft), 'A 文本草稿\n保留换行')
  assert.equal(vm.composerImages[0].mediaId, savedImageID)
  await choose('room-b')
  assert.equal(cleanText(vm.draft), 'B 私聊/图文草稿')
  assert.equal(vm.attachment.id, savedFileID)
})

test.after(() => {
  app?.unmount()
  for (const id of timers) { clearTimeout(id); clearInterval(id) }
  globalThis.setTimeout = realSetTimeout
  globalThis.setInterval = realSetInterval
  dom.window.close()
})
