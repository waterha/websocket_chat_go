// Helpers for image and video messages. Kept separate from the main component
// so the media type rules can be unit tested without a browser.

export const MAX_IMAGE_BYTES = 10 * 1024 * 1024
export const MAX_VIDEO_BYTES = 100 * 1024 * 1024

// The upload endpoint trusts the declared Content-Type as the media type; it
// does not sniff the bytes, so the client must send the file's real type.
export function mediaKind(contentType) {
  const type = String(contentType || '').toLowerCase().split(';')[0].trim()
  if (type.startsWith('image/')) return 'image'
  if (type.startsWith('video/')) return 'video'
  return ''
}

export function validateMediaFile(file) {
  const kind = mediaKind(file?.type)
  if (!kind) return { ok: false, error: '只支持图片或视频文件' }
  if (!file.size) return { ok: false, error: '文件为空，请重新选择' }
  const limit = kind === 'video' ? MAX_VIDEO_BYTES : MAX_IMAGE_BYTES
  if (file.size > limit) return { ok: false, error: kind === 'video' ? '视频最大 100 MB' : '图片最大 10 MB' }
  return { ok: true, kind }
}

export function formatBytes(value) {
  const size = Number(value) || 0
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}

export const mediaURL = id => `/api/v1/media/${encodeURIComponent(id)}`

// The sidebar shows a readable hint instead of an empty caption.
export function mediaPreview(message) {
  if (message?.kind === 'image') return '[图片]'
  if (message?.kind === 'video') return '[视频]'
  return ''
}
