import test from 'node:test'
import assert from 'node:assert/strict'
import { api, clearAvatar, MAX_AVATAR_BYTES, uploadAvatar } from './api.js'

test('non-JSON API responses report a proxy configuration error', async () => {
  const originalDocument = globalThis.document
  const originalFetch = globalThis.fetch
  globalThis.document = { cookie: '' }
  globalThis.fetch = async () => new Response('<!DOCTYPE html>', {
    headers: { 'Content-Type': 'text/html' },
  })

  try {
    await assert.rejects(api('/auth/login'), /非 JSON 响应.*代理配置/)
  } finally {
    globalThis.document = originalDocument
    globalThis.fetch = originalFetch
  }
})

test('avatar upload sends raw image bytes, filename and CSRF protection', async () => {
  const originalDocument = globalThis.document
  const originalFetch = globalThis.fetch
  const file = new File(['png-bytes'], 'portrait.png', { type: 'image/png' })
  let request
  globalThis.document = { cookie: 'csrf=token%201' }
  globalThis.fetch = async (url, options) => {
    request = { url, options }
    return Response.json({ data: { id: 'me', avatar: 'media-id' } })
  }
  try {
    const user = await uploadAvatar(file)
    assert.equal(user.avatar, 'media-id')
    assert.match(request.url, /^\/api\/v1\/me\/avatar\?file_name=portrait.png$/)
    assert.equal(request.options.method, 'POST')
    assert.equal(request.options.headers['Content-Type'], 'image/png')
    assert.equal(request.options.headers['X-CSRF-Token'], 'token 1')
    assert.equal(request.options.body, file)
    assert.equal(request.options.credentials, 'same-origin')
  } finally {
    globalThis.document = originalDocument
    globalThis.fetch = originalFetch
  }
})

test('avatar upload rejects empty and over-limit images before sending', async () => {
  const originalDocument = globalThis.document
  const originalFetch = globalThis.fetch
  let calls = 0
  globalThis.document = { cookie: '' }
  globalThis.fetch = async () => { calls++; return Response.json({ data: {} }) }
  try {
    assert.equal(MAX_AVATAR_BYTES, 2 * 1024 * 1024)
    await assert.rejects(uploadAvatar(new File([], 'empty.png', { type: 'image/png' })), /不能超过 2 MB/)
    const tooLarge = new File([new Uint8Array(MAX_AVATAR_BYTES + 1)], 'large.png', { type: 'image/png' })
    await assert.rejects(uploadAvatar(tooLarge), /不能超过 2 MB/)
    assert.equal(calls, 0)
  } finally {
    globalThis.document = originalDocument
    globalThis.fetch = originalFetch
  }
})

test('clear avatar sends the authenticated delete request', async () => {
  const originalDocument = globalThis.document
  const originalFetch = globalThis.fetch
  let request
  globalThis.document = { cookie: '' }
  globalThis.fetch = async (url, options) => {
    request = { url, options }
    return Response.json({ data: { id: 'me', avatar: '' } })
  }
  try {
    const user = await clearAvatar()
    assert.equal(user.avatar, '')
    assert.equal(request.url, '/api/v1/me/avatar')
    assert.equal(request.options.method, 'DELETE')
  } finally {
    globalThis.document = originalDocument
    globalThis.fetch = originalFetch
  }
})

