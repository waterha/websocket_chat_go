import { isDesktop } from './desktop.js'

// The CSRF token is issued in a readable cookie at login and echoed back in a
// header on every state-changing request. Safe methods do not need it.
function csrfToken() {
  const match = document.cookie.match(/(?:^|;\s*)csrf=([^;]*)/)
  return match ? decodeURIComponent(match[1]) : ''
}

function requestHeaders(headers = {}) {
  const token = csrfToken()
  return token ? { ...headers, 'X-CSRF-Token': token } : headers
}

export async function api(path, options = {}) {
  const response = await fetch(`/api/v1${path}`, {
    credentials: 'same-origin',
    ...options,
    headers: requestHeaders({ 'Content-Type': 'application/json', ...(options.headers || {}) }),
  })
  let body
  try {
    body = await response.json()
  } catch {
    throw new Error('服务器返回了非 JSON 响应，请检查前后端代理配置')
  }
  if (!response.ok) {
    const error = new Error(body.error?.message || '请求失败')
    error.status = response.status
    error.code = body.error?.code
    throw error
  }
  return body.data
}

export const MAX_AVATAR_BYTES = 2 * 1024 * 1024

export async function uploadAvatar(file) {
  if (!file || file.size <= 0 || file.size > MAX_AVATAR_BYTES) throw new Error('头像图片不能为空且不能超过 2 MB')
  const params = new URLSearchParams({ file_name: file.name || 'avatar.png' })
  const response = await fetch(`/api/v1/me/avatar?${params}`, {
    method: 'POST',
    credentials: 'same-origin',
    headers: requestHeaders({ 'Content-Type': file.type, ...(isDesktop() ? { 'X-Upload-Size': String(file.size) } : {}) }),
    body: file,
  })
  const body = await response.json().catch(() => ({}))
  if (!response.ok) {
    const error = new Error(body.error?.message || (response.status === 413 ? '头像图片不能超过 2 MB' : '头像上传失败，请重试'))
    error.status = response.status
    error.code = body.error?.code
    throw error
  }
  return body.data
}

export async function clearAvatar() {
  return api('/me/avatar', { method: 'DELETE' })
}

const MEDIA_ERRORS = {
  unsupported_media_type: '不支持该文件类型',
  media_too_large: '文件超出大小限制',
  invalid_upload: '文件上传不完整，请重试',
  not_found: '当前会话不可用，无法上传',
}

// Sends raw bytes with the file's declared type, falling back to a downloadable
// binary file when the browser cannot identify its MIME type.
export async function uploadMedia(conversationID, file, { signal } = {}) {
  const params = new URLSearchParams({ file_name: file.name || 'upload' })
  const response = await fetch(`/api/v1/conversations/${conversationID}/media?${params}`, {
    method: 'POST',
    credentials: 'same-origin',
    headers: requestHeaders({ 'Content-Type': file.type || 'application/octet-stream', ...(isDesktop() ? { 'X-Upload-Size': String(file.size) } : {}) }),
    signal,
    body: file,
  })
  const body = await response.json().catch(() => ({}))
  if (!response.ok) {
    const code = body.error?.code
    const error = new Error(MEDIA_ERRORS[code] || '上传失败，请重试')
    error.status = response.status
    throw error
  }
  return body.data
}
