// Browser and desktop share the chat logic. Only this module knows about Wails.
let config = null
const native = () => globalThis.window?.go?.main?.App
export const isDesktop = () => Boolean(native())

export async function initializeDesktop() {
  if (!isDesktop()) return
  config = await native().Config()
  document.documentElement.classList.add('desktop-client')
}

export const desktopConfig = () => config
export const checkServer = value => native().CheckServer(value)
export const saveServer = value => native().SaveConfig(value)

// A stable device and uncertain sends survive app restarts. Server namespaces
// prevent switching servers from mixing pending IDs, users or read positions.
export function chatStorage(persistent = false) {
  if (!isDesktop()) return persistent ? localStorage : sessionStorage
  const prefix = `im.desktop.${config?.serverURL || 'default'}.`
  return {
    getItem: key => localStorage.getItem(prefix + key),
    setItem: (key, value) => localStorage.setItem(prefix + key, value),
    removeItem: key => localStorage.removeItem(prefix + key),
  }
}

export function createSocket(device) {
  if (!isDesktop()) return new WebSocket(`${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/ws?device_id=${encodeURIComponent(device)}`)
  return new DesktopSocket(device)
}

// Native websockets avoid WebView-specific cookie and Origin restrictions.
// Keep the interface used by Requests and the existing reconnect/sync loop.
export class DesktopSocket {
  constructor(device) {
    this.readyState = 0
    this.id = crypto.randomUUID()
    this.bridge = native()
    this.unsubscribe = window.runtime.EventsOn('desktop:socket', event => {
      if (event.id !== this.id || this.readyState === 3) return
      if (event.type === 'open' && this.readyState === 0) { this.readyState = 1; this.onopen?.({}) }
      if (event.type === 'message' && this.readyState === 1) this.onmessage?.({ data: event.data })
      if (event.type === 'close') this.finish()
    })
    this.opening = this.bridge.OpenSocket(this.id, device)
    this.opening.then(() => {
      if (this.readyState >= 2) this.bridge.CloseSocket(this.id).catch(() => {})
    }).catch(error => { this.onerror?.(error); this.finish() })
  }

  send(payload) {
    if (this.readyState !== 1) throw new Error('实时连接未就绪')
    // Preserve call order even though bridge calls return promises.
    this.writes = (this.writes || Promise.resolve()).then(() => this.bridge.SendSocket(this.id, payload))
    this.writes.catch(error => { this.onerror?.(error); this.close() })
  }

  close() {
    if (this.readyState >= 2) return
    this.readyState = 2
    this.bridge.CloseSocket(this.id).catch(() => {}).finally(() => this.finish())
  }

  finish() {
    if (this.readyState === 3) return
    this.readyState = 3
    this.unsubscribe?.()
    this.onclose?.({})
  }
}

// Media links use the native Save dialog in desktop mode. No base64 copies of
// large files are passed through JS, and authenticated URLs never open outside.
export function installDesktopDownloads(report) {
  if (!isDesktop()) return () => {}
  const handle = event => {
    const link = event.target.closest?.('a[href]')
    if (!link) return
    const url = new URL(link.href, location.href)
    const match = url.pathname.match(/^\/api\/v1\/media\/([A-Za-z0-9_-]{1,128})$/)
    if (url.origin !== location.origin || !match) return
    event.preventDefault()
    const name = link.getAttribute('download') || link.querySelector('img')?.alt || link.textContent.trim() || 'attachment'
    native().SaveAttachment(match[1], name).catch(error => report({ message: String(error?.message || error) }))
  }
  document.addEventListener('click', handle)
  return () => document.removeEventListener('click', handle)
}
