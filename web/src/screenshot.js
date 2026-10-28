const MIN_SELECTION_SIZE = 4

const aborted = () => new DOMException('截图已取消', 'AbortError')
const clamp = (value, min, max) => Math.max(min, Math.min(max, value))
const stopTracks = stream => { for (const track of stream?.getTracks?.() || []) track.stop() }

// Map either drag direction from the scaled preview to original screen pixels.
export function selectionPixels(selection, rect, width, height) {
  if (!selection || rect.width <= 0 || rect.height <= 0) return null
  const left = clamp(Math.min(selection.x1, selection.x2), rect.left, rect.right)
  const right = clamp(Math.max(selection.x1, selection.x2), rect.left, rect.right)
  const top = clamp(Math.min(selection.y1, selection.y2), rect.top, rect.bottom)
  const bottom = clamp(Math.max(selection.y1, selection.y2), rect.top, rect.bottom)
  if (right - left < MIN_SELECTION_SIZE || bottom - top < MIN_SELECTION_SIZE) return null
  const x = Math.max(0, Math.floor((left - rect.left) * width / rect.width))
  const y = Math.max(0, Math.floor((top - rect.top) * height / rect.height))
  return { x, y, width: Math.min(width, Math.ceil((right - rect.left) * width / rect.width)) - x, height: Math.min(height, Math.ceil((bottom - rect.top) * height / rect.height)) - y }
}

function pngBlob(canvas) {
  return new Promise((resolve, reject) => canvas.toBlob(blob => blob ? resolve(blob) : reject(new Error('无法生成截图，请缩小截图区域后重试')), 'image/png'))
}

function videoFrame(video, signal) {
  return new Promise((resolve, reject) => {
    let timer
    const finish = error => {
      clearTimeout(timer)
      video.removeEventListener('loadeddata', ready)
      video.removeEventListener('error', failed)
      signal?.removeEventListener('abort', cancelled)
      error ? reject(error) : resolve()
    }
    const ready = () => { if (video.readyState >= 2 && video.videoWidth && video.videoHeight) finish() }
    const failed = () => finish(new Error('无法读取屏幕画面'))
    const cancelled = () => finish(aborted())
    video.addEventListener('loadeddata', ready)
    video.addEventListener('error', failed)
    signal?.addEventListener('abort', cancelled, { once: true })
    timer = setTimeout(failed, 10000)
    if (signal?.aborted) { cancelled(); return }
    video.play().then(ready, failed)
    ready()
  })
}

function nativeImage(dataURL, documentRef, signal) {
  if (typeof dataURL !== 'string' || !dataURL.startsWith('data:image/png;base64,')) throw new Error('桌面截图返回了无效图片')
  return new Promise((resolve, reject) => {
    const image = documentRef.createElement('img')
    const finish = error => {
      image.onload = image.onerror = null
      signal?.removeEventListener('abort', cancelled)
      error ? reject(error) : resolve(image)
    }
    const cancelled = () => finish(aborted())
    image.onload = () => finish()
    image.onerror = () => finish(new Error('无法读取桌面截图'))
    signal?.addEventListener('abort', cancelled, { once: true })
    if (signal?.aborted) { cancelled(); return }
    image.src = dataURL
  })
}

