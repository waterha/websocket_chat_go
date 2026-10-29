import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'
import { useVoice } from './voice.js'

class MockRecorder {
  static instances = []
  static isTypeSupported(type) { return type === 'audio/webm;codecs=opus' }
  constructor(stream, options = {}) {
    this.stream = stream
    this.mimeType = options.mimeType || 'audio/webm'
    this.state = 'inactive'
    MockRecorder.instances.push(this)
  }
  start() { this.state = 'recording' }
  stop() {
    if (this.state === 'inactive') return
    this.state = 'inactive'
    this.ondataavailable?.({ data: new Blob(['voice'], { type: this.mimeType }) })
    this.onstop?.()
  }
}

function makeHarness({ getUserMedia, canRecord = () => true, isCurrent = () => true } = {}) {
  const dom = new JSDOM('<!doctype html><body></body>')
  const errors = [], sends = [], streams = []
  function createStream() {
    const stream = { stopped: 0, getTracks: () => [{ stop() { stream.stopped++ } }] }
    streams.push(stream)
    return stream
  }
  const voice = useVoice({
    canRecord,
    getContext: () => ({ chatId: 'chat-1' }),
    isCurrent,
    onSend: async (...args) => sends.push(args),
    showError: error => errors.push(error),
    documentRef: dom.window.document,
    windowRef: dom.window,
    mediaDevices: { getUserMedia: getUserMedia || (async () => createStream()) },
    Recorder: MockRecorder,
  })
  return { dom, voice, errors, sends, streams, createStream }
}

function key(dom, type, { key = ' ', code = 'Space', repeat = false } = {}) {
  const event = new dom.window.KeyboardEvent(type, { key, code, repeat, bubbles: true, cancelable: true })
  dom.window.document.body.dispatchEvent(event)
  return event
}

const flush = () => new Promise(resolve => setImmediate(resolve))
const enable = harness => harness.voice.toggle()

function withClock(callback) {
  const originalNow = Date.now
  let now = 1000
  Date.now = () => now
  return Promise.resolve().then(() => callback({ advance: ms => { now += ms } })).finally(() => { Date.now = originalNow })
}

test('holding Space and repeated keydown starts one recording; keyup sends once', async () => withClock(async ({ advance }) => {
  MockRecorder.instances = []
  const h = makeHarness()
  enable(h)
  key(h.dom, 'keydown')
  await flush()
  key(h.dom, 'keydown', { repeat: true })
  key(h.dom, 'keydown')
  assert.equal(MockRecorder.instances.length, 1)
  advance(350)
  key(h.dom, 'keyup')
  await flush()
  await flush()
  assert.equal(h.sends.length, 1)
  assert.equal(h.voice.state.value, 'idle')
  assert.equal(h.streams[0].stopped, 1)
  h.voice.dispose()
  h.dom.window.close()
}))

test('Escape cancels recording, stops tracks, and never sends', async () => {
  MockRecorder.instances = []
  const h = makeHarness()
  enable(h)
  key(h.dom, 'keydown')
  await flush()
  key(h.dom, 'keydown', { key: 'Escape', code: 'Escape' })
  await flush()
  assert.equal(h.streams[0].stopped, 1)
  assert.equal(h.sends.length, 0)
  assert.equal(h.voice.state.value, 'idle')
  h.voice.dispose()
  h.dom.window.close()
})

test('permission denial reports the microphone permission error', async () => {
  const error = Object.assign(new Error('denied'), { name: 'NotAllowedError' })
  const h = makeHarness({ getUserMedia: async () => { throw error } })
  enable(h)
  key(h.dom, 'keydown')
  await flush()
  assert.match(h.errors[0]?.message || '', /麦克风权限未开启/)
  assert.equal(h.voice.state.value, 'idle')
  h.voice.dispose()
  h.dom.window.close()
})

test('keyup while permission is pending cancels and releases the late stream', async () => {
  MockRecorder.instances = []
  let resolvePermission
  const h = makeHarness({ getUserMedia: () => new Promise(resolve => { resolvePermission = resolve }) })
  enable(h)
  key(h.dom, 'keydown')
  key(h.dom, 'keyup')
  const stream = h.createStream()
  resolvePermission(stream)
  await flush()
  assert.equal(MockRecorder.instances.length, 0)
  assert.equal(stream.stopped, 1)
  assert.equal(h.sends.length, 0)
  h.voice.dispose()
  h.dom.window.close()
})

test('window blur cancels recording and releases its stream', async () => {
  MockRecorder.instances = []
  const h = makeHarness()
  enable(h)
  key(h.dom, 'keydown')
  await flush()
  h.dom.window.dispatchEvent(new h.dom.window.Event('blur'))
  assert.equal(h.streams[0].stopped, 1)
  assert.equal(h.voice.state.value, 'idle')
  assert.equal(h.sends.length, 0)
  h.voice.dispose()
  h.dom.window.close()
})

test('a stale conversation never receives the recording', async () => withClock(async ({ advance }) => {
  MockRecorder.instances = []
  let current = true
  const h = makeHarness({ isCurrent: () => current })
  enable(h)
  key(h.dom, 'keydown')
  await flush()
  advance(350)
  current = false
  key(h.dom, 'keyup')
  await flush()
  assert.equal(h.sends.length, 0)
  assert.equal(h.streams[0].stopped, 1)
  h.voice.dispose()
  h.dom.window.close()
}))
