import test from 'node:test'
import assert from 'node:assert/strict'
import { api } from './api.js'

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