function selectRegion(canvas, imageURL, documentRef, windowRef, signal) {
  if (signal?.aborted) return Promise.resolve(null)
  return new Promise((resolve, reject) => {
    const previousFocus = documentRef.activeElement
    const overlay = documentRef.createElement('div')
    overlay.className = 'screenshot-overlay'
    overlay.setAttribute('role', 'dialog')
    overlay.setAttribute('aria-modal', 'true')
    overlay.setAttribute('aria-label', '选择截图区域')
    overlay.innerHTML = `<div class="screenshot-toolbar"><div><strong>截图</strong><span>拖动选择区域 · Enter 确认 · Esc 取消</span></div><span class="screenshot-selection-size" aria-live="polite"></span><div class="screenshot-toolbar-actions"><button type="button" data-screenshot-cancel>取消</button><button type="button" data-screenshot-confirm disabled>插入聊天草稿</button></div></div><div class="screenshot-preview"><img draggable="false" alt="屏幕截图预览"><div class="screenshot-selection" hidden></div></div>`
    const image = overlay.querySelector('img')
    const preview = overlay.querySelector('.screenshot-preview')
    const box = overlay.querySelector('.screenshot-selection')
    const sizeLabel = overlay.querySelector('.screenshot-selection-size')
    const confirm = overlay.querySelector('[data-screenshot-confirm]')
    const cancel = overlay.querySelector('[data-screenshot-cancel]')
    let start = null
    let selection = null
    let settled = false
    const finish = (value = null, error) => {
      if (settled) return
      settled = true
      documentRef.removeEventListener('keydown', keyboard, true)
      windowRef.removeEventListener('resize', cancelled)
      signal?.removeEventListener('abort', cancelled)
      overlay.remove()
      if (previousFocus?.isConnected) previousFocus.focus({ preventScroll: true })
      error ? reject(error) : resolve(value)
    }
    const cancelled = () => finish()
    const complete = () => {
      // Read geometry while the preview still exists; removing the dialog first
      // loses its scale and position and makes the resulting crop incorrect.
      const pixels = selectionPixels(selection, image.getBoundingClientRect(), canvas.width, canvas.height)
      if (pixels) finish(pixels)
    }
    const keyboard = event => {
      if (event.key === 'Escape' || (event.key === 'Enter' && !confirm.disabled)) {
        event.preventDefault()
        event.stopImmediatePropagation()
        event.key === 'Escape' ? finish() : complete()
      } else if (event.key === 'Tab') {
        event.preventDefault()
        ;(documentRef.activeElement === cancel && !confirm.disabled ? confirm : cancel).focus()
      }
    }
    const updateSelection = event => {
      if (!start) return
      selection = { ...start, x2: event.clientX, y2: event.clientY }
      const rect = image.getBoundingClientRect()
      const pixels = selectionPixels(selection, rect, canvas.width, canvas.height)
      confirm.disabled = !pixels
      box.hidden = !pixels
      sizeLabel.textContent = pixels ? `${pixels.width} × ${pixels.height} px` : ''
      if (!pixels) return
      const area = preview.getBoundingClientRect()
      box.style.left = `${clamp(Math.min(selection.x1, selection.x2), rect.left, rect.right) - area.left}px`
      box.style.top = `${clamp(Math.min(selection.y1, selection.y2), rect.top, rect.bottom) - area.top}px`
      box.style.width = `${Math.abs(clamp(selection.x2, rect.left, rect.right) - selection.x1)}px`
      box.style.height = `${Math.abs(clamp(selection.y2, rect.top, rect.bottom) - selection.y1)}px`
    }
    image.addEventListener('pointerdown', event => {
      if (event.button !== 0) return
      event.preventDefault()
      image.setPointerCapture?.(event.pointerId)
      start = { x1: event.clientX, y1: event.clientY }
      updateSelection(event)
    })
    image.addEventListener('pointermove', updateSelection)
    image.addEventListener('pointerup', event => { updateSelection(event); start = null })
    image.addEventListener('pointercancel', () => { start = selection = null; box.hidden = true; confirm.disabled = true; sizeLabel.textContent = '' })
    image.addEventListener('error', () => finish(null, new Error('无法显示截图预览')))
    cancel.addEventListener('click', cancelled)
    confirm.addEventListener('click', complete)
    documentRef.addEventListener('keydown', keyboard, true)
    windowRef.addEventListener('resize', cancelled)
    signal?.addEventListener('abort', cancelled, { once: true })
    image.src = imageURL
    documentRef.body.append(overlay)
    cancel.focus()
  })
}

