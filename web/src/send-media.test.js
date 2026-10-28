import test from 'node:test'
import assert from 'node:assert/strict'
import { register } from 'node:module'
import { JSDOM } from 'jsdom'

// Proves the composer sends images and text as ONE message, in the order they appear
// in the draft, and that an image is rendered as a real picture inside the draft.
register('../css-hook.mjs', import.meta.url)

const dom = new JSDOM('<!doctype html><html><body><div id="app"></div></body></html>', { url: 'http://127.0.0.1:4173' })
for (const key of ['window', 'document', 'location', 'HTMLElement', 'Element', 'Node', 'SVGElement', 'Text', 'Comment', 'DocumentFragment', 'Document', 'DOMParser', 'ShadowRoot', 'MutationObserver', 'File', 'Blob', 'Range']) {
  globalThis[key] = key === 'window' ? dom.window : key === 'document' ? dom.window.document : key === 'location' ? dom.window.location : dom.window[key]
}
globalThis.getComputedStyle = dom.window.getComputedStyle.bind(dom.window)
globalThis.localStorage = dom.window.localStorage
globalThis.sessionStorage = dom.window.sessionStorage
globalThis.WebSocket = class { constructor() { this.readyState = 0 } close() {} }
globalThis.URL.createObjectURL = () => 'blob:preview'
globalThis.URL.revokeObjectURL = () => {}

const me = { id: 'me', email: 'me@test', name: 'Me' }
const group = {
  id: 'conv-1', name: 'Team', kind: 'group', pinned: false, muted: false,
  announcement_count: 0, last_seq: 0, last_read_seq: 0,
  members: [{ id: 'me', name: 'Me', email: 'me@test', role: 'owner', online: true }],
}

let uploadCount = 0
const calls = []
globalThis.fetch = async (url, options = {}) => {
  const path = String(url)
  calls.push({ path, method: options.method || 'GET', body: options.body })
  let data = null
  if (path.includes('/api/v1/media/')) {
    return { ok: true, blob: async () => new Blob([new Uint8Array(8)], { type: 'image/png' }) }
  } else if (path.includes('/media')) {
    uploadCount++
    data = { id: 'media-' + uploadCount, file_name: 'pasted-' + uploadCount + '.png', media_type: 'image', content_type: 'image/png', byte_size: 8 }
  } else if (path.endsWith('/messages') && options.method === 'POST') {
    data = { id: 'msg-1', conversation_id: 'conv-1', seq: 1, kind: 'user' }
  } else if (path.includes('/me')) data = me
  else if (path.includes('/friends/requests')) data = { incoming: [], outgoing: [] }
  else if (path.includes('/conversations') && !path.includes('/messages')) data = [group]
  else if (path.includes('/announcements')) data = []
  else if (path.includes('/sync')) data = { messages: [], last_seq: 0, next_seq: 0, has_more: false }
  else if (path.includes('/friends') || path.includes('/users') || path.includes('/devices')) data = []
  return { ok: true, status: 200, json: async () => ({ data }) }
}

const timers = new Set()
const realSetInterval = globalThis.setInterval
const realSetTimeout = globalThis.setTimeout
globalThis.setInterval = (handler, delay, ...args) => { const handle = realSetInterval(handler, delay, ...args); timers.add(handle); return handle }
globalThis.setTimeout = (handler, delay, ...args) => { const handle = realSetTimeout(handler, delay, ...args); timers.add(handle); return handle }

await import('./main.js')
await new Promise(resolve => realSetTimeout(resolve, 800))

const vm = document.querySelector('#app').__vue_app__._instance.setupState
vm.active = 'conv-1'
await new Promise(resolve => realSetTimeout(resolve, 400))

const composer = () => document.querySelector('.rich-composer')

function paste(file) {
  const region = composer()
  assert.ok(region, 'the rich composer was not rendered')
  const event = new dom.window.Event('paste', { bubbles: true, cancelable: true })
  Object.defineProperty(event, 'clipboardData', {
    value: { items: [{ kind: 'file', type: file.type, getAsFile: () => file }], files: [file], types: ['Files'] },
    configurable: true,
  })
  region.dispatchEvent(event)
  return event
}

function pasteHTML(html) {
  const region = composer()
  const event = new dom.window.Event('paste', { bubbles: true, cancelable: true })
  Object.defineProperty(event, 'clipboardData', {
    value: { items: [], files: [], types: ['text/html'], getData: type => type === 'text/html' ? html : '' },
    configurable: true,
  })
  region.dispatchEvent(event)
  return event
}

