// Recognises images pasted from the clipboard or dropped onto the chat, and
// prepares them so the upload produces an inline photo instead of a file.
//
// The server treats only a few declared types as inline images (jpeg, png, gif,
// webp); every other type becomes a downloadable file. A clipboard image can also
// arrive with no type at all. Both cases are normalised here so the user gets a
// photo in the conversation, which is the whole point of pasting one.
//
// Kept free of browser globals except File/Blob so the rules can be unit tested;
// the actual re-encoding is injected as a callback.

// The image types the server renders inline.
export const INLINE_IMAGE_TYPES = ['image/jpeg', 'image/png', 'image/gif', 'image/webp']

const EXTENSION_TYPES = {
  png: 'image/png',
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  gif: 'image/gif',
  webp: 'image/webp',
  bmp: 'image/bmp',
  avif: 'image/avif',
  heic: 'image/heic',
  heif: 'image/heif',
  tif: 'image/tiff',
  tiff: 'image/tiff',
}

const TYPE_EXTENSIONS = {
  'image/png': 'png',
  'image/jpeg': 'jpg',
  'image/gif': 'gif',
  'image/webp': 'webp',
}

// normalizeType lowercases a declared type and drops any parameters.
export function normalizeType(value) {
  return String(value || '').toLowerCase().split(';')[0].trim()
}

export function isInlineImageType(value) {
  return INLINE_IMAGE_TYPES.includes(normalizeType(value))
}

// inferTypeFromName covers a clipboard file that carries no type but a usable name.
export function inferTypeFromName(name) {
  const match = String(name || '').toLowerCase().match(/\.([a-z0-9]+)$/)
  return match ? EXTENSION_TYPES[match[1]] || '' : ''
}

// describeImageType always returns a type for an image, preferring the declared
// one and falling back to the name and finally to PNG, which is what a pasted
// screenshot almost always is.
export function describeImageType(file) {
  const declared = normalizeType(file?.type)
  if (declared) return declared
  return inferTypeFromName(file?.name) || 'image/png'
}

// isImageFile decides whether a dropped or pasted entry deserves handling. The
// declared type wins; a name with a known image extension also counts, because
// some platforms paste files with an empty type.
export function isImageFile(file) {
  if (!file) return false
  const declared = normalizeType(file.type)
  if (declared.startsWith('image/')) return true
  if (declared) return false
  return Boolean(inferTypeFromName(file.name))
}

// collectImages reads a ClipboardData or DataTransfer. Items are preferred because
// they expose the pasted file before the browser inserts anything; files are the
// fallback for drag and drop.
export function collectImages(transfer) {
  const found = []
  const items = transfer?.items
  if (items && items.length) {
    for (const item of Array.from(items)) {
      if (item?.kind !== 'file') continue
      const file = typeof item.getAsFile === 'function' ? item.getAsFile() : null
      if (file && isImageFile(file)) found.push(file)
    }
  }
  if (!found.length) {
    const files = transfer?.files
    if (files && files.length) {
      for (const file of Array.from(files)) {
        if (isImageFile(file)) found.push(file)
      }
    }
  }
  return found
}

// hasTransferableFiles tells the caller whether to intercept the event at all.
export function hasTransferableFiles(transfer) {
  const types = transfer?.types
  if (!types) return false
  return Array.from(types).some(type => type === 'Files' || String(type).startsWith('image/'))
}

// pastedName gives a clipboard image a stable, readable file name. A pasted
// screenshot often has no name, and the server uses the name for downloads.
export function pastedName(type, now = Date.now()) {
  const extension = TYPE_EXTENSIONS[normalizeType(type)] || 'png'
  return `pasted-${now}.${extension}`
}

// prepareImage returns a File the server will store as an inline image.
//
// - A supported type is rebuilt only when the declared type or name is missing, so
//   the upload header always matches the bytes' container extension.
// - An unsupported type (bmp, avif, heic, tiff) is re-encoded to PNG through the
//   injected converter, because the server would otherwise store it as a file.
// - When conversion is unavailable the original file is returned, so the message
//   still sends as a downloadable file rather than failing.
export async function prepareImage(file, convert) {
  const type = describeImageType(file)
  if (isInlineImageType(type)) {
    return rebuild(file, type)
  }
  if (typeof convert === 'function') {
    try {
      const blob = await convert(file)
      if (blob) {
        const name = String(file?.name || '').replace(/\.[^.]+$/, '') || `image-${Date.now()}`
        return new File([blob], `${name}.png`, { type: 'image/png', lastModified: Date.now() })
      }
    } catch { /* fall through and send the original bytes */ }
  }
  return file
}

// rebuild guarantees the File carries the corrected type and a matching name.
function rebuild(file, type) {
  const declared = normalizeType(file?.type)
  const namedExtension = inferTypeFromName(file?.name)
  if (declared === type && namedExtension === type && file?.name) return file
  const base = String(file?.name || '').replace(/\.[^.]+$/, '') || 'image'
  const extension = TYPE_EXTENSIONS[type] || 'png'
  return new File([file], `${base}.${extension}`, { type, lastModified: file?.lastModified || Date.now() })
}
