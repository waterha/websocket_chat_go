import test from 'node:test'
import assert from 'node:assert/strict'
import { register } from 'node:module'
import { JSDOM } from 'jsdom'
register('../css-hook.mjs', import.meta.url)
const dom = new JSDOM('<!doctype html><html><body><div id="app"></div></body></html>', { url: 'http://127.0.0.1:4173' })
for (const key of ['window', 'document', 'location', 'HTMLElement', 'Element', 'Node', 'SVGElement', 'Text', 'Comment', 'DocumentFragment', 'Document', 'DOMParser', 'ShadowRoot', 'MutationObserver', 'File', 'Blob', 'Range']) globalThis[key] = key === 'window' ? dom.window : key === 'document' ? dom.window.document : key === 'location' ? dom.window.location : dom.window[key]
globalThis.getComputedStyle = dom.window.getComputedStyle.bind(dom.window)
globalThis.localStorage = dom.window.localStorage; globalThis.sessionStorage = dom.window.sessionStorage
globalThis.WebSocket = class { constructor() { this.readyState = 0 } close() {} }
globalThis.URL.revokeObjectURL = () => {}
const me = { id: 'me', name: '我', email: 'me@test' }
const rooms = ['a', 'b'].map(id => ({ id, name: id, kind: 'group', members: [{ ...me, role: 'owner' }], last_seq: 0, last_read_seq: 0 }))
const stored = [{ id: 'photo-message', conversation_id: 'a', seq: 1, sender_id: 'me', kind: 'image', media_id: 'photo', media: { file_name: '表情.gif', byte_size: 8 }, content: '', created_at: new Date().toISOString() }, { id: 'emoji-message', conversation_id: 'a', seq: 2, sender_id: 'me', kind: 'user', content: '😀', created_at: new Date().toISOString() }, { id: 'text-message', conversation_id: 'a', seq: 3, sender_id: 'me', kind: 'user', content: '普通消息', created_at: new Date().toISOString() }]
const saved = [{ id: 'saved-image', kind: 'image', file_name: '表情.gif', byte_size: 8 }, { id: 'saved-emoji', kind: 'emoji', text: '😀' }]
const calls = []; let prepareGate, releasePrepare
let prepared = 0
globalThis.fetch = async (url, options = {}) => {
  const path = String(url); calls.push({ path, options }); let data = []
  if (path.endsWith('/me')) data = me
  else if (path === '/api/v1/conversations') data = rooms
  else if (path.endsWith('/friends/requests')) data = { incoming: [], outgoing: [] }
  else if (path.includes('/sync?')) {
    const after = Number(new URL(path, dom.window.location).searchParams.get('after_seq'))
    const messages = path.includes('/a/') ? stored.filter(item => item.seq > after) : []
    data = { messages, last_seq: path.includes('/a/') ? stored.at(-1).seq : 0, next_seq: messages.at(-1)?.seq || after, has_more: false }
  } else if (path.startsWith('/api/v1/stickers/upload?')) {
    data = { id: 'uploaded-local', kind: 'image', file_name: 'local.gif' }; saved.unshift(data)
  } else if (path.startsWith('/api/v1/stickers/') && options.method === 'DELETE') {
    const id = decodeURIComponent(path.split('/').at(-1)); const index = saved.findIndex(item => item.id === id)
    if (index >= 0) saved.splice(index, 1)
    return { ok: true, status: 204 }
  } else if (path === '/api/v1/stickers') data = options.method === 'POST' ? saved[0] : { stickers: saved }
  else if (path.endsWith('/prepare')) { if (prepareGate) await prepareGate; data = { id: `prepared-${++prepared}`, media_type: 'image', file_name: '表情.gif', byte_size: 8 } }
  return { ok: true, status: 200, json: async () => ({ data }) }
}
const timers = new Set(), realTimeout = globalThis.setTimeout, realInterval = globalThis.setInterval
globalThis.setTimeout = (...args) => { const id = realTimeout(...args); timers.add(id); return id }
globalThis.setInterval = (...args) => { const id = realInterval(...args); timers.add(id); return id }
const wait = (ms = 100) => new Promise(resolve => realTimeout(resolve, ms))
await import('./main.js')
await wait(400)
const app = document.querySelector('#app').__vue_app__, vm = app._instance.setupState
function openCustom() {
  const picker = document.querySelector('.emoji-picker')
  if (!picker.classList.contains('open')) picker.querySelector('.emoji-picker-toggle').click()
  const tab = [...picker.querySelectorAll('[role="tab"]')].find(item => item.textContent === '表情')
  assert.ok(tab, '用户表情入口应存在'); tab.click()
}
test('images and emoji can be added by context menu and selected from the personal tab into a safe draft', async () => {
  await vm.select('b'); await wait()
  await vm.select('a'); await wait()
  const image = document.querySelector('[data-message-seq="1"] img')
  image.dispatchEvent(new dom.window.MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 12, clientY: 12 })); await wait()
  const add = [...document.querySelectorAll('[role="menuitem"]')].find(item => item.textContent === '添加表情')
  assert.ok(add); add.click(); await wait()
  const saveCall = calls.find(call => call.path === '/api/v1/stickers' && call.options.method === 'POST')
  assert.deepEqual(JSON.parse(saveCall.options.body), { conversation_id: 'a', message_id: 'photo-message', media_id: 'photo' })
  assert.equal(vm.messageActions.favoriteNotice, '已添加表情')
  await vm.messageActions.openMenu({ clientX: 10, clientY: 10 }, vm.messages.find(item => item.id === 'emoji-message'))
  assert.equal(vm.messageActions.canAddSticker, true)
  await vm.messageActions.addSticker(vm.messageActions.menuMessage)
  await vm.messageActions.openMenu({ clientX: 10, clientY: 10 }, vm.messages.find(item => item.id === 'text-message'))
  assert.equal(vm.messageActions.canAddSticker, false); vm.messageActions.closeMenu()
  openCustom(); await wait()
  const privateImage = document.querySelector('.emoji-picker img')
  assert.ok(privateImage); assert.match(privateImage.getAttribute('src'), /\/stickers\/saved-image\/media$/)
  privateImage.closest('button').click(); await wait()
  assert.equal(vm.composerImages.length, 1, JSON.stringify({ error: vm.error, active: vm.active, chatVisible: vm.chatVisible, busy: vm.uploadBusy, prepares: calls.filter(call => call.path.endsWith('/prepare')) }))
  assert.equal(vm.composerImages[0].mediaId, 'prepared-1')
  assert.ok(document.querySelector('.rich-composer img[src="/api/v1/media/prepared-1"]'))
  assert.equal(calls.filter(call => /\/conversations\/[^/]+\/messages$/.test(call.path) && call.options.method === 'POST').length, 0, '选表情不应自动发送')
  assert.deepEqual(JSON.parse(calls.find(call => call.path.endsWith('/prepare')).options.body), { conversation_id: 'a' })
  vm.removeComposerImage(vm.composerImages[0]); await wait()
  // Hold preparation and switch away and back: the late result must not modify either draft.
  prepareGate = new Promise(resolve => { releasePrepare = resolve })
  openCustom(); await wait(); document.querySelector('.emoji-picker img').closest('button').click(); await wait()
  await vm.select('b'); await wait(); await vm.select('a'); await wait()
  releasePrepare(); prepareGate = null; await wait()
  assert.equal(vm.composerImages.length, 0)
})
test('personal library uploads and deletes through the real main callbacks without sending messages', async () => {
  openCustom(); await wait()
  const picker = document.querySelector('.emoji-picker')
  const input = picker.querySelector('input[type="file"]')
  picker.querySelector('[aria-label="上传表情"]').click()
  Object.defineProperty(input, 'files', { configurable: true, value: [new File(['local'], 'local.gif', { type: 'image/gif' })] })
  input.dispatchEvent(new dom.window.Event('change', { bubbles: true })); await wait()
  const uploadCall = calls.find(call => call.path.startsWith('/api/v1/stickers/upload?'))
  assert.ok(uploadCall)
  assert.equal(uploadCall.options.body.name, 'local.gif')
  const option = picker.querySelector('img[src="/api/v1/stickers/uploaded-local/media"]').closest('button')
  option.dispatchEvent(new dom.window.MouseEvent('contextmenu', { bubbles: true, cancelable: true })); await wait()
  const remove = picker.querySelector('[role="menuitem"]')
  assert.equal(remove.textContent, '删除表情'); remove.click(); await wait()
  assert.ok(calls.some(call => call.path === '/api/v1/stickers/uploaded-local' && call.options.method === 'DELETE'))
  assert.equal(picker.querySelector('img[src="/api/v1/stickers/uploaded-local/media"]'), null)
  assert.equal(vm.composerImages.length, 0)
  assert.equal(calls.filter(call => /\/conversations\/[^/]+\/messages$/.test(call.path) && call.options.method === 'POST').length, 0)
})
test.after(() => { app.unmount(); for (const timer of timers) { clearTimeout(timer); clearInterval(timer) }; dom.window.close() })
