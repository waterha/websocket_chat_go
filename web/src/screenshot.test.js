import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'
import { captureScreenRegion, installScreenshot, selectionPixels } from './screenshot.js'

const rect = { left: 100, top: 80, right: 900, bottom: 530, width: 800, height: 450 }
const tick = () => new Promise(resolve => setImmediate(resolve))
const deferred = () => { let resolve; const promise = new Promise(done => { resolve = done }); return { promise, resolve } }

function environment() {
  const dom = new JSDOM('<!doctype html><button id="before">草稿</button>', { url: 'http://127.0.0.1:4173' })
  const documentRef = dom.window.document
  const create = documentRef.createElement.bind(documentRef)
  const canvases = []
  let stopped = 0
  let shared = 0
  const stream = { getTracks: () => [{ stop() { stopped++ } }] }
  const mediaDevices = { async getDisplayMedia(options) { shared++; assert.deepEqual(options, { video: true, audio: false }); return stream } }
  documentRef.createElement = tag => {
    const element = create(tag)
    if (tag === 'canvas') {
      element.draws = []
      element.getContext = () => ({ drawImage: (...args) => element.draws.push(args) })
      element.toBlob = callback => callback(new Blob(['pixels'], { type: 'image/png' }))
      canvases.push(element)
    }
    if (tag === 'video') {
      Object.defineProperties(element, { readyState: { value: 2 }, videoWidth: { value: 1600 }, videoHeight: { value: 900 } })
      element.play = async () => {}
      element.pause = () => {}
    }
    if (tag === 'img') {
      Object.defineProperties(element, {
        naturalWidth: { value: 1600 }, naturalHeight: { value: 900 },
        src: { set() { queueMicrotask(() => element.onload?.()) } },
      })
    }
    return element
  }
  dom.window.HTMLImageElement.prototype.getBoundingClientRect = () => rect
  const realCreateURL = URL.createObjectURL
  const realRevokeURL = URL.revokeObjectURL
  const revoked = []
  URL.createObjectURL = () => 'blob:screenshot-preview'
  URL.revokeObjectURL = value => revoked.push(value)
  documentRef.querySelector('#before').focus()
  const options = { documentRef, windowRef: dom.window, mediaDevices, bridge: null, now: () => 123 }
  return {
    dom, documentRef, options, canvases, revoked,
    stopped: () => stopped, shared: () => shared,
    cleanup() { URL.createObjectURL = realCreateURL; URL.revokeObjectURL = realRevokeURL; dom.window.close() },
    select(x1 = 200, y1 = 180, x2 = 500, y2 = 380) {
      const image = documentRef.querySelector('.screenshot-preview img')
      image.dispatchEvent(new dom.window.MouseEvent('pointerdown', { clientX: x1, clientY: y1, button: 0 }))
      image.dispatchEvent(new dom.window.MouseEvent('pointermove', { clientX: x2, clientY: y2 }))
      image.dispatchEvent(new dom.window.MouseEvent('pointerup', { clientX: x2, clientY: y2 }))
    },
  }
}

test('selection maps both drag directions to original pixels and clamps to screen edges', () => {
  const expected = { x: 200, y: 200, width: 600, height: 400 }
  assert.deepEqual(selectionPixels({ x1: 200, y1: 180, x2: 500, y2: 380 }, rect, 1600, 900), expected)
  assert.deepEqual(selectionPixels({ x1: 500, y1: 380, x2: 200, y2: 180 }, rect, 1600, 900), expected)
  assert.deepEqual(selectionPixels({ x1: -20, y1: 0, x2: 1000, y2: 1000 }, rect, 1600, 900), { x: 0, y: 0, width: 1600, height: 900 })
  assert.equal(selectionPixels({ x1: 200, y1: 100, x2: 201, y2: 101 }, rect, 1600, 900), null)
  assert.equal(selectionPixels({ x1: 0, y1: 0, x2: 100, y2: 100 }, { ...rect, width: 0 }, 1600, 900), null)
})

