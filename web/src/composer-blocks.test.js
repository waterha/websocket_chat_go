import test from 'node:test'
import assert from 'node:assert/strict'
import {
  blocksToText, buildBlocks, countImages, insertToken, nextImageIndex,
  removeToken, tokenFor, tokensIn,
} from './composer-blocks.js'

const image = (index, mediaId) => ({ index, mediaId })

test('a token is inserted at the caret and settles on its own line', () => {
  // In the middle of text the token gets a line break on both sides.
  const middle = insertToken('前面的文字后面的文字', 2, 1)
  assert.equal(middle.text, '前面\n〔图1〕\n的文字后面的文字')
  assert.equal(middle.text.slice(middle.caret), '的文字后面的文字')
  // At the end of a line only the leading break is added.
  const atEnd = insertToken('一行文字', 4, 1)
  assert.equal(atEnd.text, '一行文字\n〔图1〕')
  // An empty draft takes the token alone.
  assert.equal(insertToken('', 0, 1).text, '〔图1〕')
})

test('image numbers are reused after a removal', () => {
  assert.equal(nextImageIndex([]), 1)
  assert.equal(nextImageIndex([image(1, 'a')]), 2)
  assert.equal(nextImageIndex([image(1, 'a'), image(2, 'b')]), 3)
  // A gap is filled rather than always growing.
  assert.equal(nextImageIndex([image(1, 'a'), image(3, 'c')]), 2)
})

test('removing an image drops only its token', () => {
  const text = '看这张\n〔图1〕\n再看这张\n〔图2〕'
  assert.equal(removeToken(text, 1), '看这张\n\n再看这张\n〔图2〕')
  assert.equal(removeToken(text, 2), '看这张\n〔图1〕\n再看这张\n')
})

test('tokens are listed in the order they appear, without repeats', () => {
  assert.deepEqual(tokensIn('a〔图2〕b〔图1〕c'), [2, 1])
  assert.deepEqual(tokensIn('〔图1〕x〔图1〕'), [1])
  assert.deepEqual(tokensIn('没有图片'), [])
})

test('the draft becomes blocks in the typed order', () => {
  const text = '先看这张\n〔图1〕\n再看下面这张\n〔图2〕\n两者对比即可'
  const blocks = buildBlocks(text, [image(1, 'media-a'), image(2, 'media-b')])
  assert.deepEqual(blocks, [
    { type: 'text', text: '先看这张' },
    { type: 'image', media_id: 'media-a' },
    { type: 'text', text: '再看下面这张' },
    { type: 'image', media_id: 'media-b' },
    { type: 'text', text: '两者对比即可' },
  ])
})

test('an image alone produces a single block', () => {
  assert.deepEqual(buildBlocks('〔图1〕', [image(1, 'media-a')]), [{ type: 'image', media_id: 'media-a' }])
})

test('blank text segments are dropped and inner line breaks survive', () => {
  const blocks = buildBlocks('\n\n〔图1〕\n\n  一段文字\n第二行  \n\n', [image(1, 'media-a')])
  assert.deepEqual(blocks, [
    { type: 'image', media_id: 'media-a' },
    { type: 'text', text: '一段文字\n第二行' },
  ])
  // Only whitespace and no images yields nothing, so an empty send is refused.
  assert.deepEqual(buildBlocks('   \n  ', []), [])
})

test('a token without a known image is kept as text rather than deleted', () => {
  // This happens if an upload failed after the token was placed.
  assert.deepEqual(buildBlocks('说明〔图7〕', []), [{ type: 'text', text: '说明〔图7〕' }])
})

test('blocks render back to the token form for a restored draft', () => {
  const images = [image(1, 'media-a'), image(2, 'media-b')]
  const text = blocksToText([
    { type: 'text', text: '第一段' },
    { type: 'image', media_id: 'media-a' },
    { type: 'text', text: '第二段' },
  ], images)
  assert.equal(text, '第一段\n〔图1〕\n第二段')
  assert.deepEqual(buildBlocks(text, images), [
    { type: 'text', text: '第一段' },
    { type: 'image', media_id: 'media-a' },
    { type: 'text', text: '第二段' },
  ])
  assert.equal(countImages(buildBlocks(text, images)), 1)
})
