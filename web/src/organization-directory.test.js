import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'

const dom = new JSDOM('<body><div id="app"></div></body>', { url: 'http://127.0.0.1:4173' })
for (const key of ['window', 'document', 'Element', 'HTMLElement', 'SVGElement', 'Node']) globalThis[key] = key === 'window' ? dom.window : dom.window[key]
const { createApp, nextTick } = await import('vue/dist/vue.esm-bundler.js')
const { OrganizationDirectory } = await import('./organization-directory.js')
const bob = { id: 'bob', name: 'Bob', email: 'bob@example.test', department: '研发部', job_title: '工程师' }
const carol = { ...bob, id: 'carol', name: 'Carol' }
const response = data => ({ ok: true, json: async () => ({ data }) })
const page = (entries, has_more = false, next_offset = entries.length) => ({ entries, departments: ['研发部'], has_more, next_offset })
const settle = async () => { for (let i = 0; i < 4; i++) { await new Promise(resolve => setTimeout(resolve, 0)); await nextTick() } }
let app, vm, requests, chat, handler
function click(selector) { const el = document.querySelector(selector); assert.ok(el, selector); el.click(); return el }
async function input(selector, value) { const el = document.querySelector(selector); el.value = value; el.dispatchEvent(new dom.window.Event('input', { bubbles: true })); await nextTick() }

test.beforeEach(() => {
  requests = []; chat = []
  handler = async (path) => path.endsWith('/me') ? response({ department: '', job_title: '' }) : response(page([bob], true, 50))
  globalThis.fetch = async (path, options) => { requests.push({ path, options }); return handler(path, options) }
  app = createApp(OrganizationDirectory, { onChat: id => chat.push(id) })
  vm = app.mount('#app')
})
test.afterEach(() => app.unmount())
test.after(() => dom.window.close())

test('loads only on opening, searches server, filters departments and starts direct chat', async () => {
  assert.equal(requests.length, 0)
  click('.org-entry'); await settle()
  assert.equal(document.activeElement.type, 'search')
  assert.match(document.querySelector('.org-person').textContent, /Bob/)
  await input('.org-search input', 'Go 工程师')
  document.querySelector('.org-search').dispatchEvent(new dom.window.Event('submit', { bubbles: true, cancelable: true }))
  await settle()
  assert.equal(new URL(requests.at(-1).path, dom.window.location).searchParams.get('q'), 'Go 工程师')
  const select = document.querySelector('.org-search select')
  select.value = '研发部'; select.dispatchEvent(new dom.window.Event('change', { bubbles: true })); await settle()
  assert.equal(new URL(requests.at(-1).path, dom.window.location).searchParams.get('department'), '研发部')
  click('.org-person button'); await settle()
  assert.deepEqual(chat, ['bob'])
  assert.equal(document.querySelector('[role="dialog"]'), null)
  assert.equal(document.activeElement, document.querySelector('.org-entry'))
})

test('appends pages without duplicates and saves only own profile', async () => {
  click('.org-entry'); await settle()
  handler = async () => response(page([bob, carol], false, 52))
  click('.org-more'); await settle()
  assert.equal(new URL(requests.at(-1).path, dom.window.location).searchParams.get('offset'), '50')
  assert.equal(document.querySelectorAll('.org-person').length, 2)
  assert.equal(document.querySelector('.org-more'), null)
  document.querySelector('details').open = true
  const fields = document.querySelectorAll('.org-profile input')
  fields[0].value = '研发部'; fields[0].dispatchEvent(new dom.window.Event('input', { bubbles: true }))
  fields[1].value = '工程师'; fields[1].dispatchEvent(new dom.window.Event('input', { bubbles: true }))
  await nextTick()
  handler = async (path, options) => { assert.equal(path, '/api/v1/directory/me'); assert.equal(options.method, 'PATCH'); assert.deepEqual(JSON.parse(options.body), { department: '研发部', job_title: '工程师' }); return response({ department: '研发部', job_title: '工程师' }) }
  document.querySelector('.org-profile form').dispatchEvent(new dom.window.Event('submit', { bubbles: true, cancelable: true }))
  await settle()
  assert.match(document.querySelector('.org-profile').textContent, /工作资料已保存/)
})

test('a failed profile load never enables saving empty data and directory retry works', async () => {
  handler = async () => { throw new Error('network') }
  click('.org-entry'); await settle()
  assert.equal(document.querySelector('.org-profile .primary').disabled, true)
  assert.ok(document.querySelector('[role="alert"]'))
  handler = async path => path.endsWith('/me') ? response({ department: '财务部', job_title: '会计' }) : response(page([]))
  await vm.load(); await vm.loadProfile(); await settle()
  assert.equal(document.querySelector('.org-profile .primary').disabled, false)
  assert.equal(vm.workDepartment, '财务部')
  assert.match(document.querySelector('.org-results').textContent, /没有找到同事/)
})

test('new searches and closing invalidate stale requests even if abort is ignored', async () => {
  click('.org-entry'); await settle()
  let finishOld
  handler = () => new Promise(resolve => { finishOld = resolve })
  const old = vm.load()
  handler = async () => response(page([carol]))
  await vm.load()
  finishOld(response(page([bob])))
  await old; await settle()
  assert.equal(vm.entries[0].id, 'carol')
  handler = () => new Promise(resolve => { finishOld = resolve })
  const closing = vm.load()
  vm.close(); finishOld(response(page([bob])))
  await closing; await settle()
  assert.equal(vm.opened, false)
  assert.equal(vm.entries.length, 0)
})
