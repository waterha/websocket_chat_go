import test from 'node:test'
import assert from 'node:assert/strict'
import { ConversationDraftStore, normalizeDraft } from './conversation-drafts.js'

function storage() { const data = new Map(); return { getItem: key => data.get(key) || null, setItem: (key, value) => data.set(key, value), removeItem: key => data.delete(key) } }
const sample = () => ({ blocks: [{ type: 'text', text: '第一行\n  保留空白\n' }, { type: 'image', media_id: 'photo' }, { type: 'text', text: '末尾' }], images: [{ mediaId: 'photo', fileName: '照片.png', previewURL: 'blob:temporary' }], attachment: null })

test('drafts retain ordered text, whitespace and images across reload with account and chat isolation', () => {
  const disk = storage(), first = new ConversationDraftStore(disk)
  first.write('alice', 'group', sample())
  first.write('alice', 'direct', { blocks: [{ type: 'text', text: '私聊草稿' }] })
  const restored = new ConversationDraftStore(disk)
  assert.deepEqual(restored.read('alice', 'group').blocks, sample().blocks)
  assert.equal(restored.read('alice', 'direct').blocks[0].text, '私聊草稿')
  assert.deepEqual(restored.read('bob', 'group').blocks, [])
  assert.equal(restored.read('alice', 'group').images[0].previewURL, undefined)
})
test('uploaded attachments persist and only the sent snapshot is cleared', () => {
  const disk = storage(), store = new ConversationDraftStore(disk)
  const sent = { blocks: [{ type: 'text', text: '说明' }], attachment: { id: 'upload', file_name: '附件.zip', media_type: 'file' } }
  store.write('me', 'chat', sent)
  assert.equal(new ConversationDraftStore(disk).read('me', 'chat').attachment.id, 'upload')
  store.write('me', 'chat', { blocks: [{ type: 'text', text: '等待发送时新增内容' }] })
  store.clearIfMatches('me', 'chat', sent)
  assert.equal(store.read('me', 'chat').blocks[0].text, '等待发送时新增内容')
  store.clearIfMatches('me', 'chat', store.read('me', 'chat'))
  assert.deepEqual(store.read('me', 'chat').blocks, [])
})
test('storage failure keeps the draft in memory and reports once', () => {
  let warnings = 0
  const store = new ConversationDraftStore({ getItem() { throw Error() }, setItem() { throw Error() }, removeItem() { throw Error() } }, () => warnings++)
  store.write('me', 'chat', sample()); store.write('me', 'chat', sample())
  assert.equal(warnings, 1)
  assert.deepEqual(store.read('me', 'chat').blocks, sample().blocks)
})
test('stored drafts restore structured data instead of arbitrary editor markup', () => {
  const result = normalizeDraft({ html: '<script>bad()</script>', blocks: [{ type: 'html', text: '<img onerror=bad()>' }, { type: 'image', media_id: 'unknown' }, { type: 'text', text: '<b>text</b>' }], images: [] })
  assert.deepEqual(result.blocks, [{ type: 'text', text: '<b>text</b>' }])
  assert.equal(result.html, undefined)
})
