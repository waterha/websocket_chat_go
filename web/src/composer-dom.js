// The composer is a rich text area: images sit inside it as real pictures, in the
// position they were pasted, so the draft reads exactly as the message will.
//
// The DOM is the source of truth while editing. Sending serialises it into the
// ordered blocks the server stores, and the plain text is derived from the same
// walk so the placeholder and mention logic keep working.
//
// Kept free of component state so the rules can be unit tested in jsdom.

// The attribute that marks an inline image and ties it to an upload.
export const MEDIA_ATTRIBUTE = 'data-media-id'

// Element tags that behave as a line of their own inside a contenteditable region.
const BLOCK_TAGS = new Set(['DIV', 'P', 'LI', 'BLOCKQUOTE'])

// collectPieces walks the composer in document order and returns its text and image
// pieces. Block elements and <br> contribute a line break, which is what the browser
// inserts when the user presses Enter.
export function collectPieces(root) {
  const pieces = []
  let buffer = ''
  const flushText = () => {
    if (buffer) {
      pieces.push({ type: 'text', text: buffer })
      buffer = ''
    }
  }
  const walk = node => {
    // Text nodes carry the typed characters.
    if (node.nodeType === 3) {
      buffer += node.nodeValue || ''
      return
    }
    if (node.nodeType !== 1) return
    if (node.tagName === 'IMG') {
      const mediaId = node.getAttribute(MEDIA_ATTRIBUTE) || ''
      // An image without an upload id cannot be sent, so it is ignored rather than
      // producing a block the server would reject.
      if (mediaId) {
        flushText()
        pieces.push({ type: 'image', mediaId })
      }
      return
    }
    if (node.tagName === 'BR') {
      buffer += '\n'
      return
    }
    // The remove button inside an image chip is chrome, not content.
    if (node.hasAttribute && node.hasAttribute('data-composer-chrome')) return
    const block = BLOCK_TAGS.has(node.tagName)
    for (const child of node.childNodes) walk(child)
    if (block && !buffer.endsWith('\n')) buffer += '\n'
  }
  for (const child of root.childNodes) walk(child)
  flushText()
  return pieces
}

// piecesToText renders the pieces as the plain text of the draft, which is what the
// placeholder, the send guard and the mention lookup read.
export function piecesToText(pieces) {
  return (pieces || [])
    .map(piece => (piece.type === 'image' ? '' : piece.text))
    .join('')
}

// piecesToBlocks turns the pieces into the ordered block list. Consecutive text is
// merged, surrounding whitespace is trimmed and empty segments are dropped so a
// message never carries blank blocks. Line breaks inside a segment are preserved.
export function piecesToBlocks(pieces) {
  const blocks = []
  let pending = ''
  const flushText = () => {
    const trimmed = pending.replace(/^[ \t\n\r]+|[ \t\n\r]+$/g, '')
    if (trimmed) blocks.push({ type: 'text', text: trimmed })
    pending = ''
  }
  for (const piece of pieces || []) {
    if (piece.type === 'image') {
      flushText()
      blocks.push({ type: 'image', media_id: piece.mediaId })
      continue
    }
    pending += piece.text
  }
  flushText()
  return blocks
}

// hasContent reports whether the composer holds anything worth sending.
export function hasContent(pieces) {
  return (pieces || []).some(piece => piece.type === 'image' || String(piece.text || '').trim() !== '')
}

// imageCount counts the images referenced by the pieces.
export function imageCount(pieces) {
  return (pieces || []).filter(piece => piece.type === 'image').length
}

// textBeforeCaret returns the text between the start of the composer and the caret,
// which the mention lookup needs because a contenteditable region has no
// selectionStart.
export function textBeforeCaret(root, selection) {
  if (!root || !selection || !selection.rangeCount) return ''
  const range = selection.getRangeAt(0)
  if (!root.contains(range.endContainer)) return ''
  const measure = document.createRange()
  measure.selectNodeContents(root)
  measure.setEnd(range.endContainer, range.endOffset)
  return measure.toString()
}

// caretRange returns the current range when it sits inside the composer, otherwise a
// range collapsed at the end so an image is still placed somewhere sensible.
export function caretRange(root, selection) {
  if (root && selection && selection.rangeCount) {
    const range = selection.getRangeAt(0)
    if (root.contains(range.startContainer)) return range
  }
  const fallback = document.createRange()
  fallback.selectNodeContents(root)
  fallback.collapse(false)
  return fallback
}

// insertImageAtCaret places an image chip at the caret and returns the chip element.
//
// The chip is non-editable so it behaves as one unit: Backspace removes it whole
// rather than editing around it. It carries the upload id and a remove control.
export function insertImageAtCaret(root, range, image) {
  const chip = document.createElement('span')
  chip.className = 'composer-image'
  chip.setAttribute('contenteditable', 'false')
  chip.setAttribute(MEDIA_ATTRIBUTE, image.mediaId)
  chip.setAttribute('data-index', String(image.index))
  chip.title = image.fileName || ''

  const picture = document.createElement('img')
  picture.src = image.previewURL
  picture.alt = image.fileName || '图片'
  chip.appendChild(picture)

  const remove = document.createElement('span')
  remove.className = 'composer-image-remove'
  remove.setAttribute('data-remove-media', image.mediaId)
  remove.setAttribute('data-composer-chrome', '')
  remove.setAttribute('role', 'button')
  remove.setAttribute('aria-label', '移除这张图片')
  remove.textContent = '×'
  chip.appendChild(remove)

  range.deleteContents()
  range.insertNode(chip)

  // A space after the chip lets typing continue without merging into the image.
  const space = document.createTextNode('\u00A0')
  chip.after(space)
  return chip
}

// removeImageFromComposer deletes the chip for one upload. The trailing space the
// insert added goes with it so removing an image does not leave gaps behind.
export function removeImageFromComposer(root, mediaId) {
  const chip = root.querySelector(`[${MEDIA_ATTRIBUTE}="${mediaId}"]`)
  if (!chip) return false
  const next = chip.nextSibling
  chip.remove()
  if (next && next.nodeType === 3 && next.nodeValue === '\u00A0') next.remove()
  return true
}

// placeCaretAfter moves the caret just past a node, so typing continues after the
// image that was just inserted.
export function placeCaretAfter(root, node, selection) {
  if (!node || !selection) return
  const range = document.createRange()
  range.setStartAfter(node)
  range.collapse(true)
  selection.removeAllRanges()
  selection.addRange(range)
}

// renderInto replaces the composer content with the given text and images. Used when
// a draft is restored after an uncertain send.
export function renderInto(root, blocks, images) {
  const byMedia = new Map((images || []).map(image => [image.mediaId, image]))
  root.textContent = ''
  for (const block of blocks || []) {
    if (block.type === 'image') {
      const image = byMedia.get(block.media_id)
      if (!image) continue
      const range = document.createRange()
      range.selectNodeContents(root)
      range.collapse(false)
      insertImageAtCaret(root, range, image)
      continue
    }
    root.appendChild(document.createTextNode(block.text || ''))
  }
}