// Typing appends a text node and fires input, which is what the browser does.
function type(text) {
  const region = composer()
  region.appendChild(document.createTextNode(text))
  region.dispatchEvent(new dom.window.Event('input', { bubbles: true }))
}

// Placing the caret at the end is what a user does before typing again.
function caretToEnd() {
  const region = composer()
  const range = document.createRange()
  range.selectNodeContents(region)
  range.collapse(false)
  const selection = dom.window.getSelection()
  selection.removeAllRanges()
  selection.addRange(range)
}

function reset() {
  calls.length = 0
  const region = composer()
  region.textContent = ''
  vm.draft = ''
  vm.clearComposerImages()
}

const posted = () => calls.filter(call => call.method === 'POST' && call.path.endsWith('/messages'))

function mockScreenSharing() {
  const originalCreate = document.createElement.bind(document)
  const originalNavigator = Object.getOwnPropertyDescriptor(globalThis, 'navigator')
  const originalRect = dom.window.HTMLImageElement.prototype.getBoundingClientRect
  let stopped = 0
  Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { mediaDevices: { async getDisplayMedia() { return { getTracks: () => [{ stop() { stopped++ } }] } } } } })
  document.createElement = tag => {
    const element = originalCreate(tag)
    if (tag === 'canvas') {
      element.getContext = () => ({ drawImage() {} })
      element.toBlob = callback => callback(new Blob(['png'], { type: 'image/png' }))
    } else if (tag === 'video') {
      Object.defineProperties(element, { readyState: { value: 2 }, videoWidth: { value: 800 }, videoHeight: { value: 600 } })
      element.play = async () => {}
      element.pause = () => {}
    }
    return element
  }
  dom.window.HTMLImageElement.prototype.getBoundingClientRect = () => ({ left: 0, top: 0, right: 800, bottom: 600, width: 800, height: 600 })
  return { stopped: () => stopped, restore() {
    document.createElement = originalCreate
    dom.window.HTMLImageElement.prototype.getBoundingClientRect = originalRect
    if (originalNavigator) Object.defineProperty(globalThis, 'navigator', originalNavigator)
    else delete globalThis.navigator
  } }
}

test('Ctrl+Q inserts a cropped screenshot at the saved caret and sends only after confirmation', async () => {
  reset()
  const screen = mockScreenSharing()
  try {
    type('前后')
    const range = document.createRange()
    range.setStart(composer().firstChild, 1)
    range.collapse(true)
    window.getSelection().removeAllRanges()
    window.getSelection().addRange(range)
    const event = new dom.window.KeyboardEvent('keydown', { key: 'q', ctrlKey: true, bubbles: true, cancelable: true })
    composer().dispatchEvent(event)
    assert.equal(event.defaultPrevented, true)
    await new Promise(resolve => realSetTimeout(resolve, 50))
    const image = document.querySelector('.screenshot-preview img')
    assert.ok(image)
    assert.equal(screen.stopped(), 1)
    assert.equal(calls.some(call => call.path.includes('/media?')), false)
    image.dispatchEvent(new dom.window.MouseEvent('pointerdown', { clientX: 20, clientY: 30, button: 0 }))
    image.dispatchEvent(new dom.window.MouseEvent('pointerup', { clientX: 220, clientY: 130 }))
    document.querySelector('[data-screenshot-confirm]').click()
    await new Promise(resolve => realSetTimeout(resolve, 100))
    assert.equal(composer().querySelectorAll('.composer-image img').length, 1)
    assert.equal(posted().length, 0, 'capturing must not send a message')
    assert.equal(document.querySelector('.screenshot-overlay'), null)
    await vm.send()
    const payload = JSON.parse(String(posted()[0].body))
    assert.deepEqual(payload.blocks.map(block => block.type), ['text', 'image', 'text'])
    assert.equal(payload.blocks[0].text, '前')
    assert.equal(payload.blocks[2].text, '后')
  } finally { screen.restore() }
})

test('the screenshot toolbar can cancel without uploading or changing the draft', async () => {
  reset()
  const screen = mockScreenSharing()
  try {
    type('保留草稿')
    const button = document.querySelector('[aria-label="截图（Ctrl+Q）"]')
    assert.ok(button)
    button.click()
    await new Promise(resolve => realSetTimeout(resolve, 50))
    document.querySelector('[data-screenshot-cancel]').click()
    await new Promise(resolve => realSetTimeout(resolve, 50))
    assert.equal(composer().textContent, '保留草稿')
    assert.equal(vm.composerImages.length, 0)
    assert.equal(calls.some(call => call.path.includes('/media?')), false)
    assert.equal(posted().length, 0)
  } finally { screen.restore() }
})

