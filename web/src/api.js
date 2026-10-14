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
