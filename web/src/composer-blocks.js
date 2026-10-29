// Helpers shared by the composer and the message renderer.
//
// The composer itself is a rich region whose DOM is the source of truth, so the
// token-based draft parsing that used to live here is gone; what remains is the
// numbering of images and the mapping of a stored message back to renderable
// segments.

// nextImageIndex returns the smallest number not yet used, so a removed image leaves
// its number free for the next one. The number is only a label now that the draft
// shows pictures rather than placeholders.
export function nextImageIndex(images) {
  const used = new Set((images || []).map(image => image.index))
  let index = 1
  while (used.has(index)) index++
  return index
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
      type: message.kind === 'image' ? 'image' : message.kind === 'video' ? 'video' : message.kind === 'audio' ? 'audio' : 'file',
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
