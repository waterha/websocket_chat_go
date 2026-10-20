import test from 'node:test'
import assert from 'node:assert/strict'
import { formatBytes, mediaKind, mediaPreview, mediaURL, validateMediaFile } from './media.js'

const file = (type, size) => ({ type, size })

test('media kind follows the declared content type only', () => {
  assert.equal(mediaKind('image/png'), 'image')
  assert.equal(mediaKind('IMAGE/JPEG; charset=binary'), 'image')
  assert.equal(mediaKind('video/mp4'), 'video')
  assert.equal(mediaKind('application/pdf'), 'file')
  assert.equal(mediaKind('text/plain'), 'file')
  assert.equal(mediaKind(''), 'file')
  assert.equal(mediaKind(undefined), 'file')
})

test('files are validated against per-type limits', () => {
  assert.deepEqual(validateMediaFile(file('image/webp', 2048)), { ok: true, kind: 'image' })
  assert.deepEqual(validateMediaFile(file('video/webm', 2048)), { ok: true, kind: 'video' })
  assert.deepEqual(validateMediaFile(file('application/pdf', 2048)), { ok: true, kind: 'file' })
  assert.equal(validateMediaFile(file('image/png', 10 * 1024 * 1024)).ok, true)
  assert.equal(validateMediaFile(file('image/png', 10 * 1024 * 1024 + 1)).ok, false)
  assert.equal(validateMediaFile(file('video/mp4', 100 * 1024 * 1024)).ok, true)
  assert.equal(validateMediaFile(file('video/mp4', 100 * 1024 * 1024 + 1)).ok, false)
  assert.equal(validateMediaFile(file('application/zip', 50 * 1024 * 1024)).ok, true)
  assert.equal(validateMediaFile(file('application/zip', 50 * 1024 * 1024 + 1)).ok, false)
  // A missing or malformed type cannot be uploaded because the server refuses it.
  assert.equal(validateMediaFile(file('', 10)).ok, false)
  assert.equal(validateMediaFile(file('not a type', 10)).ok, false)
  assert.equal(validateMediaFile(file('image/png', 0)).ok, false)
  assert.equal(validateMediaFile(null).ok, false)
})

test('sizes and previews render stable text', () => {
  assert.equal(formatBytes(512), '512 B')
  assert.equal(formatBytes(2048), '2.0 KB')
  assert.equal(formatBytes(3 * 1024 * 1024), '3.0 MB')
  assert.equal(formatBytes(undefined), '0 B')
  assert.equal(mediaPreview({ kind: 'image' }), '[图片]')
  assert.equal(mediaPreview({ kind: 'video' }), '[视频]')
  assert.equal(mediaPreview({ kind: 'file' }), '[文件]')
  assert.equal(mediaPreview({ kind: 'user', content: 'hi' }), '')
})

test('media urls encode the identifier', () => {
  assert.equal(mediaURL('abc'), '/api/v1/media/abc')
  assert.equal(mediaURL('a/b'), '/api/v1/media/a%2Fb')
})
