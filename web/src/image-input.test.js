import test from 'node:test'
import assert from 'node:assert/strict'
import {
  collectImages, describeImageType, hasTransferableFiles, inferTypeFromName,
  isImageFile, isInlineImageType, normalizeType, pastedName, prepareImage,
} from './image-input.js'

const file = (name, type, size = 16) => new File([new Uint8Array(size)], name, { type })

test('only the server inline types are treated as photos', () => {
  assert.equal(isInlineImageType('image/png'), true)
  assert.equal(isInlineImageType('image/jpeg'), true)
  assert.equal(isInlineImageType('IMAGE/GIF'), true)
  assert.equal(isInlineImageType('image/webp'), true)
  // These would be stored as downloadable files by the server.
  assert.equal(isInlineImageType('image/bmp'), false)
  assert.equal(isInlineImageType('image/svg+xml'), false)
  assert.equal(isInlineImageType(''), false)
  assert.equal(normalizeType('image/png; charset=binary'), 'image/png')
})

test('a missing type is recovered from the name, then defaults to PNG', () => {
  assert.equal(inferTypeFromName('shot.PNG'), 'image/png')
  assert.equal(inferTypeFromName('scan.jpeg'), 'image/jpeg')
  assert.equal(inferTypeFromName('archive.zip'), '')
  assert.equal(describeImageType({ type: 'image/webp', name: 'x.bin' }), 'image/webp')
  assert.equal(describeImageType({ type: '', name: 'shot.png' }), 'image/png')
  // A pasted screenshot with neither type nor extension is assumed to be PNG.
  assert.equal(describeImageType({ type: '', name: '' }), 'image/png')
})

test('image detection covers declared types and unnamed files by extension', () => {
  assert.equal(isImageFile(file('a.png', 'image/png')), true)
  assert.equal(isImageFile({ name: 'b.JPG', type: '' }), true)
  // A known non-image type is not an image even with an image-like name.
  assert.equal(isImageFile({ name: 'c.png', type: 'text/plain' }), false)
  assert.equal(isImageFile({ name: 'd.pdf', type: 'application/pdf' }), false)
  assert.equal(isImageFile(null), false)
})

test('clipboard items are read through getAsFile', () => {
  const png = file('image.png', 'image/png')
  const transfer = {
    items: [
      { kind: 'string', type: 'text/plain', getAsFile: () => null },
      { kind: 'file', type: 'image/png', getAsFile: () => png },
    ],
    files: [],
  }
  const found = collectImages(transfer)
  assert.equal(found.length, 1)
  assert.equal(found[0], png)
})

test('drag and drop falls back to the files list', () => {
  const dropped = file('photo.jpg', 'image/jpeg')
  const text = file('notes.txt', 'text/plain')
  const found = collectImages({ items: [], files: [dropped, text] })
  assert.deepEqual(found.map(entry => entry.name), ['photo.jpg'])
  // A transfer with no files at all yields nothing rather than throwing.
  assert.deepEqual(collectImages(undefined), [])
})

test('interception only happens for file-carrying transfers', () => {
  assert.equal(hasTransferableFiles({ types: ['Files'] }), true)
  assert.equal(hasTransferableFiles({ types: ['image/png'] }), true)
  assert.equal(hasTransferableFiles({ types: ['text/plain'] }), false)
  assert.equal(hasTransferableFiles(undefined), false)
})

test('a pasted image gets a readable name for downloads', () => {
  assert.equal(pastedName('image/png', 1700000000000), 'pasted-1700000000000.png')
  assert.equal(pastedName('image/jpeg', 1), 'pasted-1.jpg')
})

test('a supported image keeps its bytes and type', async () => {
  const original = file('shot.png', 'image/png')
  const prepared = await prepareImage(original)
  assert.equal(prepared, original)
})

test('an image with no declared type is rebuilt with a matching name', async () => {
  const nameless = new File([new Uint8Array(8)], '', { type: '' })
  const prepared = await prepareImage(nameless)
  assert.equal(prepared.type, 'image/png')
  assert.match(prepared.name, /\.png$/)
  // The bytes are preserved; only the type and name were corrected.
  assert.equal(prepared.size, nameless.size)
})

test('an unsupported image type is re-encoded so it stays a photo', async () => {
  const bmp = file('shot.bmp', 'image/bmp')
  let called = 0
  const prepared = await prepareImage(bmp, async source => {
    called++
    assert.equal(source, bmp)
    return new Blob([new Uint8Array(4)], { type: 'image/png' })
  })
  assert.equal(called, 1)
  assert.equal(prepared.type, 'image/png')
  assert.equal(prepared.name, 'shot.png')
})

test('a failed conversion falls back to the original file instead of failing', async () => {
  const bmp = file('shot.bmp', 'image/bmp')
  const prepared = await prepareImage(bmp, async () => { throw new Error('canvas unavailable') })
  assert.equal(prepared, bmp)
  // A converter that returns nothing also falls back.
  assert.equal(await prepareImage(bmp, async () => null), bmp)
})
