import test from 'node:test'
import assert from 'node:assert/strict'
import { register } from 'node:module'
import { JSDOM } from 'jsdom'

register('../css-hook.mjs', import.meta.url)

const dom = new JSDOM('<div id="app"></div>', { url: 'http://127.0.0.1:4173' })
const originalNavigator = Object.getOwnPropertyDescriptor(globalThis, 'navigator')
const originalMediaDevices = Object.getOwnPropertyDescriptor(dom.window.navigator, 'mediaDevices')
const originalRecorder = Object.getOwnPropertyDescriptor(globalThis, 'MediaRecorder')
for (const key of ['window', 'document', 'location', 'HTMLElement', 'Element', 'Node', 'SVGElement', 'Text', 'Comment', 'DocumentFragment', 'Document', 'DOMParser', 'ShadowRoot', 'MutationObserver', 'File', 'Blob', 'Range']) globalThis[key] = dom.window[key]
Object.defineProperty(globalThis, 'navigator', { configurable: true, value: dom.window.navigator })
Object.defineProperty(dom.window.navigator, 'mediaDevices', { configurable: true, value: { getUserMedia: async () => stream } })
globalThis.getComputedStyle = dom.window.getComputedStyle.bind(dom.window)
globalThis.localStorage = dom.window.localStorage
globalThis.sessionStorage = dom.window.sessionStorage
globalThis.WebSocket = class { close() {} }
globalThis.URL.createObjectURL = () => 'blob:voice-preview'
globalThis.URL.revokeObjectURL = () => {}

let trackStops = 0
const stream = { getTracks: () => [{ stop() { trackStops++ } }] }
class MockRecorder {
  static instances = []
  static isTypeSupported(type) { return type === 'audio/webm;codecs=opus' }
  constructor(_stream, options = {}) {
    this.mimeType = options.mimeType || 'audio/webm'
    this.state = 'inactive'
    MockRecorder.instances.push(this)
  }
  start() { this.state = 'recording' }
  stop() {
    if (this.state === 'inactive') return
    this.state = 'inactive'
    this.ondataavailable?.({ data: new Blob(['voice bytes'], { type: this.mimeType }) })
    this.onstop?.()
  }
}
globalThis.MediaRecorder = MockRecorder

const me = { id: 'me', name: 'Me', email: 'me@test' }
const conversations = [{ id: 'one', name: 'one', kind: 'direct', members: [{ ...me, role: 'member' }], last_seq: 0 }]
const uploads = []
const sent = []
let audioMessage = null
let uploadID = 0
globalThis.fetch = async (url, options = {}) => {
  const path = String(url)
  let data = []
  if (path.includes('/media?')) {
    uploads.push({ path, options })
    uploadID++
    data = {
      id: `audio-${uploadID}`,
      file_name: options.body.name,
      media_type: 'audio',
      content_type: options.body.type,
      byte_size: options.body.size,
    }
  } else if (path.endsWith('/messages') && options.method === 'POST') {
    const payload = JSON.parse(options.body)
    sent.push(payload)
    audioMessage = {
      id: 'message-1', conversation_id: 'one', sender_id: 'me', seq: 1,
      kind: 'audio', content: payload.content, media_id: payload.media_id,
      media: { file_name: uploads.at(-1).options.body.name, content_type: uploads.at(-1).options.body.type, byte_size: uploads.at(-1).options.body.size },
      blocks: payload.blocks, mentions: [], created_at: '2026-10-29T06:00:00Z',
    }
    data = { id: audioMessage.id, conversation_id: 'one', seq: 1 }
  } else if (path.endsWith('/me')) data = me
  else if (path.endsWith('/friends/requests')) data = { incoming: [], outgoing: [] }
  else if (path === '/api/v1/conversations') data = conversations
  else if (path.includes('/sync')) data = { messages: audioMessage ? [audioMessage] : [], last_seq: audioMessage ? 1 : 0, next_seq: audioMessage ? 1 : 0, has_more: false }
  return { ok: true, status: 200, json: async () => ({ data }) }
}

const realTimeout = globalThis.setTimeout
const realInterval = globalThis.setInterval
const timers = new Set()
globalThis.setTimeout = (...args) => { const id = realTimeout(...args); timers.add(id); return id }
globalThis.setInterval = (...args) => { const id = realInterval(...args); timers.add(id); return id }
const tick = () => new Promise(resolve => realTimeout(resolve, 0))
const waitFor = async predicate => {
  for (let i = 0; i < 100; i++) {
    if (predicate()) return
    await new Promise(resolve => realTimeout(resolve, 10))
  }
  assert.fail('timed out waiting for app state')
}

await import('./main.js')
await new Promise(resolve => realTimeout(resolve, 100))
const app = document.querySelector('#app').__vue_app__
const vm = app._instance.setupState
vm.active = 'one'
await new Promise(resolve => realTimeout(resolve, 100))
const voiceButton = () => document.querySelector('button[aria-label="语音"]')
function key(type, keyValue = ' ', code = 'Space') {
  const event = new dom.window.KeyboardEvent(type, { key: keyValue, code, bubbles: true, cancelable: true })
  document.body.dispatchEvent(event)
}

const originalNow = Date.now
let now = 1_800_000_000_000
test('toolbar voice recording uploads and sends playable audio; Escape cancels without upload', async () => {
  Date.now = () => now
  try {
    assert.ok(voiceButton())
    voiceButton().click()
    key('keydown')
    await waitFor(() => MockRecorder.instances.length === 1)
    key('keydown', 'Escape', 'Escape')
    key('keyup')
    await tick()
    assert.equal(uploads.length, 0)
    assert.equal(sent.length, 0)
    assert.equal(trackStops, 1)

    key('keydown')
    await waitFor(() => MockRecorder.instances.length === 2)
    now += 350
    key('keyup')
    await waitFor(() => sent.length === 1 && document.querySelector('.stream audio.media-audio'))

    assert.equal(uploads.length, 1)
    assert.equal(uploads[0].options.body.type, 'audio/webm')
    assert.match(uploads[0].options.body.name, /\.webm$/)
    assert.equal(sent[0].media_id, 'audio-1')
    assert.ok(sent[0].media_id)
    assert.deepEqual(sent[0].blocks, [])
    assert.equal(sent[0].content, '')
    const player = document.querySelector('.stream audio.media-audio')
    assert.ok(player)
    assert.equal(player.controls, true)
    assert.equal(player.getAttribute('src'), '/api/v1/media/audio-1')
  } finally {
    Date.now = originalNow
  }
})

test.after(() => {
  app.unmount()
  for (const id of timers) { clearInterval(id); clearTimeout(id) }
  globalThis.setTimeout = realTimeout
  globalThis.setInterval = realInterval
  if (originalRecorder) Object.defineProperty(globalThis, 'MediaRecorder', originalRecorder)
  else delete globalThis.MediaRecorder
  if (originalMediaDevices) Object.defineProperty(dom.window.navigator, 'mediaDevices', originalMediaDevices)
  else delete dom.window.navigator.mediaDevices
  if (originalNavigator) Object.defineProperty(globalThis, 'navigator', originalNavigator)
  else delete globalThis.navigator
  dom.window.close()
})
