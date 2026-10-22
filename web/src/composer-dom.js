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
    // The chip wrapping an image carries the upload id, so it is matched before its
    // children are walked. Matching only <img> would miss the id, which lives on the
    // wrapper so the picture and its remove control behave as one unit.
    const chipID = node.getAttribute && node.getAttribute(MEDIA_ATTRIBUTE)
    if (chipID) {
      flushText()
      pieces.push({ type: 'image', mediaId: chipID })
      return
    }
    // An <img> that is not inside a chip has no upload, so it cannot be sent and is
    // ignored rather than producing a block the server would reject.
    if (node.tagName === 'IMG') return
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
    // The non-breaking space the image insert adds, and any ordinary space around a
    // segment, is not part of what the user meant to say. Leading and trailing
    // whitespace is trimmed while inner line breaks survive.
    const trimmed = pending.replace(/[\s\u00A0]+$/, '').replace(/^[\s\u00A0]+/, '')
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
//
// The walk is the one the draft uses, so the remove control inside an image chip does
// not leak its `×` into the measured text: reading `range.toString()` used to shift the
// mention offsets by one character per picture. An image reads as a line break, which
// ends a mention token just as a picture between "@" and the caret should.
export function textBeforeCaret(root, selection) {
  if (!root || !selection || !selection.rangeCount) return ''
  const range = selection.getRangeAt(0)
  if (!root.contains(range.endContainer)) return ''
  const measure = document.createRange()
  measure.selectNodeContents(root)
  measure.setEnd(range.endContainer, range.endOffset)
  const pieces = collectPieces(measure.cloneContents())
  return pieces.map(piece => (piece.type === 'image' ? '\n' : piece.text)).join('')
}

// replaceMentionToken replaces the "@..." token that ends at the caret with `text`, and
// returns the inserted node, or null when no such token is there.
//
// The draft is edited in place instead of being rewritten from its text. A
// contenteditable region's text says nothing about the pictures inside it, so rewriting
// the region from `piecesToText` used to delete every image in the draft silently.
export function replaceMentionToken(root, selection, typed, text) {
  if (!root || !selection || !selection.rangeCount) return null
  const caret = selection.getRangeAt(0)
  if (!caret.collapsed || !root.contains(caret.startContainer)) return null
  // `typed` is what the user wrote after the "@": the "@" itself is one more character.
  let remaining = [...String(typed || '')].length + 1
  let node = caret.startContainer
  let offset = caret.startOffset
  if (node.nodeType !== 3) {
    node = adjacentTextBefore(root, node)
    if (!node) return null
    offset = (node.nodeValue || '').length
  }
  let startNode = null
  let startOffset = 0
  for (;;) {
    const take = Math.min(offset, remaining)
    remaining -= take
    offset -= take
    if (remaining === 0) { startNode = node; startOffset = offset; break }
    // The token is contiguous text: a picture or a line break in the middle means the
    // walk overshot and the mention is no longer a single token.
    const previous = offset === 0 ? adjacentTextBefore(root, node) : node
    if (!previous) return null
    node = previous
    offset = (node.nodeValue || '').length
  }
  const range = document.createRange()
  range.setStart(startNode, startOffset)
  range.setEnd(caret.startContainer, caret.startOffset)
  range.deleteContents()
  const inserted = document.createTextNode(text)
  range.insertNode(inserted)
  return inserted
}

// adjacentTextBefore returns the text node that ends just before `node`, or null when a
// picture, an image or a line break comes first. Nothing but text nodes may sit between,
// which is what makes a mention token contiguous.
function adjacentTextBefore(root, node) {
  let cursor = node
  while (cursor && cursor !== root) {
    let sibling = cursor.previousSibling
    while (sibling) {
      if (sibling.nodeType === 3) return sibling
      if (sibling.nodeType === 1) {
        if (isChip(sibling) || sibling.tagName === 'BR' || sibling.tagName === 'IMG') return null
        const inner = lastTextIn(sibling)
        if (!inner) return null
        return inner
      }
      sibling = sibling.previousSibling
    }
    cursor = cursor.parentNode
  }
  return null
}