test('Ctrl+Z restores the previous composer text', () => {
  reset()
  type('撤销测试')
  const event = new dom.window.KeyboardEvent('keydown', { key: 'z', ctrlKey: true, bubbles: true, cancelable: true })
  composer().dispatchEvent(event)
  assert.equal(event.defaultPrevented, true)
  assert.equal(vm.draft, '')
})

test('Ctrl+Z removes a pasted image from the composer', async () => {
  reset()
  paste(new File([new Uint8Array(8)], 'undo.png', { type: 'image/png' }))
  await new Promise(resolve => realSetTimeout(resolve, 500))
  assert.equal(vm.composerImages.length, 1)
  composer().dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'z', ctrlKey: true, bubbles: true, cancelable: true }))
  assert.equal(vm.composerImages.length, 0)
  assert.equal(composer().querySelector('.composer-image'), null)
})

test('undoing an image removal restores a sendable image and Shift+Ctrl+Z redoes it', async () => {
  reset()
  paste(new File([new Uint8Array(8)], 'restore.png', { type: 'image/png' }))
  await new Promise(resolve => realSetTimeout(resolve, 500))
  const mediaID = vm.composerImages[0].mediaId
  composer().querySelector('[data-remove-media]').click()
  assert.equal(vm.composerImages.length, 0)
  composer().dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'z', ctrlKey: true, bubbles: true, cancelable: true }))
  assert.equal(vm.composerImages[0].mediaId, mediaID)
  assert.equal(composer().querySelector('.composer-image img').getAttribute('src'), '/api/v1/media/' + mediaID)
  composer().dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Z', ctrlKey: true, shiftKey: true, bubbles: true, cancelable: true }))
  assert.equal(vm.composerImages.length, 0)
  composer().dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'z', ctrlKey: true, bubbles: true, cancelable: true }))
  await vm.send()
  const payload = JSON.parse(String(posted()[0].body))
  assert.deepEqual(payload.blocks, [{ type: 'image', media_id: mediaID }])
  composer().dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'z', ctrlKey: true, bubbles: true, cancelable: true }))
  assert.equal(composer().textContent, '')
  assert.equal(vm.composerImages.length, 0)
})

test('undoing rich paste restores the preceding text in a single step', async () => {
  reset()
  type('原文')
  caretToEnd()
  pasteHTML('<p>粘贴文字</p><img src="data:image/png;base64,aW1hZ2U=" alt="undo.png" />')
  await new Promise(resolve => realSetTimeout(resolve, 500))
  assert.equal(vm.composerImages.length, 1)
  composer().dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'z', ctrlKey: true, bubbles: true, cancelable: true }))
  assert.equal(vm.draft, '原文')
  assert.equal(vm.composerImages.length, 0)
  assert.equal(composer().textContent, '原文')
})

test('Chinese composition is undone as one edit rather than restoring partial input', () => {
  reset()
  composer().dispatchEvent(new dom.window.CompositionEvent('compositionstart', { bubbles: true }))
  type('zhong')
  composer().textContent = '中文'
  composer().dispatchEvent(new dom.window.Event('input', { bubbles: true }))
  composer().dispatchEvent(new dom.window.CompositionEvent('compositionend', { bubbles: true }))
  composer().dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'z', ctrlKey: true, bubbles: true, cancelable: true }))
  assert.equal(vm.draft, '')
  assert.equal(composer().textContent, '')
})

test('pasting an image shows it inside the draft without sending', async () => {
  reset()
  const event = paste(new File([new Uint8Array(8)], 'image.png', { type: 'image/png' }))
  await new Promise(resolve => realSetTimeout(resolve, 500))

  assert.equal(event.defaultPrevented, true, 'the paste must be intercepted')
  assert.ok(calls.some(call => call.path.includes('/media')), 'the image was not uploaded')
  assert.equal(posted().length, 0, 'pasting must not post a message')
  // The picture itself is in the draft: this is the behaviour the change is for.
  const picture = composer().querySelector('.composer-image img')
  assert.ok(picture, 'the draft does not show the pasted image')
  assert.equal(vm.composerImages.length, 1, 'the image is not held by the composer')
  // No placeholder text is left behind.
  assert.doesNotMatch(composer().textContent, /〔图/, 'a placeholder token is still shown')
})

