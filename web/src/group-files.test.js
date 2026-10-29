import test from 'node:test'
import assert from 'node:assert/strict'
import { register } from 'node:module'
import { JSDOM } from 'jsdom'
register('../css-hook.mjs', import.meta.url)
const dom = new JSDOM('<div id="app"></div>', { url: 'http://127.0.0.1:4173' })
for (const key of ['window', 'document', 'location', 'HTMLElement', 'Element', 'Node', 'SVGElement', 'Text', 'Comment', 'DocumentFragment']) globalThis[key] = dom.window[key]
const { createApp, nextTick } = await import('vue/dist/vue.esm-bundler.js')
const { GroupFiles } = await import('./group-files.js')
const oldFile = { id: 'old', kind: 'file', seq: 20, sender_id: 'alice', created_at: '2025-01-01T10:00:00Z', media_id: 'old-media', media: { file_name: '旧文档.pdf', byte_size: 2048 } }
const secondFile = { ...oldFile, id: 'second', seq: 19, media_id: 'second-media', media: { file_name: '另一个文件.zip', byte_size: 4096 } }
const conversation = { id: 'group', kind: 'group', name: '测试群', members: [{ id: 'alice', name: 'Alice' }, { id: 'new', name: '新成员', role: 'member' }] }
const calls = []
let hold = false, fail = false, resolvePending
let requestedSignal
let app, vm
const tick = async () => { await new Promise(resolve => setTimeout(resolve, 10)); await nextTick() }
globalThis.fetch = async (url, options = {}) => {
  calls.push(new URL(String(url), location.href))
  requestedSignal = options.signal
  if (fail) return { ok: false, status: 500, json: async () => ({ error: { message: 'failed' } }) }
  const data = calls.at(-1).searchParams.has('before_seq')
    ? { messages: [oldFile, secondFile], next_before_seq: 19, has_more: false }
    : { messages: [oldFile, { ...secondFile, id: 'recalled', recalled_at: 'now' }], next_before_seq: 20, has_more: true }
  if (hold) return new Promise(resolve => { resolvePending = () => resolve({ ok: true, json: async () => ({ data }) }) })
  return { ok: true, json: async () => ({ data }) }
}
test.before(() => { app = createApp(GroupFiles, { conversation }); vm = app.mount('#app') })

test('group files loads on demand and shows old shared documents with download links', async () => {
  assert.equal(calls.length, 0)
  document.querySelector('.group-files-trigger').click()
  await tick()
  assert.equal(calls.length, 1)
  assert.match(calls[0].pathname, /conversations\/group\/messages\/search$/)
  assert.equal(calls[0].searchParams.get('type'), 'files')
  assert.equal(calls[0].searchParams.has('from'), false)
  assert.equal(document.querySelectorAll('.group-file-row').length, 1)
  const link = document.querySelector('.group-file-download')
  assert.equal(link.getAttribute('href'), '/api/v1/media/old-media')
  assert.equal(link.getAttribute('download'), '旧文档.pdf')
  assert.match(document.querySelector('.group-file-info').textContent, /Alice/)
  assert.match(document.querySelector('.group-file-info').textContent, /2.0 KB/)
  assert.match(document.querySelector('.group-file-info').textContent, /2025/)
  assert.equal(document.querySelector('[role="dialog"]').getAttribute('aria-modal'), 'true')
})

test('pagination keeps existing files, removes duplicates and refresh resets cursor', async () => {
  await vm.load(true)
  await tick()
  assert.equal(calls.at(-1).searchParams.get('before_seq'), '20')
  assert.equal(document.querySelectorAll('.group-file-row').length, 2)
  assert.equal(document.querySelector('.group-files-more'), null)
  await vm.load()
  await tick()
  assert.equal(calls.at(-1).searchParams.has('before_seq'), false)
  assert.equal(document.querySelectorAll('.group-file-row').length, 1)
})

test('failure is retryable and closing prevents late results and restores focus', async () => {
  fail = true
  await vm.load()
  await tick()
  assert.match(document.querySelector('.search-error').textContent, /加载失败/)
  fail = false
  await vm.load()
  assert.equal(vm.error, '')
  hold = true
  const pending = vm.load()
  await tick()
  const panel = document.querySelector('[role="dialog"]')
  panel.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
  await tick()
  assert.equal(requestedSignal.aborted, true)
  resolvePending()
  await pending
  assert.equal(document.querySelector('[role="dialog"]'), null)
  assert.equal(document.activeElement, document.querySelector('.group-files-trigger'))
  hold = false
})

test.after(() => { app.unmount(); dom.window.close() })
