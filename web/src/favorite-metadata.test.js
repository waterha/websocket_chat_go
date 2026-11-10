import test from 'node:test'
import assert from 'node:assert/strict'
import { register } from 'node:module'
import { JSDOM } from 'jsdom'
register('../css-hook.mjs', import.meta.url)
const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'http://localhost' })
for (const key of ['window', 'document', 'HTMLElement', 'Element', 'Node', 'SVGElement', 'Text', 'Comment', 'Document', 'DocumentFragment']) globalThis[key] = key === 'window' ? dom.window : key === 'document' ? dom.window.document : dom.window[key]
const { createApp, ref, nextTick } = await import('vue/dist/vue.esm-bundler.js')
const { FavoritesPanel } = await import('./favorites.js')
const calls = []
let handler, app
const original = { id: 'saved/id', sender_name: '作者', conversation_name: '来源', tags: [], note: '', message: { kind: 'image', media_id: 'image/id', media: { file_name: '快照.png' }, content: '原始快照' } }
globalThis.fetch = async (url, options = {}) => { calls.push({ url: String(url), ...options }); const data = await handler(String(url), options); return { ok: true, json: async () => ({ data }) } }
const settle = async () => { await nextTick(); await new Promise(resolve => setTimeout(resolve, 5)); await nextTick() }
const $ = selector => document.querySelector(selector)
const input = (selector, value) => { const node = $(selector); node.value = value; node.dispatchEvent(new dom.window.Event('input', { bubbles: true })) }
const key = (node, value, shiftKey = false) => node.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: value, shiftKey, bubbles: true, cancelable: true }))
async function mount() {
  const Root = { components: { FavoritesPanel }, setup() { return { user: ref('one'), open: ref(true) } }, template: '<FavoritesPanel v-model:open="open" :userID="user" />' }
  const host = document.createElement('div'); document.body.append(host); app = createApp(Root); const vm = app.mount(host); await settle(); return vm
}
async function edit(tags, note) { $('.favorite-edit').click(); await settle(); input('.favorite-editor input', tags); input('.favorite-editor textarea', note); await settle() }
async function save() { $('.favorite-editor').dispatchEvent(new dom.window.Event('submit', { bubbles: true, cancelable: true })); await settle() }
test.afterEach(async () => { app?.unmount(); app = null; await settle(); document.body.replaceChildren(); calls.length = 0 })
test.after(() => dom.window.close())

test('metadata PATCH normalizes tags, safely renders note, preserves media snapshot and searches loaded metadata', async () => {
  const note = '个人线索\n<script>unsafe</script>'
  handler = (url, options) => options.method === 'PATCH' ? { ...original, tags: ['工作', '😀'], note } : { favorites: [original], has_more: false }
  await mount(); const imageURL = $('img').getAttribute('src')
  assert.match($('[aria-label="搜索收藏"]').placeholder, /已加载.*标签.*个人备注/)
  await edit(' 工作 ， 😀,工作, , 😀 ', note)
  assert.equal($('[aria-label="移除收藏 作者"]').disabled, true)
  await save()
  const patch = calls.find(call => call.method === 'PATCH')
  assert.match(patch.url, /favorites\/saved%2Fid$/)
  assert.deepEqual(JSON.parse(patch.body), { tags: ['工作', '😀'], note })
  assert.deepEqual([...document.querySelectorAll('.favorite-tag')].map(node => node.textContent), ['工作', '😀'])
  assert.equal($('.favorite-note p').textContent, note); assert.equal($('script'), null)
  assert.equal($('img').getAttribute('src'), imageURL); assert.equal($('.media-caption').textContent, '原始快照')
  assert.equal(document.activeElement, $('.favorite-edit'))
  const requestCount = calls.length
  for (const value of ['工作', '个人线索', '找不到']) { input('[aria-label="搜索收藏"]', value); await settle(); assert.equal(document.querySelectorAll('.favorite-entry').length, value === '找不到' ? 0 : 1) }
  assert.equal(calls.length, requestCount)
})

test('clearing matching metadata restores focus and keeps the saved snapshot', async () => {
  const saved = { ...original, tags: ['临时标签'], note: '旧备注' }
  handler = (url, options) => options.method === 'PATCH' ? { ...saved, tags: [], note: '' } : { favorites: [saved], has_more: false }
  await mount(); input('[aria-label="搜索收藏"]', '临时标签'); await settle(); await edit('', ''); await save()
  assert.deepEqual(JSON.parse(calls.find(call => call.method === 'PATCH').body), { tags: [], note: '' })
  assert.equal($('.favorite-tag'), null); assert.equal($('.favorite-note'), null); assert.equal($('.favorite-editor'), null)
  assert.equal(document.activeElement, $('.favorite-edit')); assert.equal($('.media-caption').textContent, '原始快照')
})