test('IME confirmation and deletion after an image are left to the input method', async () => {
  reset()
  paste(new File([new Uint8Array(8)], 'ime.png', { type: 'image/png' }))
  await new Promise(resolve => realSetTimeout(resolve, 500))
  const selection = dom.window.getSelection()
  assert.equal(selection.anchorNode.nodeType, 3)
  assert.equal(selection.anchorNode, composer().querySelector('.composer-image').nextSibling)
  composer().dispatchEvent(new dom.window.CompositionEvent('compositionstart', { bubbles: true }))
  for (const key of ['Backspace', 'Enter']) {
    const event = new dom.window.KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
    composer().dispatchEvent(event)
    assert.equal(event.defaultPrevented, false)
  }
  composer().dispatchEvent(new dom.window.CompositionEvent('compositionend', { bubbles: true }))
  const legacy = new dom.window.KeyboardEvent('keydown', { key: 'Enter', keyCode: 229, bubbles: true, cancelable: true })
  composer().dispatchEvent(legacy)
  assert.equal(legacy.defaultPrevented, false)
  assert.equal(posted().length, 0)
  assert.equal(vm.composerImages.length, 1)
})

test('text, image, text, image leave as one ordered message', async () => {
  reset()

  // Explain, paste, explain again, paste: the composition from the request.
  type('第一张是登录页')
  caretToEnd()
  paste(new File([new Uint8Array(8)], 'one.png', { type: 'image/png' }))
  await new Promise(resolve => realSetTimeout(resolve, 450))

  type('\n第二张是报错信息')
  caretToEnd()
  paste(new File([new Uint8Array(8)], 'two.png', { type: 'image/png' }))
  await new Promise(resolve => realSetTimeout(resolve, 450))

  assert.equal(composer().querySelectorAll('.composer-image img').length, 2, 'both images should be visible in the draft')

  await vm.send()
  await new Promise(resolve => realSetTimeout(resolve, 400))

  const messages = posted()
  assert.equal(messages.length, 1, `expected one message, got ${messages.length}`)
  const payload = JSON.parse(String(messages[0].body))
  const blocks = payload.blocks || []
  const types = blocks.map(block => block.type)
  assert.deepEqual(types, ['text', 'image', 'text', 'image'], `unexpected block order: ${JSON.stringify(types)}`)
  assert.equal(blocks[0].text, '第一张是登录页')
  assert.equal(blocks[2].text, '第二张是报错信息')
  assert.ok(blocks[1].media_id && blocks[3].media_id, 'both images must be referenced')
  assert.notEqual(blocks[1].media_id, blocks[3].media_id, 'the two images must be distinct')
  // The composer is emptied once the message is sent.
  assert.equal(vm.composerImages.length, 0, 'the images were not released after sending')
  assert.equal(composer().textContent.trim(), '', 'the draft was not cleared after sending')
})

test('pasting copied rich text uploads embedded images and sends every segment in order', async () => {
  reset()
  const event = pasteHTML('<html><body><div>xx<img src="data:image/png;base64,aW1hZ2U=" alt="image.png" />duds<img src="data:image/png;base64,aW1hZ2U=" alt="image.png" /></div></body></html>')
  assert.equal(event.defaultPrevented, true, 'the rich image paste was not intercepted')
  await new Promise(resolve => realSetTimeout(resolve, 900))

  assert.equal(composer().querySelectorAll('.composer-image img').length, 2, 'the pasted images were not uploaded into the draft')
  assert.equal(composer().querySelectorAll('[data-rich-paste-image]').length, 0, 'an image placeholder remained in the draft')
  assert.match(composer().textContent, /xx.*duds/s, 'the surrounding text was not preserved')

  await vm.send()
  await new Promise(resolve => realSetTimeout(resolve, 400))
  const payload = JSON.parse(String(posted()[0].body))
  assert.deepEqual(payload.blocks.map(block => block.type), ['text', 'image', 'text', 'image'])
  assert.equal(payload.blocks[0].text, 'xx')
  assert.equal(payload.blocks[2].text, 'duds')
})

test('pasting selected chat content removes bubble styling and full-width image links', async () => {
  reset()
  const event = pasteHTML('<div class="message-row mine"><div class="bubble" style="background-color: rgb(183, 101, 76)"><div class="block-stack"><p class="block-text">前文</p><a class="block-image" href="/api/v1/media/photo"><img src="/api/v1/media/photo" alt="photo.png" /></a><p class="block-text">后文</p></div></div></div>')
  assert.equal(event.defaultPrevented, true, 'chat HTML should be handled by the composer')
  await new Promise(resolve => realSetTimeout(resolve, 900))

  assert.equal(composer().querySelector('.bubble, .message-row, .block-stack, .block-image'), null, 'chat styling or a full-width image link leaked into the draft')
  assert.equal(composer().querySelector('[style*="background"]'), null, 'the copied bubble background leaked into the draft')
  assert.ok(composer().querySelector('.composer-image img'), 'the selected image was not converted to a draft attachment')

  await vm.send()
  await new Promise(resolve => realSetTimeout(resolve, 400))
  const payload = JSON.parse(String(posted()[0].body))
  assert.deepEqual(payload.blocks.map(block => block.type), ['text', 'image', 'text'])
  assert.equal(payload.blocks[0].text, '前文')
  assert.equal(payload.blocks[2].text, '后文')
})

