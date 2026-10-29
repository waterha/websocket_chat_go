import test from 'node:test'
import assert from 'node:assert/strict'
import { register } from 'node:module'
import { JSDOM } from 'jsdom'

register('../css-hook.mjs', import.meta.url)
const { cropBounds, cropAvatar } = await import('./avatar-crop.js')

test('cropBounds centers landscape, portrait, and square images', () => {
  assert.deepEqual(cropBounds(1200, 800), { left: 200, top: 0, size: 800 })
  assert.deepEqual(cropBounds(800, 1200), { left: 0, top: 200, size: 800 })
  assert.deepEqual(cropBounds(800, 800), { left: 0, top: 0, size: 800 })
})

test('cropBounds zooms around the center and applies offsets in crop-size units', () => {
  assert.deepEqual(cropBounds(1200, 800, 2), { left: 400, top: 200, size: 400 })
  assert.deepEqual(cropBounds(800, 1200, 4), { left: 300, top: 500, size: 200 })
  assert.deepEqual(cropBounds(1200, 800, 2, .25, -.25), { left: 300, top: 300, size: 400 })
})

test('cropBounds clamps positive and negative offsets to all image edges', () => {
  for (const [width, height] of [[1200, 800], [800, 1200]]) {
    for (const zoom of [1, 2, 4]) {
      const size = Math.min(width, height) / zoom
      assert.deepEqual(cropBounds(width, height, zoom, 100, 100), { left: 0, top: 0, size })
      assert.deepEqual(cropBounds(width, height, zoom, -100, -100), {
        left: width - size, top: height - size, size,
      })
    }
  }
})

function setup(t, { width = 1200, height = 800 } = {}) {
  const dom = new JSDOM('<button id="previous">Choose avatar</button>', { url: 'http://localhost/' })
  const { window } = dom
  const images = []
  class MockImage {
    naturalWidth = width
    naturalHeight = height
    src = ''
    constructor() { images.push(this) }
  }
  const originals = new Map()
  for (const [key, value] of Object.entries({ document: window.document, Image: MockImage })) {
    originals.set(key, Object.getOwnPropertyDescriptor(globalThis, key))
    Object.defineProperty(globalThis, key, { value, writable: true, configurable: true })
  }
  t.after(() => {
    for (const [key, descriptor] of originals) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor)
      else delete globalThis[key]
    }
    window.close()
  })
  const createURL = t.mock.method(URL, 'createObjectURL', () => 'blob:avatar-source')
  const revokeURL = t.mock.method(URL, 'revokeObjectURL', () => {})
  const showModal = window.HTMLDialogElement.prototype.showModal = t.mock.fn(function () { this.open = true })
  const close = window.HTMLDialogElement.prototype.close = t.mock.fn(function () {
    this.open = false
    this.dispatchEvent(new window.Event('close'))
  })
  const context = { fillStyle: '', fillRect: t.mock.fn(), drawImage: t.mock.fn() }
  const getContext = t.mock.method(window.HTMLCanvasElement.prototype, 'getContext', () => context)
  const encodings = []
  const toBlob = t.mock.method(window.HTMLCanvasElement.prototype, 'toBlob', function (callback, type, quality) {
    encodings.push({ canvas: this, callback, type, quality })
  })
  const previous = window.document.querySelector('#previous')
  previous.focus()
  const file = new File(['original image bytes'], 'original.png', { type: 'image/png' })
  const dialog = () => window.document.querySelector('dialog.avatar-crop-dialog')
  const load = async () => {
    assert.equal(images.length, 1)
    assert.equal(images[0].src, 'blob:avatar-source')
    images[0].onload()
    await Promise.resolve()
    assert.ok(dialog()?.open)
    return dialog()
  }
  const assertClean = () => {
    assert.equal(dialog(), null)
    assert.equal(close.mock.callCount(), 1)
    assert.equal(revokeURL.mock.callCount(), 1)
    assert.deepEqual(revokeURL.mock.calls[0].arguments, ['blob:avatar-source'])
    assert.equal(window.document.activeElement, previous)
  }
  return { window, images, createURL, revokeURL, showModal, close, context, getContext, toBlob, encodings, file, dialog, load, assertClean }
}

