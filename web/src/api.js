export async function api(path, options = {}) {
  const response = await fetch(`/api/v1${path}`, {
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  const body = await response.json()
  if (!response.ok) {
    const error = new Error(body.error?.message || '请求失败')
    error.status = response.status
    throw error
  }
  return body.data
}

const MEDIA_ERRORS = {
  unsupported_media_type: '只支持图片或视频文件',
  media_too_large: '文件超出大小限制',
  invalid_upload: '文件上传不完整，请重试',
  not_found: '当前会话不可用，无法上传',
}

// Sends raw bytes to the media endpoint. The browser sets Content-Type from the
// file's type, which is exactly the header the server validates against.
export async function uploadMedia(conversationID, file) {
  const params = new URLSearchParams({ file_name: file.name || 'upload' })
  const response = await fetch(`/api/v1/conversations/${conversationID}/media?${params}`, {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': file.type },
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