test('pasting a fully selected formatted message does not add leading blank lines', async () => {
  reset()
  const event = pasteHTML('<html><body>\n  <div class="message-row">\n    <div class="bubble">\n      <div class="block-stack">\n        <p class="block-text">完整消息</p>\n      </div>\n    </div>\n  </div>\n</body></html>')
  assert.equal(event.defaultPrevented, true)
  assert.equal(vm.draft, '完整消息')
})

test('an image alone is a valid message', async () => {
  reset()
  paste(new File([new Uint8Array(8)], 'solo.png', { type: 'image/png' }))
  await new Promise(resolve => realSetTimeout(resolve, 450))

  await vm.send()
  await new Promise(resolve => realSetTimeout(resolve, 400))
  const messages = posted()
  assert.equal(messages.length, 1)
  const payload = JSON.parse(String(messages[0].body))
  assert.equal(payload.blocks.length, 1)
  assert.equal(payload.blocks[0].type, 'image')
})

test('the remove control deletes the picture from the draft', async () => {
  reset()
  paste(new File([new Uint8Array(8)], 'gone.png', { type: 'image/png' }))
  await new Promise(resolve => realSetTimeout(resolve, 450))
  assert.equal(composer().querySelectorAll('.composer-image').length, 1)

  // Clicking the chip's remove control is how a user drops an image.
  const remove = composer().querySelector('[data-remove-media]')
  assert.ok(remove, 'the chip has no remove control')
  remove.dispatchEvent(new dom.window.MouseEvent('click', { bubbles: true, cancelable: true }))
  await new Promise(resolve => realSetTimeout(resolve, 200))

  assert.equal(composer().querySelectorAll('.composer-image').length, 0, 'the picture was not removed')
  assert.equal(vm.composerImages.length, 0, 'the composer still holds the image')
})

// Backspace is the deletion a user reaches for first, so it must delete the picture as
// one unit rather than editing around it or doing nothing visible.
function press(key, init = {}) {
  const event = new dom.window.KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...init })
  composer().dispatchEvent(event)
  return event
}

test('Backspace deletes a pasted picture instead of a character', async () => {
  reset()
  type('看这张')
  caretToEnd()
  paste(new File([new Uint8Array(8)], 'pasted.png', { type: 'image/png' }))
  await new Promise(resolve => realSetTimeout(resolve, 450))
  assert.equal(composer().querySelectorAll('.composer-image').length, 1)

  const event = press('Backspace')
  await new Promise(resolve => realSetTimeout(resolve, 150))

  assert.equal(event.defaultPrevented, true, 'Backspace was not handled by the composer')
  assert.equal(composer().querySelectorAll('.composer-image').length, 0, 'the picture is still in the draft')
  assert.equal(vm.composerImages.length, 0, 'the composer still holds the removed image')
  // The text that introduced the picture is untouched.
  assert.equal(vm.draft, '看这张')
})

test('Backspace in ordinary text is left to the browser', async () => {
  reset()
  type('普通文字')
  caretToEnd()
  const event = press('Backspace')
  await new Promise(resolve => realSetTimeout(resolve, 100))
  // Nothing was prevented: the browser performs the edit, as it does in any text field.
  assert.equal(event.defaultPrevented, false)
  assert.equal(vm.draft, '普通文字')
})

test('an image-only draft can be sent and then deleted with Backspace', async () => {
  reset()
  paste(new File([new Uint8Array(8)], 'solo.png', { type: 'image/png' }))
  await new Promise(resolve => realSetTimeout(resolve, 450))
  // A picture alone enables the send button, which the plain draft text cannot express.
  const button = document.querySelector('.composer .primary')
  assert.equal(button.disabled, false, 'a picture-only draft is not sendable')

  press('Backspace')
  await new Promise(resolve => realSetTimeout(resolve, 150))
  assert.equal(composer().querySelectorAll('.composer-image').length, 0)
  assert.equal(vm.composerImages.length, 0)
  assert.equal(document.querySelector('.composer .primary').disabled, true, 'an emptied draft is still sendable')
})

test.after(() => {
  for (const handle of timers) { clearInterval(handle); clearTimeout(handle) }
  globalThis.setInterval = realSetInterval
  globalThis.setTimeout = realSetTimeout
  dom.window.close()
})
