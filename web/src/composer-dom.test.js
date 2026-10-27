import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'
import {
  caretRange, collectPieces, hasContent, imageCount, insertImageAtCaret, insertTextAtCaret,
  MEDIA_ATTRIBUTE, piecesToBlocks, piecesToText, placeCaretAfter,
  removeImageFromComposer, renderInto, textBeforeCaret,
} from './composer-dom.js'

const dom = new JSDOM('<!doctype html><html><body><div id="root"></div></body></html>')
globalThis.window = dom.window
globalThis.document = dom.window.document
globalThis.Node = dom.window.Node

const root = () => {
  const element = document.createElement('div')
  element.contentEditable = 'true'
  // Each test starts from an empty body so a leftover element or selection from the
  // previous one cannot influence it.
  document.body.innerHTML = ''
  document.body.appendChild(element)
  return element
}
const image = (index, mediaId) => ({ index, mediaId, previewURL: 'blob:' + mediaId, fileName: mediaId + '.png' })

test('typed text and line breaks become pieces in order', () => {
  const element = root()
  element.textContent = '第一行'
  // The browser inserts a <br> or a new block on Enter; both must read as a newline.
  element.appendChild(document.createElement('br'))
  element.appendChild(document.createTextNode('第二行'))
  assert.deepEqual(collectPieces(element), [{ type: 'text', text: '第一行\n第二行' }])
})

test('text inserts at the caret and notifies the composer', () => {
  const element = root()
  element.textContent = 'AB'
  const range = document.createRange()
  range.setStart(element.firstChild, 1)
  range.collapse(true)
  const selection = window.getSelection()
  selection.removeAllRanges()
  selection.addRange(range)
  let inputEvents = 0
  element.addEventListener('input', () => inputEvents++)

  insertTextAtCaret(element, '😊', selection)

  assert.equal(element.textContent, 'A😊B')
  assert.equal(inputEvents, 1)
  assert.equal(selection.anchorOffset, 2)
})

test('a div per line reads as separate lines', () => {
  const element = root()
  const first = document.createElement('div')
  first.textContent = '一行'
  const second = document.createElement('div')
  second.textContent = '二行'
  element.append(first, second)
  const pieces = collectPieces(element)
  assert.equal(piecesToText(pieces).replace(/\n+$/, ''), '一行\n二行')
})

test('an image chip sits between the text around it', () => {
  const element = root()
  element.textContent = '看这张'
  const range = document.createRange()
  range.selectNodeContents(element)
  range.collapse(false)
  insertImageAtCaret(element, range, image(1, 'media-a'))
  element.appendChild(document.createTextNode('就是这样'))

  const pieces = collectPieces(element)
  const blocks = piecesToBlocks(pieces)
  assert.deepEqual(blocks, [
    { type: 'text', text: '看这张' },
    { type: 'image', media_id: 'media-a' },
    { type: 'text', text: '就是这样' },
  ])
})

test('several images keep the order they were inserted in', () => {
  const element = root()
  const range = document.createRange()
  range.selectNodeContents(element)
  range.collapse(false)

  insertImageAtCaret(element, range, image(1, 'media-a'))
  element.appendChild(document.createTextNode('说明一'))
  const second = document.createRange()
  second.selectNodeContents(element)
  second.collapse(false)
  insertImageAtCaret(element, second, image(2, 'media-b'))
  element.appendChild(document.createTextNode('说明二'))

  assert.deepEqual(piecesToBlocks(collectPieces(element)), [
    { type: 'image', media_id: 'media-a' },
    { type: 'text', text: '说明一' },
    { type: 'image', media_id: 'media-b' },
    { type: 'text', text: '说明二' },
  ])
  assert.equal(imageCount(collectPieces(element)), 2)
})

test('an image chip carries its upload id and is not editable', () => {
  const element = root()
  const range = document.createRange()
  range.selectNodeContents(element)
  range.collapse(false)
  const chip = insertImageAtCaret(element, range, image(1, 'media-a'))

  assert.equal(chip.getAttribute(MEDIA_ATTRIBUTE), 'media-a')
  assert.equal(chip.getAttribute('contenteditable'), 'false')
  // The picture itself is rendered, which is the point of the change.
  assert.ok(chip.querySelector('img'), 'the chip must contain an image')
  // The remove control is chrome and must not become message text.
  assert.ok(chip.querySelector('[data-composer-chrome]'))
})

