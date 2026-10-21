// The composer lets images sit inside the text at the caret position, so one
// message can read "text, image, text, image" exactly as it was typed.
//
// An image is represented in the draft by a token such as 〔图1〕. A token moves with
// the surrounding text while the user edits, which is why the draft stays a plain
// string: keeping character offsets instead would drift on every edit before the
// image. Sending parses the tokens back into ordered blocks.
//
// Kept free of DOM access so the rules can be unit tested.

// TOKEN_PATTERN matches a token and captures the image number.
const TOKEN_PATTERN = /〔图(\d+)〕/g

// tokenFor renders the placeholder a given image number uses.
export function tokenFor(index) {
  return `〔图${index}〕`
}

// nextImageIndex returns the smallest number not yet used, so a deleted image
// leaves its number free for the next one.
export function nextImageIndex(images) {
  const used = new Set((images || []).map(image => image.index))
  let index = 1
  while (used.has(index)) index++
  return index
}

// insertToken puts an image token at the caret and returns the new caret position,
// which lands just after the token so typing continues after the image.
export function insertToken(text, caret, index) {
  const source = String(text ?? '')
  const position = Math.max(0, Math.min(Number(caret) || 0, source.length))
  const token = tokenFor(index)
  // A token needs a line of its own to read as a separate segment, so a line break
  // is added when the caret is in the middle of text.
  const before = source.slice(0, position)
  const after = source.slice(position)
  const prefix = before && !before.endsWith('\n') ? '\n' : ''
  const suffix = after && !after.startsWith('\n') ? '\n' : ''
  const inserted = prefix + token + suffix
  return { text: before + inserted + after, caret: position + inserted.length }
}

// removeToken deletes one image's token from the draft.
export function removeToken(text, index) {
  return String(text ?? '').replace(tokenFor(index), '')
}

// tokensIn lists the image numbers referenced by the draft, in the order they
// appear. Repeats are ignored: one upload appears once in the message.
export function tokensIn(text) {
  const found = []
  for (const match of String(text ?? '').matchAll(TOKEN_PATTERN)) {
    const index = Number(match[1])
    if (!found.includes(index)) found.push(index)
  }
  return found
}

// buildBlocks turns the draft plus the uploaded images into the ordered block list
// the server stores.
//
// Text between tokens becomes a text block, each token becomes an image block, and
// empty text segments are dropped so a message never carries blank blocks. A token
// whose image is unknown is treated as literal text, because silently deleting what
// the user typed would be worse than showing it.
export function buildBlocks(text, images) {
  const source = String(text ?? '')
  const byIndex = new Map((images || []).map(image => [image.index, image]))
  const blocks = []
  let cursor = 0

  const pushText = value => {
    // Surrounding whitespace is trimmed but inner line breaks are preserved.
    const trimmed = String(value).replace(/^[\s]+|[\s]+$/g, '')
    if (trimmed) blocks.push({ type: 'text', text: trimmed })
  }

  for (const match of source.matchAll(TOKEN_PATTERN)) {
    const image = byIndex.get(Number(match[1]))
    if (!image) continue
    pushText(source.slice(cursor, match.index))
    blocks.push({ type: 'image', media_id: image.mediaId })
    cursor = match.index + match[0].length
  }
  pushText(source.slice(cursor))
  return blocks
}

// blocksToText renders a stored message back to the token form, used when a draft is
// restored after an uncertain send.
export function blocksToText(blocks, images) {
  const byMedia = new Map((images || []).map(image => [image.mediaId, image.index]))
  const parts = []
  for (const block of blocks || []) {
    if (block.type === 'text') {
      parts.push(block.text)
      continue
    }
    const index = byMedia.get(block.media_id)
    if (index) parts.push(tokenFor(index))
  }
  return parts.join('\n')
}

// countImages reports how many images a draft references, used to decide whether
// sending is allowed with no text at all.
export function countImages(blocks) {
  return (blocks || []).filter(block => block.type === 'image').length
}

// messageSegments returns what a message should render, in order.
//
// A message sent with blocks renders those blocks. An older message, or one sent as
// plain text or a single attachment, renders through its content and media so the
// existing markup keeps working. A recalled message renders nothing.
export function messageSegments(message) {
  if (message?.recalled_at) return []
  const blocks = message?.blocks
  if (Array.isArray(blocks) && blocks.length) {
    return blocks.map(block => block.type === 'image'
      ? { type: 'image', mediaId: block.media_id, fileName: block.file_name || '', byteSize: block.byte_size || 0 }
      : { type: 'text', text: block.text || '' })
  }
  // The single-attachment shape: the media, then any caption.
  const segments = []
  if (message?.media_id) {
    segments.push({
      type: message.kind === 'image' ? 'image' : message.kind === 'video' ? 'video' : 'file',
      mediaId: message.media_id,
      fileName: message.media?.file_name || '',
      byteSize: message.media?.byte_size || 0,
    })
  }
  if (message?.content) segments.push({ type: 'text', text: message.content })
  return segments
}

// hasBlocks reports whether a message uses the ordered shape, so the template can
// pick the block renderer instead of the legacy one.
export function hasBlocks(message) {
  return Array.isArray(message?.blocks) && message.blocks.length > 0 && !message?.recalled_at
}
