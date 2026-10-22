import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'
import { onImageLinkClick } from './image-viewer.js'

test('clicking a chat image opens an in-page preview instead of navigating', () => {
  const dom = new JSDOM('<div id="app"><a class="block-image" href="/api/v1/media/photo"><img alt="photo.png"></a></div>', { url: 'http://localhost/' })
  const originalDocument = globalThis.document
  const originalElement = globalThis.Element
  globalThis.document = dom.window.document
  globalThis.Element = dom.window.Element
  const dialog = dom.window.document.createElement('dialog')
  dialog.showModal = () => { dialog.open = true }
  dialog.close = () => {
    dialog.open = false
    dialog.dispatchEvent(new dom.window.Event('close'))
  }
  const originalCreateElement = dom.window.document.createElement.bind(dom.window.document)
  dom.window.document.createElement = tag => tag === 'dialog' ? dialog : originalCreateElement(tag)
  dom.window.document.getElementById('app').addEventListener('click', onImageLinkClick)

  try {
    const event = new dom.window.MouseEvent('click', { bubbles: true, cancelable: true })
    dom.window.document.querySelector('a').dispatchEvent(event)

    assert.equal(event.defaultPrevented, true)
    assert.equal(dom.window.document.querySelectorAll('dialog.image-viewer').length, 1)
    assert.equal(dialog.querySelector('img').src, 'http://localhost/api/v1/media/photo')
    assert.equal(dialog.querySelector('img').alt, 'photo.png')

    dialog.querySelector('button').click()
    assert.equal(dom.window.document.querySelector('dialog'), null)
  } finally {
    globalThis.document = originalDocument
    globalThis.Element = originalElement
    dom.window.close()
  }
})