export async function captureScreenRegion({
  mediaDevices = globalThis.navigator?.mediaDevices,
  bridge = globalThis.window?.go?.main?.App,
  documentRef = globalThis.document,
  windowRef = globalThis.window,
  signal,
  now = Date.now,
} = {}) {
  let stream
  let video
  let imageURL
  const cancelStream = () => stopTracks(stream)
  signal?.addEventListener('abort', cancelStream, { once: true })
  try {
    if (signal?.aborted) return null
    const canvas = documentRef.createElement('canvas')
    const context = canvas.getContext('2d')
    if (!context) throw new Error('当前环境无法处理截图')
    if (bridge) {
      if (!bridge.CaptureScreenshot) throw new Error('请更新桌面客户端以使用 Ctrl+Q 截图')
      const dataURL = await bridge.CaptureScreenshot()
      if (signal?.aborted) return null
      const image = await nativeImage(dataURL, documentRef, signal)
      canvas.width = image.naturalWidth
      canvas.height = image.naturalHeight
      context.drawImage(image, 0, 0)
    } else {
      if (!mediaDevices?.getDisplayMedia) throw new Error('当前环境不支持屏幕截图，请使用 HTTPS 或本机地址，并更新 Chrome/Edge')
      // Keep this before the first await: screen sharing requires the original
      // key/click user activation. Only one frame is retained and never uploaded.
      stream = await mediaDevices.getDisplayMedia({ video: true, audio: false })
      if (signal?.aborted) return null
      video = documentRef.createElement('video')
      video.muted = true
      video.playsInline = true
      video.srcObject = stream
      await videoFrame(video, signal)
      canvas.width = video.videoWidth
      canvas.height = video.videoHeight
      context.drawImage(video, 0, 0)
      stopTracks(stream)
      stream = null
      video.srcObject = null
    }
    if (!canvas.width || !canvas.height) throw new Error('截图画面为空，请重试')
    if (signal?.aborted) return null
    imageURL = URL.createObjectURL(await pngBlob(canvas))
    const pixels = await selectRegion(canvas, imageURL, documentRef, windowRef, signal)
    if (!pixels || signal?.aborted) return null
    const crop = documentRef.createElement('canvas')
    crop.width = pixels.width
    crop.height = pixels.height
    const croppedContext = crop.getContext('2d')
    if (!croppedContext) throw new Error('无法处理所选截图区域')
    croppedContext.drawImage(canvas, pixels.x, pixels.y, pixels.width, pixels.height, 0, 0, pixels.width, pixels.height)
    const blob = await pngBlob(crop)
    if (signal?.aborted) return null
    return new File([blob], `screenshot-${now()}.png`, { type: 'image/png' })
  } catch (error) {
    if (error?.name === 'NotAllowedError' || error?.name === 'AbortError') return null
    throw error
  } finally {
    signal?.removeEventListener('abort', cancelStream)
    stopTracks(stream)
    if (video) { video.pause(); video.srcObject = null }
    if (imageURL) URL.revokeObjectURL(imageURL)
  }
}

export function installScreenshot({ canCapture, getContext, isCurrent, onCapture, onError, capture = captureScreenRegion, documentRef = document }) {
  let controller = null
  let disposed = false
  async function start() {
    if (disposed || controller || !canCapture()) return
    const context = getContext()
    const current = new AbortController()
    controller = current
    try {
      const file = await capture({ signal: current.signal })
      if (file && !current.signal.aborted && !disposed && isCurrent(context) && canCapture()) await onCapture(file, context)
    } catch (error) {
      if (!current.signal.aborted && !disposed && isCurrent(context)) onError({ message: String(error?.message || error) })
    } finally { if (controller === current) controller = null }
  }
  function cancel() { controller?.abort() }
  const keyboard = event => {
    if (!event.ctrlKey || event.altKey || event.shiftKey || event.metaKey || event.isComposing || event.keyCode === 229 || event.key.toLowerCase() !== 'q' || !canCapture()) return
    event.preventDefault()
    event.stopPropagation()
    if (!event.repeat) start()
  }
  documentRef.addEventListener('keydown', keyboard, true)
  return { start, cancel, dispose() { disposed = true; cancel(); documentRef.removeEventListener('keydown', keyboard, true) } }
}