test('cancel and Escape discard drafts, restore edit focus, and textarea participates in the focus trap', async () => {
  handler = () => ({ favorites: [original], has_more: false }); const vm = await mount()
  await edit('草稿', '未保存\n备注')
  $('.favorite-cancel-edit').click(); await settle()
  assert.equal($('.favorite-editor'), null); assert.equal(document.activeElement, $('.favorite-edit'))
  await edit('草稿', '未保存')
  const textarea = $('.favorite-editor textarea'); textarea.focus(); key(textarea, 'Tab'); assert.equal(document.activeElement, textarea)
  const controls = [...$('.favorites-panel').querySelectorAll('button:not(:disabled),input:not(:disabled),textarea:not(:disabled),a[href],audio[controls],video[controls]')]
  assert.ok(controls.includes(textarea)); controls.at(-1).focus(); key(controls.at(-1), 'Tab'); assert.equal(document.activeElement, controls[0])
  key(textarea, 'Escape'); await settle(); assert.equal(vm.open, true); assert.equal($('.favorite-editor'), null); assert.equal(document.activeElement, $('.favorite-edit'))
  assert.equal(calls.filter(call => call.method === 'PATCH').length, 0)
  $('.favorite-edit').click(); await settle(); assert.equal($('.favorite-editor input').value, ''); assert.equal($('.favorite-editor textarea').value, '')
})

test('failed save retains input; pending save disables duplicates, remove and Escape cancellation', async () => {
  let rejectPatch
  handler = (url, options) => options.method === 'PATCH' ? new Promise((resolve, reject) => { rejectPatch = reject }) : { favorites: [original], has_more: false }
  const vm = await mount(); await edit('标签', '第一行\n第二行'); await save(); await save()
  assert.equal(calls.filter(call => call.method === 'PATCH').length, 1)
  for (const node of document.querySelectorAll('.favorite-editor input,.favorite-editor textarea,.favorite-editor button')) assert.equal(node.disabled, true)
  key($('.favorite-editor textarea'), 'Escape'); await settle(); assert.equal(vm.open, true); assert.ok($('.favorite-editor'))
  assert.equal($('[aria-label="移除收藏 作者"]').disabled, true)
  rejectPatch(new Error('English internal failure')); await settle()
  assert.match($('.favorite-editor [role="alert"]').textContent, /保存失败.*重试/)
  assert.equal($('.favorite-editor input').value, '标签'); assert.equal($('.favorite-editor textarea').value, '第一行\n第二行')
  assert.equal($('.favorite-editor button').disabled, false)
  handler = () => ({ ...original, tags: ['标签'], note: '第一行\n第二行' }); await save(); assert.equal($('.favorite-editor'), null)
})

test('Unicode code point limits accept emoji boundary and reject oversized tags, counts and note without PATCH', async () => {
  handler = (url, options) => options.method === 'PATCH' ? { ...original, ...JSON.parse(options.body) } : { favorites: [original], has_more: false }
  await mount(); await edit('😀'.repeat(21), '')
  await save(); assert.match($('.favorite-editor [role="alert"]').textContent, /20/)
  input('.favorite-editor input', Array.from({ length: 11 }, (_, i) => `标签${i}`).join(',')); await save(); assert.match($('.favorite-editor [role="alert"]').textContent, /10/)
  input('.favorite-editor input', '😀'.repeat(20)); input('.favorite-editor textarea', '😀'.repeat(2001)); await save(); assert.match($('.favorite-editor [role="alert"]').textContent, /2000/)
  assert.equal(calls.filter(call => call.method === 'PATCH').length, 0)
  input('.favorite-editor textarea', '😀'.repeat(2000)); await save(); assert.equal($('.favorite-editor'), null)
})

for (const outcome of ['success', 'failure']) test(`account switch isolates pending PATCH ${outcome}`, async () => {
  let resolvePatch, rejectPatch
  handler = (url, options) => options.method === 'PATCH' ? new Promise((resolve, reject) => { resolvePatch = resolve; rejectPatch = reject }) : { favorites: [original], has_more: false }
  const vm = await mount(); await edit('旧账号私密标签', '旧账号私密备注'); await save()
  const patch = calls.find(call => call.method === 'PATCH'); vm.user = 'two'; await settle(); assert.equal(patch.signal.aborted, true)
  if (outcome === 'success') resolvePatch({ ...original, tags: ['旧账号私密标签'], note: '旧账号私密备注' }); else rejectPatch(new Error('旧账号错误'))
  await settle(); assert.equal($('.favorite-editor'), null); assert.equal($('.favorite-note'), null); assert.doesNotMatch($('.favorites-panel').textContent, /旧账号/)
  $('.favorite-edit').click(); await settle(); assert.equal($('.favorite-editor input').value, ''); assert.equal($('.favorite-editor textarea').value, '')
})

for (const action of ['close', 'refresh', 'unmount']) test(`${action} invalidates pending editor and late completion`, async () => {
  let resolvePatch
  handler = (url, options) => options.method === 'PATCH' ? new Promise(resolve => { resolvePatch = resolve }) : { favorites: [original], has_more: false }
  const vm = await mount(); await edit('私密标签', '私密备注'); await save(); const patch = calls.find(call => call.method === 'PATCH')
  if (action === 'unmount') { app.unmount(); app = null } else if (action === 'refresh') { vm.open = false; await settle(); vm.open = true } else $('[aria-label="关闭收藏"]').click()
  await settle(); assert.equal(patch.signal.aborted, true)
  resolvePatch({ ...original, tags: ['私密标签'], note: '私密备注' }); await settle()
  assert.equal($('.favorite-note'), null); assert.equal($('.favorite-editor'), null)
})
