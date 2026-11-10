import test from 'node:test'
import assert from 'node:assert/strict'
import { register } from 'node:module'
import { JSDOM } from 'jsdom'
register('../css-hook.mjs', import.meta.url)
const dom = new JSDOM('<div id="app"></div>', { url: 'http://127.0.0.1:4173' })
for (const key of ['window', 'document', 'location', 'HTMLElement', 'Element', 'Node', 'SVGElement', 'Text', 'Comment', 'DocumentFragment', 'Document', 'DOMParser', 'ShadowRoot', 'MutationObserver', 'File', 'Blob', 'Range']) globalThis[key] = dom.window[key]
globalThis.getComputedStyle = dom.window.getComputedStyle.bind(dom.window)
globalThis.localStorage = dom.window.localStorage; globalThis.sessionStorage = dom.window.sessionStorage
globalThis.WebSocket = class { close() {} }
const me = { id: 'alice', name: 'Alice', email: 'alice@test' }
let release, signal
const calls = []
globalThis.fetch = async (url, options = {}) => {
  calls.push(String(url))
  if (String(url).startsWith('/api/v1/messages/search?')) {
    signal = options.signal
    return new Promise(resolve => { release = () => resolve({ ok: true, json: async () => ({ data: { results: [{ message: { id: 'old-private-result' } }], has_more: false } }) }) })
  }
  let data = []
  if (String(url).endsWith('/me')) data = me
  if (String(url).endsWith('/friends/requests')) data = { incoming: [], outgoing: [] }
  return { ok: true, json: async () => ({ data }) }
}
const realTimeout = globalThis.setTimeout, realInterval = globalThis.setInterval, timers = new Set()
globalThis.setTimeout = (...args) => { const timer = realTimeout(...args); timers.add(timer); return timer }
globalThis.setInterval = (...args) => { const timer = realInterval(...args); timers.add(timer); return timer }
const tick = () => new Promise(resolve => realTimeout(resolve, 100))
await import('./main.js'); await tick()
const app = document.querySelector('#app').__vue_app__, vm = app._instance.setupState
test('account changes abort and clear global search before an old response can arrive', async () => {
  await vm.openGlobalSearch(); vm.globalQuery = 'private'; const pending = vm.searchAll()
  assert.equal(vm.globalBusy, true)
  vm.me = null
  assert.equal(signal.aborted, true)
  assert.equal(vm.showGlobalSearch, false)
  assert.equal(vm.globalBusy, false)
  vm.me = { id: 'bob', name: 'Bob' }; release(); await pending; await tick()
  assert.deepEqual(vm.globalResults, [])
  assert.equal(vm.globalQuery, '')
  assert.equal(calls.filter(path => path.startsWith('/api/v1/messages/search?')).length, 1)
})
test.after(() => { app.unmount(); for (const timer of timers) { clearTimeout(timer); clearInterval(timer) }; globalThis.setTimeout = realTimeout; globalThis.setInterval = realInterval; dom.window.close() })
