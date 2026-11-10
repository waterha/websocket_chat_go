import test from 'node:test'
import assert from 'node:assert/strict'
import { register } from 'node:module'
import { JSDOM } from 'jsdom'
register('../css-hook.mjs', import.meta.url)
const dom = new JSDOM('<!doctype html><html><body><button id="trigger">收藏入口</button><div id="app"></div></body></html>', { url: 'http://localhost' })
for (const key of ['window', 'document', 'HTMLElement', 'Element', 'Node', 'SVGElement', 'Text', 'Comment', 'DocumentFragment']) globalThis[key] = key === 'window' ? dom.window : key === 'document' ? dom.window.document : dom.window[key]
const { createApp, ref, nextTick } = await import('vue/dist/vue.esm-bundler.js')
const { FavoritesPanel, useFavorites } = await import('./favorites.js')
const { SavedMessageContent } = await import('./saved-message-content.js')
const { useMessageActions } = await import('./message-actions.js')
const calls = []
let handler = () => ({ favorites: [], has_more: false })
globalThis.fetch = async (url, options = {}) => { calls.push({ url: String(url), ...options }); const data = await handler(String(url), options); return { ok: true, json: async () => ({ data }) } }
const settle = async () => { await nextTick(); await new Promise(resolve => setTimeout(resolve, 5)); await nextTick() }
const snapshot = (id = 'favorite-1', extra = {}) => ({ id, sender_name: '小明', conversation_name: '来源群', conversation_kind: 'group', is_own: true, saved_at: '2026-01-01T10:00:00Z', message: { id: 'message-1', sender_id: 'me', kind: 'user', created_at: '2026-01-01T09:00:00Z', content: '完整正文' }, ...extra })
let app
function mount(component, props = {}) { app = createApp(component, props); const host = document.createElement('div'); document.body.append(host); const vm = app.mount(host); return { vm, host } }
async function cleanup() { app?.unmount(); app = null; await settle(); calls.length = 0 }
test.after(() => { app?.unmount(); dom.window.close() })

test('saved content preserves ordered text, newlines, quotes and every private media type safely', async () => {
  const favorite = snapshot('saved/id', { message: { kind: 'user', reply: { sender_name: '引用作者', excerpt: '引用\n原文' }, blocks: [
    { type: 'text', text: '第一行\n第二行<script>alert(1)</script>' }, { type: 'image', media_id: 'image/id', file_name: '图.png' }, { type: 'text', text: '图片之后' }, { type: 'video', media_id: 'video', file_name: '片.mp4' }, { type: 'audio', media_id: 'audio', file_name: '音.wav' }, { type: 'file', media_id: 'file', file_name: '文件.pdf' },
  ] } })
  const { host } = mount(SavedMessageContent, { favorite }); await settle()
  assert.ok(host.querySelector('.saved-message-content.mine .bubble .block-stack'))
  assert.deepEqual([...host.querySelector('.block-stack').children].map(node => node.className), ['block-text', 'block-image', 'block-text', 'media-video', 'media-audio', 'block-file'])
  assert.equal(host.querySelector('.block-text').textContent, '第一行\n第二行<script>alert(1)</script>')
  assert.equal(host.querySelector('script'), null)
  assert.equal(host.querySelector('.reply-quote strong').textContent, '引用作者')
  assert.equal(host.querySelector('.reply-quote span').textContent, '引用\n原文')
  for (const node of host.querySelectorAll('[src],a[href]')) assert.match(node.getAttribute('src') || node.getAttribute('href'), /^\/api\/v1\/favorites\/saved%2Fid\/media\//)
  await cleanup()
})

test('single attachment rendering retains chat media classes, caption and private URLs', async () => {
  for (const kind of ['image', 'video', 'audio', 'file']) {
    const { host } = mount(SavedMessageContent, { favorite: snapshot('fav', { message: { kind, media_id: 'asset', media: { file_name: '附件', byte_size: 42 }, content: '说明\n换行' } }) }); await settle()
    assert.equal(host.querySelector('.media-caption').textContent, '说明\n换行')
    assert.ok(host.querySelector(`.media-${kind}`))
    for (const node of host.querySelectorAll('[src],a[href]')) assert.equal(node.getAttribute('src') || node.getAttribute('href'), '/api/v1/favorites/fav/media/asset')
    await cleanup()
  }
})

test('panel loads, searches, pages with UUID cursor and retries failed pages without losing entries', async () => {
  let fail = true
  handler = (url) => {
    if (!url.includes('before_id=')) return { favorites: [snapshot()], has_more: true, next_before_id: 'uuid-cursor' }
    if (fail) throw new Error('网络中断')
    return { favorites: [snapshot('favorite-2', { sender_name: '另一位' })], has_more: false }
  }
  mount(FavoritesPanel, { open: true, userID: 'me' }); await settle()
  assert.equal(document.activeElement.getAttribute('aria-label'), '搜索收藏')
  assert.match(document.querySelector('.favorite-entry').textContent, /来源群.*我发送的消息/s)
  assert.match(document.querySelector('.message-meta').textContent, /小明/)
  document.querySelector('.favorites-more').click(); await settle()
  assert.match(calls.at(-1).url, /before_id=uuid-cursor/)
  assert.match(document.querySelector('[role="alert"]').textContent, /网络中断/)
  assert.equal(document.querySelectorAll('.favorite-entry').length, 1)
  fail = false; document.querySelector('[role="alert"] button').click(); await settle()
  assert.equal(document.querySelectorAll('.favorite-entry').length, 2)
  const input = document.querySelector('[aria-label="搜索收藏"]'); input.value = '另一位'; input.dispatchEvent(new dom.window.Event('input', { bubbles: true })); await settle()
  assert.equal(document.querySelectorAll('.favorite-entry').length, 1)
  await cleanup()
})

test('panel removal needs confirmation, retries errors, traps focus and restores the opener on Escape', async () => {
  let fail = true
  handler = (url, options) => { if (options.method === 'DELETE') { if (fail) throw new Error('删除失败'); return {} } return { favorites: [snapshot()], has_more: false } }
  document.querySelector('#trigger').focus()
  const Root = { components: { FavoritesPanel }, setup() { return { open: ref(true) } }, template: '<FavoritesPanel v-model:open="open" userID="me" />' }
  mount(Root); await settle()
  document.querySelector('[aria-label="移除收藏 小明"]').click(); await settle()
  assert.equal(calls.filter(call => call.method === 'DELETE').length, 0)
  assert.equal(document.activeElement.textContent, '取消')
  document.querySelector('.favorites-confirm .danger').click(); await settle()
  assert.match(document.querySelector('.favorites-confirm [role="alert"]').textContent, /删除失败/)
  fail = false; document.querySelector('.favorites-confirm .danger').click(); await settle()
  assert.equal(document.querySelectorAll('.favorite-entry').length, 0)
  const controls = [...document.querySelector('.favorites-panel').querySelectorAll('button:not(:disabled),input:not(:disabled),textarea:not(:disabled),a[href],audio[controls],video[controls]')]
  controls.at(-1).focus(); controls.at(-1).dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true })); assert.equal(document.activeElement, controls[0])
  controls[0].dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); await settle()
  assert.equal(document.querySelector('.favorites-panel'), null)
  assert.equal(document.activeElement.id, 'trigger')
  await cleanup()
})