test('browser capture stops sharing before selection and crops to a PNG only on confirmation', async () => {
  const env = environment()
  try {
    const promise = captureScreenRegion(env.options)
    await tick()
    assert.equal(env.shared(), 1)
    assert.equal(env.stopped(), 1)
    assert.ok(env.documentRef.querySelector('[aria-label="选择截图区域"]'))
    const confirm = env.documentRef.querySelector('[data-screenshot-confirm]')
    assert.equal(confirm.disabled, true)
    env.select()
    assert.equal(confirm.disabled, false)
    confirm.click()
    const file = await promise
    assert.equal(file.name, 'screenshot-123.png')
    assert.equal(file.type, 'image/png')
    assert.deepEqual(env.canvases[1].draws[0].slice(1), [200, 200, 600, 400, 0, 0, 600, 400])
    assert.equal(env.documentRef.querySelector('.screenshot-overlay'), null)
    assert.equal(env.documentRef.activeElement.id, 'before')
    assert.deepEqual(env.revoked, ['blob:screenshot-preview'])
  } finally { env.cleanup() }
})

test('Escape cancels a selection without creating a crop and releases the preview', async () => {
  const env = environment()
  try {
    const promise = captureScreenRegion(env.options)
    await tick()
    env.documentRef.dispatchEvent(new env.dom.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    assert.equal(await promise, null)
    assert.equal(env.canvases.length, 1)
    assert.equal(env.stopped(), 1)
    assert.equal(env.documentRef.querySelector('.screenshot-overlay'), null)
    assert.equal(env.revoked.length, 1)
  } finally { env.cleanup() }
})

test('native desktop capture uses the Go bridge instead of browser sharing', async () => {
  const env = environment()
  try {
    let nativeCalls = 0
    const promise = captureScreenRegion({ ...env.options, bridge: { async CaptureScreenshot() { nativeCalls++; return 'data:image/png;base64,cGl4ZWxz' } } })
    await tick()
    env.select(500, 380, 200, 180)
    env.documentRef.dispatchEvent(new env.dom.window.KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }))
    assert.equal((await promise).type, 'image/png')
    assert.equal(nativeCalls, 1)
    assert.equal(env.shared(), 0)
  } finally { env.cleanup() }
})

test('a cancelled browser permission prompt creates no preview', async () => {
  const env = environment()
  try {
    const result = await captureScreenRegion({ ...env.options, mediaDevices: { async getDisplayMedia() { throw new DOMException('denied', 'NotAllowedError') } } })
    assert.equal(result, null)
    assert.equal(env.documentRef.querySelector('.screenshot-overlay'), null)
  } finally { env.cleanup() }
})

test('aborting while the screen picker is pending stops tracks when it resolves', async () => {
  const env = environment()
  try {
    const controller = new AbortController()
    const pending = deferred()
    let stopped = 0
    const promise = captureScreenRegion({ ...env.options, signal: controller.signal, mediaDevices: { getDisplayMedia: () => pending.promise } })
    controller.abort()
    pending.resolve({ getTracks: () => [{ stop() { stopped++ } }] })
    assert.equal(await promise, null)
    assert.equal(stopped, 1)
    assert.equal(env.documentRef.querySelector('.screenshot-overlay'), null)
  } finally { env.cleanup() }
})

test('aborting an open selection or resizing it closes the dialog', async () => {
  for (const action of ['abort', 'resize']) {
    const env = environment()
    try {
      const controller = new AbortController()
      const promise = captureScreenRegion({ ...env.options, signal: controller.signal })
      await tick()
      if (action === 'abort') controller.abort()
      else env.dom.window.dispatchEvent(new env.dom.window.Event('resize'))
      assert.equal(await promise, null)
      assert.equal(env.documentRef.querySelector('.screenshot-overlay'), null)
      assert.equal(env.stopped(), 1)
    } finally { env.cleanup() }
  }
})

