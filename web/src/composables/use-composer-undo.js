import { mediaURL } from '../media.js'
import { collectPieces, piecesToText } from '../composer-dom.js'

// Owns rich-composer snapshots, caret restoration and IME-aware undo/redo.
export function useComposerUndo({ draftArea, composerImages, uploadBusy, draft, updateMentionState }) {
  let undoRoot = null
  let undoHistory = []
  let undoPosition = -1
  let composerComposing = false
  function composerSnapshot() {
    const root = draftArea.value
    const selection = window.getSelection()
    const offsets = node => {
      const path = []
      while (node && node !== root) {
        path.unshift(Array.prototype.indexOf.call(node.parentNode.childNodes, node))
        node = node.parentNode
      }
      return path
    }
    const range = selection?.rangeCount ? selection.getRangeAt(0) : null
    const caret = range && root.contains(range.startContainer) && root.contains(range.endContainer)
      ? { start: offsets(range.startContainer), startOffset: range.startOffset, end: offsets(range.endContainer), endOffset: range.endOffset } : null
    return { html: root.innerHTML, images: composerImages.value.map(image => ({ ...image })), caret }
  }
  function resetComposerUndo() {
    undoHistory = draftArea.value ? [composerSnapshot()] : []
    undoPosition = undoHistory.length - 1
    composerComposing = false
  }
  function recordComposerUndo() {
    const root = draftArea.value
    if (!root || composerComposing || uploadBusy.value || root.querySelector('[data-rich-paste-image]')) return
    const snapshot = composerSnapshot()
    if (undoHistory[undoPosition]?.html === snapshot.html) return
    undoHistory.splice(undoPosition + 1)
    undoHistory.push(snapshot)
    if (undoHistory.length > 100) undoHistory.shift()
    undoPosition = undoHistory.length - 1
  }
  function onComposerUndoKey(event) {
    if (!(event.ctrlKey || event.metaKey) || event.altKey || event.isComposing || event.key.toLowerCase() !== 'z') return
    event.preventDefault()
    const root = draftArea.value
    if (composerComposing || uploadBusy.value || root.querySelector('[data-rich-paste-image]')) return
    const position = undoPosition + (event.shiftKey ? 1 : -1)
    const snapshot = undoHistory[position]
    if (!snapshot) return
    undoPosition = position
    for (const image of composerImages.value) if (image.previewURL?.startsWith('blob:')) URL.revokeObjectURL(image.previewURL)
    composerImages.value = snapshot.images.map(image => ({ ...image, previewURL: mediaURL(image.mediaId) }))
    root.innerHTML = snapshot.html
    for (const image of composerImages.value) {
      const chip = [...root.querySelectorAll('[data-media-id]')].find(node => node.getAttribute('data-media-id') === image.mediaId)
      if (chip?.querySelector('img')) chip.querySelector('img').src = image.previewURL
    }
    snapshot.html = root.innerHTML
    draft.value = piecesToText(collectPieces(root))
    root.focus()
    const range = document.createRange()
    const caret = snapshot.caret
    const resolve = path => path.reduce((node, index) => node?.childNodes[index], root)
    if (caret && resolve(caret.start) && resolve(caret.end)) {
      range.setStart(resolve(caret.start), caret.startOffset)
      range.setEnd(resolve(caret.end), caret.endOffset)
    } else { range.selectNodeContents(root); range.collapse(false) }
    const selection = window.getSelection()
    selection.removeAllRanges()
    selection.addRange(range)
    updateMentionState()
  }
  function installComposerUndo(root) {
    if (undoRoot === root) return
    undoRoot = root
    resetComposerUndo()
    root.addEventListener('keydown', onComposerUndoKey)
    root.addEventListener('compositionstart', () => { composerComposing = true })
    root.addEventListener('compositionend', () => { composerComposing = false; recordComposerUndo() })
    root.addEventListener('beforeinput', () => {
      if (undoHistory[undoPosition]?.html === root.innerHTML) undoHistory[undoPosition].caret = composerSnapshot().caret
    })
  }

  return { resetComposerUndo, recordComposerUndo, installComposerUndo, isComposerComposing: () => composerComposing }
}
