import test from 'node:test'
import assert from 'node:assert/strict'
import { hasBlocks, messageSegments, nextImageIndex } from './composer-blocks.js'

const image = (index, mediaId) => ({ index, mediaId })

test('image numbers are reused after a removal', () => {
  assert.equal(nextImageIndex([]), 1)
  assert.equal(nextImageIndex([image(1, 'a')]), 2)
  assert.equal(nextImageIndex([image(1, 'a'), image(2, 'b')]), 3)
  // A gap is filled rather than always growing.
  assert.equal(nextImageIndex([image(1, 'a'), image(3, 'c')]), 2)
})

test('a blocks message renders its segments in the stored order', () => {
  const message = {
    blocks: [
      { type: 'text', text: '看这张' },
      { type: 'image', media_id: 'media-a', file_name: 'a.png', byte_size: 12 },
      { type: 'text', text: '就是这样' },
    ],
  }
  assert.deepEqual(messageSegments(message), [
    { type: 'text', text: '看这张' },
    { type: 'image', mediaId: 'media-a', fileName: 'a.png', byteSize: 12 },
    { type: 'text', text: '就是这样' },
  ])
  assert.equal(hasBlocks(message), true)
})

test('an older single-attachment message still renders', () => {
  const message = { kind: 'image', media_id: 'media-old', content: '说明', media: { file_name: 'old.png', byte_size: 5 } }
  assert.deepEqual(messageSegments(message), [
    { type: 'image', mediaId: 'media-old', fileName: 'old.png', byteSize: 5 },
    { type: 'text', text: '说明' },
  ])
  // It must not take the blocks path.
  assert.equal(hasBlocks(message), false)
})

test('a plain text message renders as one text segment', () => {
  assert.deepEqual(messageSegments({ content: '你好' }), [{ type: 'text', text: '你好' }])
  assert.equal(hasBlocks({ content: '你好' }), false)
})

test('a video and a generic file keep their kind', () => {
  assert.equal(messageSegments({ kind: 'video', media_id: 'v' })[0].type, 'video')
  assert.equal(messageSegments({ kind: 'file', media_id: 'f' })[0].type, 'file')
})

test('a recalled message renders nothing, blocks included', () => {
  const recalled = {
    recalled_at: new Date().toISOString(),
    content: 'secret',
    blocks: [{ type: 'image', media_id: 'media-a' }],
  }
  assert.deepEqual(messageSegments(recalled), [])
  assert.equal(hasBlocks(recalled), false)
})

test('an empty or missing block list falls back to the old shape', () => {
  assert.equal(hasBlocks({ blocks: [] }), false)
  assert.equal(hasBlocks({}), false)
  assert.deepEqual(messageSegments({ blocks: [], content: '回退' }), [{ type: 'text', text: '回退' }])
})