test('identity switches discard stale results', async () => {
  let resolve
  handler = () => new Promise(done => { resolve = done })
  const Root = { components: { FavoritesPanel }, setup() { return { user: ref('one') } }, template: '<FavoritesPanel :open="true" :userID="user" />' }
  const { vm } = mount(Root); await settle(); const finishOld = resolve
  vm.user = 'two'; await settle(); finishOld({ favorites: [snapshot('stale')], has_more: false }); await settle()
  assert.equal(document.querySelectorAll('.favorite-entry').length, 0)
  resolve({ favorites: [snapshot('fresh')], has_more: false }); await settle()
  assert.equal(document.querySelector('.favorite-entry').dataset.favoriteId, 'fresh')
  await cleanup()
})

test('favorite menu saves thread snapshots once, exposes busy and success/error and rejects ineligible messages', async () => {
  let resolve
  const item = { id: 'thread-reply', kind: 'user', conversation_id: 'source', content: '回复全文' }
  handler = () => new Promise(done => { resolve = done })
  const Root = { setup() { const favorite = useFavorites(); return { actions: useMessageActions({ messages: ref([]), threadRoot: ref(null), threadReplies: ref([item]), active: ref('source'), me: ref({ id: 'me' }), current: ref({}), conversations: ref([]), showError() {}, onFavorite: favorite.save }) } }, template: '<div />' }
  const { vm } = mount(Root)
  await vm.actions.openMenu({ clientX: 1, clientY: 1 }, item)
  vm.actions.runMenu('favorite'); vm.actions.runMenu('favorite'); await settle()
  assert.equal(vm.actions.favoriteBusy, item.id); assert.equal(calls.length, 1)
  assert.deepEqual(JSON.parse(calls[0].body), { conversation_id: 'source', message_id: 'thread-reply' })
  resolve(snapshot()); await settle(); assert.equal(vm.actions.favoriteNotice, '已收藏')
  handler = () => { throw new Error('保存失败') }
  await vm.actions.openMenu({}, item); vm.actions.runMenu('favorite'); await settle(); assert.equal(vm.actions.favoriteError, '保存失败')
  for (const extra of [{ recalled_at: 'now' }, { kind: 'system' }]) { await vm.actions.openMenu({}, { ...item, ...extra }); assert.equal(vm.actions.menu, null) }
  await cleanup()
})
