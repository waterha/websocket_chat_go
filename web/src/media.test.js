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
  // Explicitly malformed types still cannot be uploaded.
  assert.equal(validateMediaFile(file('not a type', 10)).ok, false)
  assert.equal(validateMediaFile(file('image/png', 0)).ok, false)
  assert.equal(validateMediaFile(null).ok, false)
})

test('unknown file types use the ordinary file limit and reject empty files', () => {
  const limit = 50 * 1024 * 1024
  for (const type of ['', undefined]) {
    assert.deepEqual(validateMediaFile(file(type, 1)), { ok: true, kind: 'file' })
    assert.deepEqual(validateMediaFile(file(type, limit)), { ok: true, kind: 'file' })
    assert.deepEqual(validateMediaFile(file(type, limit + 1)), { ok: false, error: '文件最大 50 MB' })
    assert.deepEqual(validateMediaFile(file(type, 0)), { ok: false, error: '文件为空，请重新选择' })
  }
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

test('voice recordings have an audio kind and a ten MiB limit', () => {
  assert.equal(mediaKind('audio/webm;codecs=opus'), 'audio')
  assert.deepEqual(validateMediaFile(file('audio/ogg', 10 * 1024 * 1024)), { ok: true, kind: 'audio' })
  assert.deepEqual(validateMediaFile(file('audio/webm', 10 * 1024 * 1024 + 1)), { ok: false, error: '语音最大 10 MB' })
  assert.equal(mediaPreview({ kind: 'audio' }), '[语音]')
})

test('media urls encode the identifier', () => {
  assert.equal(mediaURL('abc'), '/api/v1/media/abc')
  assert.equal(mediaURL('a/b'), '/api/v1/media/a%2Fb')
})
