// Helpers for image and video messages. Kept separate from the main component
// so the media type rules can be unit tested without a browser.

export const MAX_IMAGE_BYTES = 10 * 1024 * 1024
export const MAX_VIDEO_BYTES = 100 * 1024 * 1024
export const MAX_FILE_BYTES = 50 * 1024 * 1024
export const MAX_AUDIO_BYTES = 10 * 1024 * 1024

// The upload endpoint trusts the declared Content-Type as the message kind; it
// does not sniff the bytes, so the client must send the file's real type. Any
// other type becomes a downloadable file instead of inline media.
export function mediaKind(contentType) {
  const type = String(contentType || '').toLowerCase().split(';')[0].trim()
  if (type.startsWith('image/')) return 'image'
  if (type.startsWith('video/')) return 'video'
  if (type.startsWith('audio/')) return 'audio'
  return 'file'
}

export function validateMediaFile(file) {
  if (!file) return { ok: false, error: '请选择文件' }
  const type = String(file.type || 'application/octet-stream').toLowerCase().split(';')[0].trim()
  if (type.split('/').length !== 2 || !type.split('/')[0] || !type.split('/')[1]) {
    return { ok: false, error: '无法识别文件类型，请更换文件' }
  }
  if (!file.size) return { ok: false, error: '文件为空，请重新选择' }
  const kind = mediaKind(type)
  const limit = kind === 'video' ? MAX_VIDEO_BYTES : kind === 'image' ? MAX_IMAGE_BYTES : kind === 'audio' ? MAX_AUDIO_BYTES : MAX_FILE_BYTES
  if (file.size > limit) {
    return { ok: false, error: kind === 'video' ? '视频最大 100 MB' : kind === 'image' ? '图片最大 10 MB' : kind === 'audio' ? '语音最大 10 MB' : '文件最大 50 MB' }
  }
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
  if (message?.kind === 'audio') return '[语音]'
  if (message?.kind === 'file') return '[文件]'
  return ''
}