test('cropAvatar only produces a 512px JPEG File after confirmation and encoding complete', async t => {
  const env = setup(t)
  let settled = false
  const pending = cropAvatar(env.file)
  pending.then(() => { settled = true })
  const dialog = await env.load()
  assert.deepEqual(env.createURL.mock.calls[0].arguments, [env.file])
  assert.equal(env.showModal.mock.callCount(), 1)
  assert.equal(env.getContext.mock.callCount(), 0)
  assert.equal(env.toBlob.mock.callCount(), 0)
  assert.equal(settled, false)

  const confirm = dialog.querySelector('[data-action="confirm"]')
  confirm.click()
  assert.equal(confirm.disabled, true)
  assert.equal(env.getContext.mock.callCount(), 1)
  assert.deepEqual(env.getContext.mock.calls[0].arguments, ['2d'])
  assert.equal(env.context.fillStyle, '#ffffff')
  assert.deepEqual(env.context.fillRect.mock.calls[0].arguments, [0, 0, 512, 512])
  assert.deepEqual(env.context.drawImage.mock.calls[0].arguments, [env.images[0], 200, 0, 800, 800, 0, 0, 512, 512])
  assert.equal(env.toBlob.mock.callCount(), 1)
  const encoding = env.encodings[0]
  assert.equal(encoding.canvas.width, 512)
  assert.equal(encoding.canvas.height, 512)
  assert.equal(encoding.type, 'image/jpeg')
  assert.equal(encoding.quality, .92)
  await Promise.resolve()
  assert.equal(settled, false)
  assert.equal(env.revokeURL.mock.callCount(), 0)

  const blob = new Blob(['encoded jpeg bytes'], { type: 'image/jpeg' })
  encoding.callback(blob)
  const result = await pending
  assert.ok(result instanceof File)
  assert.notEqual(result, env.file)
  assert.equal(result.name, 'avatar.jpg')
  assert.equal(result.type, 'image/jpeg')
  assert.equal(await result.text(), await blob.text())
  env.assertClean()
})

for (const action of ['cancel button', 'close button', 'dialog cancel', 'Escape']) {
  test(`cropAvatar returns null on ${action} without encoding`, async t => {
    const env = setup(t)
    const pending = cropAvatar(env.file)
    const dialog = await env.load()
    if (action === 'cancel button') dialog.querySelector('[data-action="cancel"]').click()
    else if (action === 'close button') dialog.querySelector('[aria-label="关闭裁剪"]').click()
    else {
      const event = action === 'Escape'
        ? new env.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
        : new env.window.Event('cancel', { cancelable: true })
      dialog.dispatchEvent(event)
      assert.equal(event.defaultPrevented, true)
    }
    assert.equal(await pending, null)
    assert.equal(env.getContext.mock.callCount(), 0)
    assert.equal(env.toBlob.mock.callCount(), 0)
    env.assertClean()
  })
}

test('cropAvatar returns null immediately for an already aborted signal', async t => {
  const env = setup(t)
  const controller = new AbortController()
  controller.abort()
  assert.equal(await cropAvatar(env.file, { signal: controller.signal }), null)
  assert.equal(env.images.length, 0)
  assert.equal(env.createURL.mock.callCount(), 0)
  assert.equal(env.revokeURL.mock.callCount(), 0)
  assert.equal(env.showModal.mock.callCount(), 0)
  assert.equal(env.toBlob.mock.callCount(), 0)
})

test('aborting image loading returns null and revokes the source URL', async t => {
  const env = setup(t)
  const controller = new AbortController()
  const pending = cropAvatar(env.file, { signal: controller.signal })
  controller.abort()
  assert.equal(await pending, null)
  assert.equal(env.images[0].src, '')
  assert.equal(env.showModal.mock.callCount(), 0)
  assert.equal(env.close.mock.callCount(), 0)
  assert.equal(env.toBlob.mock.callCount(), 0)
  assert.deepEqual(env.revokeURL.mock.calls.map(call => call.arguments), [['blob:avatar-source']])
})

for (const duringEncoding of [false, true]) {
  test(`aborting ${duringEncoding ? 'during JPEG encoding' : 'the open dialog'} returns null`, async t => {
    const env = setup(t)
    const controller = new AbortController()
    const pending = cropAvatar(env.file, { signal: controller.signal })
    const dialog = await env.load()
    if (duringEncoding) dialog.querySelector('[data-action="confirm"]').click()
    controller.abort()
    assert.equal(await pending, null)
    assert.equal(env.toBlob.mock.callCount(), duringEncoding ? 1 : 0)
    env.assertClean()
    if (duringEncoding) {
      env.encodings[0].callback(new Blob(['late jpeg'], { type: 'image/jpeg' }))
      env.assertClean()
    }
  })
}

test('range input changes the crop coordinates used by drawImage', async t => {
  const env = setup(t, { width: 800, height: 1200 })
  const pending = cropAvatar(env.file)
  const dialog = await env.load()
  const range = dialog.querySelector('input[type="range"]')
  const preview = dialog.querySelector('.avatar-crop-stage img')
  assert.equal(preview.style.width, '100%')
  assert.equal(preview.style.height, '150%')
  assert.equal(preview.style.top, '-25%')
  range.value = '2'
  range.dispatchEvent(new env.window.Event('input', { bubbles: true }))
  assert.equal(preview.style.width, '200%')
  assert.equal(preview.style.height, '300%')
  assert.equal(preview.style.left, '-50%')
  assert.equal(preview.style.top, '-100%')
  assert.equal(env.toBlob.mock.callCount(), 0)
  dialog.querySelector('[data-action="confirm"]').click()
  assert.deepEqual(env.context.drawImage.mock.calls[0].arguments, [env.images[0], 200, 400, 400, 400, 0, 0, 512, 512])
  env.encodings[0].callback(new Blob(['jpeg'], { type: 'image/jpeg' }))
  assert.ok(await pending instanceof File)
  env.assertClean()
})
