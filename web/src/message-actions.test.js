import test from 'node:test'
import assert from 'node:assert/strict'
import { isActionableMessage, messageCopyText, prepareForward } from './message-actions.js'

test('message action helpers exclude recalled and system messages', () => {
  assert.equal(isActionableMessage({ id: '1', kind: 'user' }), true)
  assert.equal(isActionableMessage({ id: '2', kind: 'user', recalled_at: 'now' }), false)
  assert.equal(isActionableMessage({ id: '3', kind: 'system' }), false)
})

test('copy text keeps text and media in the original order', () => {
  assert.equal(messageCopyText({ blocks: [
    { type: 'text', text: '先说' },
    { type: 'image', media_id: 'image-1', file_name: '照片.png' },
    { type: 'text', text: '再说' },
  ] }), '先说\n[图片] 照片.png\n再说')
  assert.equal(messageCopyText({ kind: 'file', media_id: 'file-1', media: { file_name: '说明.pdf' } }), '[文件] 说明.pdf')
})

test('forwarding an ordered message re-uploads media to the target conversation', async () => {
  const originalFetch = globalThis.fetch
  const originalFile = globalThis.File
  const originalDocument = globalThis.document
  globalThis.document = { cookie: '' }
  globalThis.File = class extends Blob {
    constructor(parts, name, options) { super(parts, options); this.name = name }
  }
  const calls = []
  globalThis.fetch = async (url, options = {}) => {
    calls.push({ url: String(url), options })
    if (String(url).includes('/api/v1/media/')) return new Response(new Blob(['image'], { type: 'image/png' }))
    return new Response(JSON.stringify({ data: { id: 'target-image' } }), { status: 201, headers: { 'Content-Type': 'application/json' } })
  }
  try {
    const payload = await prepareForward({ id: 'source', kind: 'user', blocks: [
      { type: 'text', text: '前' },
      { type: 'image', media_id: 'source-image', file_name: '图片.png' },
      { type: 'text', text: '后' },
    ] }, 'target-conversation', 'forward-client-id')
    assert.equal(payload.client_msg_id, 'forward-client-id')
    assert.deepEqual(payload.blocks, [
      { type: 'text', text: '前' },
      { type: 'image', media_id: 'target-image' },
      { type: 'text', text: '后' },
    ])
    assert.equal(calls.length, 2)
    assert.equal(calls[0].options.credentials, 'same-origin')
    assert.match(calls[1].url, /\/api\/v1\/conversations\/target-conversation\/media/)
  } finally {
    globalThis.fetch = originalFetch
    globalThis.File = originalFile
    globalThis.document = originalDocument
  }
})
