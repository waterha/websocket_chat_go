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
const me = { id: 'me', name: 'Me', email: 'me@test' }
const group = { id: 'history', name: 'History', kind: 'group', members: [{ ...me, role: 'owner' }, { id: 'bob', name: 'Bob', role: 'member' }], last_seq: 0 }
const requests = []
let pendingResolve
let hold = false
const result = { id: 'image-1', conversation_id: 'history', sender_id: 'bob', seq: 8, kind: 'image', content: '', created_at: '2026-10-29T01:00:00Z' }
globalThis.fetch = async (url, options = {}) => {
  const path = String(url)
  let data = []
  if (path.includes('/messages/search?')) {
    requests.push(new URL(path, location.href))
    if (hold) return new Promise(resolve => { pendingResolve = () => resolve({ ok: true, json: async () => ({ data: { messages: [result], next_before_seq: 8, has_more: false } }) }) })
    const more = path.includes('before_seq=')
    data = { messages: more ? [{ ...result, id: 'image-2', seq: 7 }] : [result], next_before_seq: more ? 7 : 8, has_more: !more }
  } else if (path.endsWith('/me')) data = me
  else if (path.endsWith('/friends/requests')) data = { incoming: [], outgoing: [] }
  else if (path === '/api/v1/conversations') data = [group]
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
vm.active = 'history'
await tick()

test('history UI filters combine without keywords and persist across pagination', async () => {
  await vm.openSearch()
  await tick()
  const panel = document.querySelector('#message-search-panel')
  assert.ok(panel.querySelector('input[aria-label="开始日期"]'))
  assert.equal(panel.querySelector('select[aria-label="按成员筛选"]').options.length, 3)
  vm.searchStart = '2026-10-01'
  vm.searchEnd = '2026-10-29'
  vm.searchSender = 'bob'
  vm.searchType = 'images'
  await vm.searchMessages()
  await tick()
  const params = requests.at(-1).searchParams
  assert.equal(params.get('q'), '')
  assert.equal(params.get('sender_id'), 'bob')
  assert.equal(params.get('type'), 'images')
  assert.ok(params.get('from'))
  assert.ok(params.get('to'))
  assert.match(panel.textContent, /表情\/图片/)
  await vm.searchMessages(true)
  assert.equal(requests.at(-1).searchParams.get('before_seq'), '8')
  assert.equal(requests.at(-1).searchParams.get('sender_id'), 'bob')
  assert.equal(vm.searchResults.length, 2)
})

test('changing history filters invalidates in-flight results and rejects reversed dates', async () => {
  hold = true
  const searching = vm.searchMessages()
  await tick()
  vm.searchType = 'files'
  vm.changeSearch()
  pendingResolve()
  await searching
  assert.equal(vm.searchResults.length, 0)
  hold = false
  vm.searchStart = '2026-10-30'
  vm.searchEnd = '2026-10-29'
  const count = requests.length
  await vm.searchMessages()
  assert.match(vm.searchError, /开始日期/)
  assert.equal(requests.length, count)
  vm.closeSearch()
  assert.equal(vm.searchSender, '')
  assert.equal(vm.searchType, 'all')
  assert.equal(vm.searchStart, '')
})

test.after(() => {
  app.unmount()
  for (const id of timers) { clearInterval(id); clearTimeout(id) }
  globalThis.setTimeout = realTimeout
  globalThis.setInterval = realInterval
  dom.window.close()
})
