import test from 'node:test'
import assert from 'node:assert/strict'
import { stickerCandidate, useStickers } from './stickers.js'

test('sticker candidates distinguish pure emoji, selected images, mixed content and forbidden messages', () => {
  assert.deepEqual(stickerCandidate({ id: 'e', content: '😀 👍🏽' }), { text: '😀 👍🏽' })
  assert.deepEqual(stickerCandidate({ id: 'e', blocks: [{ type: 'text', text: '😀' }, { type: 'text', text: '🎉' }] }), { text: '😀🎉' })
  const mixed = { id: 'm', blocks: [{ type: 'text', text: '文字' }, { type: 'image', media_id: 'a' }, { type: 'image', media_id: 'b' }] }
  assert.equal(stickerCandidate(mixed), null)
  assert.deepEqual(stickerCandidate(mixed, 'b'), { media_id: 'b' })
  assert.equal(stickerCandidate(mixed, 'outside'), null)
  assert.deepEqual(stickerCandidate({ id: 'p', kind: 'image', media_id: 'p', content: '说明' }), { media_id: 'p' })
  for (const item of [null, { id: 'x', content: 'hi😀' }, { id: 'x', kind: 'file', media_id: 'f', content: '😀' }, { id: 'x', kind: 'system', content: '😀' }, { id: 'x', recalled_at: 'now', kind: 'image', media_id: 'p' }]) assert.equal(stickerCandidate(item), null)
})
test('sticker API deduplicates saves, only submits source identities and prepares an authorized target copy', async t => {
  const oldDocument = globalThis.document, oldFetch = globalThis.fetch
  globalThis.document = { cookie: 'csrf=token' }
  const calls = []; let release
  globalThis.fetch = async (path, options) => {
    calls.push({ path, options })
    if (path === '/api/v1/stickers' && options.method === 'POST') await new Promise(resolve => { release = resolve })
    return { ok: true, json: async () => ({ data: path.endsWith('/prepare') ? { id: 'target-media', media_type: 'image' } : { stickers: [{ id: 'saved' }] } }) }
  }
  t.after(() => { globalThis.document = oldDocument; globalThis.fetch = oldFetch })
  const stickers = useStickers(), message = { id: 'source', kind: 'image', media_id: 'photo' }
  const first = stickers.save(message, 'chat'), duplicate = stickers.save(message, 'chat')
  assert.equal(first, duplicate)
  assert.equal(calls.length, 1)
  assert.deepEqual(JSON.parse(calls[0].options.body), { conversation_id: 'chat', message_id: 'source', media_id: 'photo' })
  assert.equal(calls[0].options.headers['X-CSRF-Token'], 'token')
  release(); await first
  assert.deepEqual(await stickers.list(), [{ id: 'saved' }])
  assert.equal((await stickers.prepare({ id: 'saved/id' }, 'target')).id, 'target-media')
  assert.equal(calls.at(-1).path, '/api/v1/stickers/saved%2Fid/prepare')
  assert.deepEqual(JSON.parse(calls.at(-1).options.body), { conversation_id: 'target' })
  await assert.rejects(stickers.save({ id: 'text', content: '非表情' }, 'chat'), /右键图片或纯表情/)
})
