import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { JSDOM } from 'jsdom'
import { installEmojiPicker } from './emoji-picker.js'

test('emoji picker stays hidden until opened and inserts an emoji at the saved caret', () => {
  const dom = new JSDOM('<form class="composer"><div contenteditable="true">AB</div><div><span>Send</span></div></form>')
  const style = dom.window.document.createElement('style')
  style.textContent = readFileSync(new URL('./style.css', import.meta.url), 'utf8')
  dom.window.document.head.append(style)
  const originalDocument = globalThis.document
  const originalWindow = globalThis.window
  globalThis.document = dom.window.document
  globalThis.window = dom.window
  const editor = dom.window.document.querySelector('[contenteditable]')
  const range = dom.window.document.createRange()
  range.setStart(editor.firstChild, 1)
  range.collapse(true)
  dom.window.getSelection().addRange(range)
  let inputEvents = 0
  editor.addEventListener('input', () => inputEvents++)

  try {
    installEmojiPicker(editor)
    const picker = dom.window.document.querySelector('.emoji-picker')
    const toggle = picker.querySelector('.emoji-picker-toggle')
    const grid = picker.querySelector('.emoji-picker-grid')
    assert.equal(toggle.getAttribute('aria-expanded'), 'false')
    assert.equal(dom.window.getComputedStyle(grid).display, 'none')
    const down = new dom.window.MouseEvent('pointerdown', { bubbles: true, cancelable: true })
    toggle.dispatchEvent(down)
    assert.equal(down.defaultPrevented, true)
    toggle.click()
    assert.equal(toggle.getAttribute('aria-expanded'), 'true')
    assert.equal(dom.window.getComputedStyle(grid).display, 'grid')
    toggle.click()
    assert.equal(toggle.getAttribute('aria-expanded'), 'false')
    assert.equal(dom.window.getComputedStyle(grid).display, 'none')
    toggle.click()
    picker.querySelector('.emoji-option').click()

    assert.equal(editor.textContent, 'A😀B')
    assert.equal(inputEvents, 1)
    assert.equal(picker.classList.contains('open'), false)
    assert.equal(toggle.getAttribute('aria-expanded'), 'false')
    assert.equal(dom.window.getComputedStyle(grid).display, 'none')
  } finally {
    globalThis.document = originalDocument
    globalThis.window = originalWindow
    dom.window.close()
  }
})
