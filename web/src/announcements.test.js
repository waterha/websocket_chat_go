import test from 'node:test'
import assert from 'node:assert/strict'
import { register } from 'node:module'
import { JSDOM } from 'jsdom'

register('../css-hook.mjs', import.meta.url)
const dom = new JSDOM('<div id="app"></div>', { url: 'http://127.0.0.1:4173' })
for (const key of ['window', 'document', 'location', 'HTMLElement', 'Element', 'Node', 'SVGElement', 'Text', 'Comment', 'DocumentFragment', 'Document', 'DOMParser', 'ShadowRoot', 'MutationObserver']) globalThis[key] = dom.window[key]
Object.defineProperty(globalThis, 'navigator', { configurable: true, value: dom.window.navigator })
globalThis.getComputedStyle = dom.window.getComputedStyle.bind(dom.window)
globalThis.localStorage = dom.window.localStorage
globalThis.sessionStorage = dom.window.sessionStorage
globalThis.WebSocket = class { close() {} }
globalThis.URL.createObjectURL = () => 'blob:announcement'
globalThis.URL.revokeObjectURL = () => {}
const me = { id: 'me', name: '群主', email: 'me@test' }
const group = { id: 'group', name: '研发群', kind: 'group', members: [{ ...me, role: 'owner' }], last_seq: 0 }
let announcements = Array.from({ length: 60 }, (_, index) => ({ id: `notice-${index}`, author_id: me.id, content: `公告 ${index}`, created_at: '2026-11-01T12:00:00Z' }))
const writes = []
globalThis.fetch = async (url, options = {}) => {
  const path = String(url)
  let data = []
  if (path.endsWith('/me')) data = me
  else if (path.endsWith('/friends/requests')) data = { incoming: [], outgoing: [] }
  else if (path === '/api/v1/conversations') data = [group]
  else if (path.includes('/sync')) data = { messages: [], last_seq: 0, next_seq: 0, has_more: false }
  else if (path.endsWith('/announcements')) {
    if (options.method === 'POST') {
      const payload = JSON.parse(options.body)
      writes.push(payload)
      data = { id: 'created', author_id: me.id, content: payload.content, created_at: '2026-11-01T13:00:00Z' }
      announcements = [data, ...announcements]
    } else data = announcements
  }
  return { ok: true, status: 200, json: async () => ({ data }) }
}
const realTimeout = globalThis.setTimeout
const realInterval = globalThis.setInterval
const timers = new Set()
globalThis.setTimeout = (...args) => { const handle = realTimeout(...args); timers.add(handle); return handle }
globalThis.setInterval = (...args) => { const handle = realInterval(...args); timers.add(handle); return handle }
const { nextTick } = await import('vue/dist/vue.esm-bundler.js')
let app, vm
test.after(() => {
  app?.unmount()
  for (const timer of timers) { clearTimeout(timer); clearInterval(timer) }
  globalThis.setTimeout = realTimeout
  globalThis.setInterval = realInterval
  dom.window.close()
})
const waitFor = async predicate => {
  for (let i = 0; i < 100; i++) {
    if (predicate()) return
    await new Promise(resolve => realTimeout(resolve, 10))
  }
  assert.fail('timed out waiting for announcement state')
}

test('a long announcement list keeps creation in the header, reveals the editor and publishes', async () => {
  await import('./main.js')
  app = document.querySelector('#app').__vue_app__
  vm = app._instance.setupState
  await waitFor(() => vm.me && vm.conversations.length)
  vm.active = group.id
  await waitFor(() => vm.current?.id === group.id)
  vm.openAnnouncementPanel()
  await waitFor(() => document.querySelectorAll('.announcement-item').length === 60)
  const modal = document.querySelector('.announcement-modal')
  const body = modal.querySelector('.announcement-body')
  const create = modal.querySelector('.modal-head .announcement-create')
  assert.ok(create)
  assert.equal(body.contains(create), false)
  assert.equal(modal.querySelector('[aria-label="关闭"]'), null)
  assert.equal([...modal.querySelectorAll('button')].some(button => button.textContent.trim() === '关闭'), false)
  body.scrollTop = 2000
  create.click(); await nextTick()
  const editor = body.querySelector('.announcement-editor')
  assert.ok(editor.compareDocumentPosition(body.querySelector('.announcement-list')) & Node.DOCUMENT_POSITION_FOLLOWING)
  assert.equal(body.scrollTop, 0)
  assert.equal(document.activeElement, editor.querySelector('textarea'))
  editor.querySelector('textarea').value = '新的群公告'
  editor.querySelector('textarea').dispatchEvent(new window.Event('input', { bubbles: true }))
  await nextTick()
  editor.dispatchEvent(new window.Event('submit', { bubbles: true, cancelable: true }))
  await waitFor(() => document.querySelectorAll('.announcement-item').length === 61 && !vm.announcementSaving)
  assert.deepEqual(writes, [{ content: '新的群公告' }])
  assert.ok(modal.querySelector('.modal-head .announcement-create'))
  body.scrollTop = 2000
  body.querySelector('.announcement-item-actions button').click(); await nextTick()
  assert.equal(body.scrollTop, 0)
  assert.equal(document.activeElement.value, '新的群公告')
  vm.cancelAnnouncementEditor(); await nextTick()
  document.querySelector('.announcement-backdrop').click(); await nextTick()
  assert.equal(document.querySelector('.announcement-modal'), null)
})

test('only owners and admins see announcement creation', async () => {
  for (const role of ['member', 'admin', 'owner']) {
    vm.current.members[0].role = role
    vm.openAnnouncementPanel(); await nextTick()
    assert.equal(Boolean(document.querySelector('.announcement-create')), role !== 'member')
    vm.closeAnnouncementPanel(); await nextTick()
  }
})