test('an image without an upload id is ignored rather than sent', () => {
  const element = root()
  const stray = document.createElement('img')
  element.appendChild(stray)
  element.appendChild(document.createTextNode('文字'))
  // A stray picture with no upload would produce a block the server rejects.
  assert.deepEqual(piecesToBlocks(collectPieces(element)), [{ type: 'text', text: '文字' }])
})

test('removing an image deletes its chip and the spacer after it', () => {
  const element = root()
  const range = document.createRange()
  range.selectNodeContents(element)
  range.collapse(false)
  insertImageAtCaret(element, range, image(1, 'media-a'))
  element.appendChild(document.createTextNode('后面'))

  assert.equal(removeImageFromComposer(element, 'media-a'), true)
  assert.equal(element.querySelector('[' + MEDIA_ATTRIBUTE + ']'), null)
  // No leading spacer is left behind.
  assert.equal(piecesToText(collectPieces(element)), '后面')
  // Removing again reports that nothing was there.
  assert.equal(removeImageFromComposer(element, 'media-a'), false)
})

test('blank drafts are recognised as empty and whitespace is trimmed', () => {
  const element = root()
  element.textContent = '   '
  assert.equal(hasContent(collectPieces(element)), false)
  assert.deepEqual(piecesToBlocks(collectPieces(element)), [])

  element.textContent = ''
  assert.equal(hasContent(collectPieces(element)), false)
  // An image alone is content.
  const range = document.createRange()
  range.selectNodeContents(element)
  range.collapse(false)
  insertImageAtCaret(element, range, image(1, 'media-a'))
  assert.equal(hasContent(collectPieces(element)), true)
})

test('inner line breaks survive inside one text segment', () => {
  const element = root()
  element.textContent = '第一行'
  element.appendChild(document.createElement('br'))
  element.appendChild(document.createTextNode('第二行'))
  assert.deepEqual(piecesToBlocks(collectPieces(element)), [{ type: 'text', text: '第一行\n第二行' }])
})

test('the caret is measured as text before it, for the mention lookup', () => {
  const element = root()
  element.textContent = '你好 @ali'
  const selection = dom.window.getSelection()
  const range = document.createRange()
  // The text is seven characters long, so the caret sits at its end.
  range.setStart(element.firstChild, element.firstChild.length)
  range.collapse(true)
  selection.removeAllRanges()
  selection.addRange(range)

  assert.equal(textBeforeCaret(element, selection), '你好 @ali')
  // A selection that sits in a different element contributes nothing.
  const other = document.createElement('div')
  other.textContent = 'x'
  document.body.appendChild(other)
  assert.equal(textBeforeCaret(other, selection), '')
  other.remove()
})

test('the caret falls back to the end when the selection is elsewhere', () => {
  const element = root()
  element.textContent = 'abc'
  const selection = dom.window.getSelection()
  selection.removeAllRanges()
  const range = caretRange(element, selection)
  assert.equal(range.collapsed, true)
})

test('a restored draft renders text and images again', () => {
  const element = root()
  renderInto(element, [
    { type: 'text', text: '第一段' },
    { type: 'image', media_id: 'media-a' },
    { type: 'text', text: '第二段' },
  ], [image(1, 'media-a')])
  assert.equal(element.querySelectorAll('img').length, 1)
  assert.deepEqual(piecesToBlocks(collectPieces(element)), [
    { type: 'text', text: '第一段' },
    { type: 'image', media_id: 'media-a' },
    { type: 'text', text: '第二段' },
  ])
})

test('placing the caret after a node lets typing continue past the image', () => {
  const element = root()
  const range = document.createRange()
  range.selectNodeContents(element)
  range.collapse(false)
  const chip = insertImageAtCaret(element, range, image(1, 'media-a'))
  const selection = dom.window.getSelection()
  placeCaretAfter(element, chip, selection)
  assert.equal(selection.rangeCount, 1)
  assert.equal(selection.anchorNode, chip.nextSibling)
  assert.equal(selection.anchorNode.nodeType, 3)
  assert.equal(selection.anchorOffset, 1)
  const typing = selection.getRangeAt(0)
  typing.insertNode(document.createTextNode('你好'))
  assert.deepEqual(piecesToBlocks(collectPieces(element)), [
    { type: 'image', media_id: 'media-a' },
    { type: 'text', text: '你好' },
  ])
})