test('unsupported browser and older desktop clients produce actionable errors', async () => {
  await assert.rejects(captureScreenRegion({ mediaDevices: {}, bridge: null, documentRef: { createElement: () => ({ getContext: () => ({}) }) } }), /不支持屏幕截图/)
  await assert.rejects(captureScreenRegion({ bridge: {}, documentRef: { createElement: () => ({ getContext: () => ({}) }) } }), /更新桌面客户端/)
})

function controllerEnvironment() {
  const dom = new JSDOM('<!doctype html><input>')
  let eligible = true
  let context = 1
  const captures = []
  const inserted = []
  const errors = []
  const result = deferred()
  const controller = installScreenshot({
    documentRef: dom.window.document,
    canCapture: () => eligible,
    getContext: () => context,
    isCurrent: value => value === context,
    capture: options => { captures.push(options); return result.promise },
    onCapture: (file, value) => inserted.push({ file, context: value }),
    onError: error => errors.push(error),
  })
  return { dom, controller, captures, inserted, errors, result, setEligible: value => { eligible = value }, switchContext() { context++ },
    key(init = {}) { const event = new dom.window.KeyboardEvent('keydown', { key: 'q', ctrlKey: true, bubbles: true, cancelable: true, ...init }); dom.window.document.querySelector('input').dispatchEvent(event); return event },
    cleanup() { controller.dispose(); dom.window.close() },
  }
}

test('Ctrl+Q starts one capture even inside an input and never sends a message', async () => {
  const env = controllerEnvironment()
  try {
    assert.equal(env.key().defaultPrevented, true)
    env.key({ repeat: true })
    env.key()
    assert.equal(env.captures.length, 1)
    const file = new File(['png'], 'screen.png', { type: 'image/png' })
    env.result.resolve(file)
    await tick()
    assert.deepEqual(env.inserted, [{ file, context: 1 }])
    assert.deepEqual(env.errors, [])
  } finally { env.cleanup() }
})

test('shortcut ignores other modifiers, composition, and chats that cannot accept a draft', async () => {
  const env = controllerEnvironment()
  try {
    for (const init of [{ ctrlKey: false }, { altKey: true }, { shiftKey: true }, { metaKey: true }, { isComposing: true }, { keyCode: 229 }, { key: 'w' }]) assert.equal(env.key(init).defaultPrevented, false)
    env.setEligible(false)
    assert.equal(env.key().defaultPrevented, false)
    assert.equal(env.captures.length, 0)
  } finally { env.cleanup() }
})

test('switching conversations or cancelling discards the captured file', async () => {
  for (const action of ['switch', 'cancel', 'dispose']) {
    const env = controllerEnvironment()
    try {
      env.key()
      if (action === 'switch') env.switchContext()
      else if (action === 'cancel') env.controller.cancel()
      else env.controller.dispose()
      env.result.resolve(new File(['png'], 'screen.png'))
      await tick()
      assert.equal(env.inserted.length, 0)
      if (action !== 'switch') assert.equal(env.captures[0].signal.aborted, true)
    } finally { env.cleanup() }
  }
})

test('screenshot failures report an error and dispose removes the shortcut listener', async () => {
  const dom = new JSDOM('<!doctype html><body></body>')
  const errors = []
  let calls = 0
  const controller = installScreenshot({ documentRef: dom.window.document, canCapture: () => true, getContext: () => 1, isCurrent: () => true, onCapture() {}, onError: error => errors.push(error), capture: async () => { calls++; throw new Error('capture failed') } })
  try {
    await controller.start()
    assert.equal(errors[0].message, 'capture failed')
    controller.dispose()
    dom.window.document.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'q', ctrlKey: true, bubbles: true }))
    await tick()
    assert.equal(calls, 1)
  } finally { controller.dispose(); dom.window.close() }
})
