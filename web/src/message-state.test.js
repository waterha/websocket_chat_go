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
const me = { id: 'me', name: 'Me', email: 'me@test' }, group = { id: 'state-chat', name: 'State', kind: 'group', members: [{ ...me, role: 'owner' }], last_seq: 0 }
let mode = 'ready', release, requests = 0
const response = data => ({ ok: true, status: 200, json: async () => ({ data }) })
globalThis.fetch = async url => {
  const path = String(url)
  if (path.includes('/sync?')) {
    requests++
    if (mode === 'held') return new Promise(resolve => { release = () => resolve(response({ messages: [], last_seq: 0, next_seq: 0, has_more: false })) })
    if (mode === 'failed') return { ok: false, status: 503, json: async () => ({ error: { code: 'unavailable', message: '聊天记录加载失败，请重试。' } }) }
    return response({ messages: [], last_seq: 0, next_seq: 0, has_more: false })
  }
  if (path.endsWith('/me')) return response(me)
  if (path === '/api/v1/conversations') return response([group])
  if (path.endsWith('/friends/requests')) return response({ incoming: [], outgoing: [] })
  return response([])
}
const realTimeout = globalThis.setTimeout, realInterval = globalThis.setInterval, timers = new Set()
globalThis.setTimeout = (...args) => { const timer = realTimeout(...args); timers.add(timer); return timer }
globalThis.setInterval = (...args) => { const timer = realInterval(...args); timers.add(timer); return timer }
const tick = () => new Promise(resolve => realTimeout(resolve, 100))
await import('./main.js'); await tick()
const app = document.querySelector('#app').__vue_app__, vm = app._instance.setupState

test('chat history distinguishes loading, empty and failure and retries from the shared button', async () => {
  mode = 'held'; const opening = vm.loadCurrentMessages(group.id); await tick()
  const stream = document.querySelector('[aria-label="聊天记录"]')
  assert.ok(stream.querySelector('.content-state--loading'), JSON.stringify({ loading: vm.messagesLoading, error: vm.messagesError, requests, held: !!release, markup: stream.innerHTML }))
  assert.equal(stream.querySelector('.content-state--empty'), null)
  release(); await opening; await tick()
  assert.match(stream.querySelector('.content-state--empty').textContent, /暂无内容.*说声你好/s)
  mode = 'failed'; await vm.loadCurrentMessages(); await tick()
  assert.ok(stream.querySelector('[role="alert"] .content-state__retry'))
  assert.equal(stream.querySelector('.content-state--empty'), null)
  const previousRequests = requests
  mode = 'ready'; stream.querySelector('.content-state__retry').click(); await tick()
  assert.equal(requests, previousRequests + 1)
  assert.equal(vm.messagesError, '')
  assert.ok(stream.querySelector('.content-state--empty'))
})
test.after(() => { app.unmount(); for (const timer of timers) { clearTimeout(timer); clearInterval(timer) }; globalThis.setTimeout = realTimeout; globalThis.setInterval = realInterval; dom.window.close() })