// lastTextIn returns the last text node inside a block element, or null when the element
// ends with something that is not text.
function lastTextIn(element) {
  for (let index = element.childNodes.length - 1; index >= 0; index--) {
    const child = element.childNodes[index]
    if (child.nodeType === 3) return child
    if (child.nodeType === 1 && !isChip(child) && child.tagName !== 'BR' && child.tagName !== 'IMG') {
      const inner = lastTextIn(child)
      if (inner) return inner
    }
    return null
  }
  return null
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

export function insertTextAtCaret(root, text, selection) {
  const range = caretRange(root, selection)
  const node = document.createTextNode(text)
  range.deleteContents()
  range.insertNode(node)
  range.setStartAfter(node)
  range.collapse(true)
  selection.removeAllRanges()
  selection.addRange(range)
  root.focus()
  root.dispatchEvent(new root.ownerDocument.defaultView.Event('input', { bubbles: true }))
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
  detachChip(chip)
  return true
}

// isChip reports whether a node is one of the pictures the composer holds.
function isChip(node) {
  return node.nodeType === 1 && typeof node.hasAttribute === 'function' && node.hasAttribute(MEDIA_ATTRIBUTE)
}

// detachChip removes a picture and the spacer the insert left after it. The spacer is
// invisible, so keeping it would make the next Backspace do nothing visible.
function detachChip(chip) {
  const next = chip.nextSibling
  if (next && next.nodeType === 3 && next.nodeValue === '\u00A0') next.remove()
  chip.remove()
}

// caretInGap puts the caret where a removed picture used to be, so typing continues at
// that point instead of jumping to the edge of the region.
function caretInGap(parent, previous, next, selection) {
  if (!selection || !parent) return
  const range = document.createRange()
  if (previous && previous.nodeType === 3) range.setStart(previous, (previous.nodeValue || '').length)
  else if (previous) range.setStartAfter(previous)
  else if (next && next.parentNode === parent) range.setStartBefore(next)
  else range.setStart(parent, 0)
  range.collapse(true)
  selection.removeAllRanges()
  selection.addRange(range)
}

// unitLength counts what Backspace and Delete remove one at a time: the characters of
// a text node, one line break, or one whole picture chip.
function unitLength(node) {
  if (node.nodeType === 3) return (node.nodeValue || '').length
  if (node.nodeType !== 1) return 0
  if (isChip(node) || node.tagName === 'BR') return 1
  let total = 0
  for (const child of node.childNodes) total += unitLength(child)
  return total
}

// caretUnitIndex turns a caret into the position it occupies in that unit sequence, so
// the node next to it can be identified without relying on pixel geometry.
function caretUnitIndex(root, container, offset) {
  let index = 0
  let found = -1
  const walk = node => {
    if (found >= 0) return
    if (node === container) {
      if (node.nodeType === 3) { found = index + Math.min(offset, (node.nodeValue || '').length); return }
      for (let i = 0; i < Math.min(offset, node.childNodes.length); i++) index += unitLength(node.childNodes[i])
      found = index
      return
    }
    if (node.nodeType === 3) { index += (node.nodeValue || '').length; return }
    if (node.nodeType !== 1) return
    if (isChip(node) || node.tagName === 'BR') { index += 1; return }
    for (const child of node.childNodes) walk(child)
  }
  walk(root)
  return found
}

// unitAt returns the single unit at a linear index.
function unitAt(root, target) {
  let index = 0
  let hit = null
  const walk = node => {
    if (hit || index > target) return
    if (node.nodeType === 3) {
      const text = node.nodeValue || ''
      if (target < index + text.length) hit = { type: 'text', node, char: text[target - index] }
      index += text.length
      return
    }
    if (node.nodeType !== 1) return
    if (isChip(node)) { if (index === target) hit = { type: 'chip', node }; index += 1; return }
    if (node.tagName === 'BR') { if (index === target) hit = { type: 'break', node }; index += 1; return }
    for (const child of node.childNodes) walk(child)
  }
  walk(root)
  return hit
}

// spacerFollowingPicture reports whether a text node is exactly the single spacer the
// insert left directly after a picture. Only that spacer is skipped, so spaces the user
// typed later still delete one character at a time.
function spacerFollowingPicture(node) {
  if (node.nodeValue !== '\u00A0') return null
  const previous = node.previousSibling
  return previous && isChip(previous) ? previous : null
}

// deleteImageNearCaret removes a whole picture when Backspace or Delete is pressed next
// to it, and returns the upload id that went away. It returns null when the keystroke
// is an ordinary edit the browser must perform.
export function deleteImageNearCaret(root, selection, direction = 'backward') {
  if (!root || !selection || !selection.rangeCount) return null
  const range = selection.getRangeAt(0)
  // A selection that spans a picture is left to the browser; the caller reconciles the
  // visible pictures with what is left in the region afterwards.
  if (!range.collapsed || !root.contains(range.startContainer)) return null
  const caret = caretUnitIndex(root, range.startContainer, range.startOffset)
  if (caret < 0) return null
  const limit = unitLength(root)
  let target = direction === 'forward' ? caret : caret - 1
  if (direction !== 'forward') {
    // The invisible spacer right after a picture must not swallow the keystroke.
    let unit = target >= 0 ? unitAt(root, target) : null
    while (unit && unit.type === 'text' && spacerFollowingPicture(unit.node)) {
      target -= 1
      unit = target >= 0 ? unitAt(root, target) : null
    }
  }
  if (target < 0 || target >= limit) return null
  const unit = unitAt(root, target)
  if (!unit || unit.type !== 'chip') return null
  const mediaId = unit.node.getAttribute(MEDIA_ATTRIBUTE)
  const parent = unit.node.parentNode
  const previous = unit.node.previousSibling
  const spacer = unit.node.nextSibling
  const next = spacer && spacer.nodeType === 3 && spacer.nodeValue === '\u00A0' ? spacer.nextSibling : spacer
  detachChip(unit.node)
  // An empty region keeps showing its placeholder, so the caret is simply put back at
  // the start without adding filler nodes the next walk would have to ignore.
  caretInGap(parent, previous, next, selection)
  return mediaId
}

// imageIDsIn lists the uploads the region currently shows, in document order. It is
// what keeps the component's picture list in step with a draft the browser edited
// (Backspace, Delete, cut) without going through the remove control.
export function imageIDsIn(root) {
  if (!root) return []
  const ids = []
  for (const chip of root.querySelectorAll(`[${MEDIA_ATTRIBUTE}]`)) {
    const id = chip.getAttribute(MEDIA_ATTRIBUTE)
    if (id) ids.push(id)
  }
  return ids
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
