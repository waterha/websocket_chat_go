import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'

const dom = new JSDOM('<body><div id="app"></div></body>', { url: 'http://127.0.0.1:4173' })
for (const key of ['window', 'document', 'Document', 'Element', 'HTMLElement', 'SVGElement', 'Node']) globalThis[key] = key === 'window' ? dom.window : dom.window[key]
const { createApp, nextTick } = await import('vue/dist/vue.esm-bundler.js')
const { GroupDirectory } = await import('./group-directory.js')
const response = data => ({ ok: true, json: async () => ({ data }) })
const group = { id: 'team', name: '研发群', member_count: 3, join_approval: true }
const settle = async () => { for (let i = 0; i < 4; i++) { await new Promise(resolve => setTimeout(resolve, 0)); await nextTick() } }
let app, vm, calls, joined, handler
function click(selector) { const el = document.querySelector(selector); assert.ok(el, selector); el.click() }
test.beforeEach(() => {
  calls = []; joined = []
  handler = async path => response(path.includes('/discover?') ? [group] : [])
  globalThis.fetch = async (path, options) => { calls.push({ path, options }); return handler(path, options) }
  app = createApp(GroupDirectory, { onJoined: id => joined.push(id) }); vm = app.mount('#app')
})
test.afterEach(() => app.unmount())
test.after(() => dom.window.close())

test('loads on demand, searches by keyword and displays approval state', async () => {
  assert.equal(calls.length, 0)
  click('.group-directory-trigger'); await settle()
  assert.equal(document.activeElement.type, 'search')
  assert.match(document.querySelector('.group-directory-results').textContent, /研发群.*需审批/s)
  vm.query = '研发'; await vm.load()
  const search = calls.findLast(call => call.path.includes('/discover?'))
  assert.equal(new URL(search.path, dom.window.location).searchParams.get('q'), '研发')
})

test('submits approval requests without granting access and allows withdrawal', async () => {
  await vm.open(); await settle()
  vm.choose(group); vm.message = '我是新同事'
  handler = async (path, options) => {
    if (options.method === 'POST') { assert.deepEqual(JSON.parse(options.body), { message: '我是新同事' }); return response({ pending: true }) }
    return response(path.includes('/discover?') ? [{ ...group, request_status: 'pending' }] : [])
  }
  await vm.join(); await settle()
  assert.deepEqual(joined, [])
  assert.equal(vm.opened, true)
  assert.match(document.body.textContent, /等待群主或管理员审批/)
  assert.match(document.querySelector('.group-directory-results button').textContent, /撤回/)
  handler = async path => response(path.includes('/discover?') ? [group] : [])
  await vm.withdraw('team')
  assert.ok(calls.some(call => call.options.method === 'DELETE' && call.path.endsWith('/team/join-requests')))
})

test('direct membership closes the dialog and emits the joined conversation', async () => {
  await vm.open(); await settle()
  vm.choose({ ...group, join_approval: false })
  handler = async () => response({ pending: false })
  await vm.join(); await settle()
  assert.deepEqual(joined, ['team'])
  assert.equal(document.querySelector('[role="dialog"]'), null)
})

test('failed requests keep the application text and do not emit membership', async () => {
  await vm.open(); vm.choose(group); vm.message = '保留说明'
  handler = async () => { throw new Error('offline') }
  await vm.join(); await settle()
  assert.equal(vm.message, '保留说明')
  assert.deepEqual(joined, [])
  assert.ok(document.querySelector('[role="alert"]'))
})

test('closing invalidates in-flight list results and restores focus', async () => {
  await vm.open(); await settle()
  let resolveDiscover
  handler = async path => path.includes('/discover?') ? new Promise(resolve => { resolveDiscover = resolve }) : response([])
  const loading = vm.load()
  vm.close(); resolveDiscover(response([{ ...group, name: '旧响应' }]))
  await loading; await settle()
  assert.equal(vm.opened, false)
  assert.equal(document.activeElement, document.querySelector('.group-directory-trigger'))
  assert.notEqual(vm.groups[0]?.name, '旧响应')
})
