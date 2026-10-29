import './avatar-crop.css'

export function cropBounds(width, height, zoom = 1, x = 0, y = 0) {
  const size = Math.min(width, height) / zoom
  const left = Math.max(0, Math.min(width - size, (width - size) / 2 - x * size))
  const top = Math.max(0, Math.min(height - size, (height - size) / 2 - y * size))
  return { left, top, size }
}

// Only the confirmed crop is uploaded; the original image remains local.
export async function cropAvatar(file, { signal } = {}) {
  if (signal?.aborted) return null
  const source = URL.createObjectURL(file)
  const image = new Image()
  try {
    await new Promise((resolve, reject) => {
      const abort = () => { image.src = ''; reject(new DOMException('已取消', 'AbortError')) }
      image.onload = () => { signal?.removeEventListener('abort', abort); resolve() }
      image.onerror = () => { signal?.removeEventListener('abort', abort); reject(new Error('无法读取头像图片，请更换图片')) }
      signal?.addEventListener('abort', abort, { once: true })
      image.src = source
    })
    if (signal?.aborted) return null
    return await new Promise(resolve => {
      const previousFocus = document.activeElement
      const dialog = document.createElement('dialog')
      dialog.className = 'avatar-crop-dialog'
      dialog.setAttribute('aria-label', '裁剪头像')
      dialog.innerHTML = `<div class="avatar-crop-head"><h2>裁剪头像</h2><button type="button" aria-label="关闭裁剪">×</button></div><p class="subtle">拖动图片调整位置，缩放选取想要的部分。裁剪后为静态头像。</p><div class="avatar-crop-stage" role="img" aria-label="头像裁剪预览，可使用方向键调整位置" tabindex="0"><img alt="" draggable="false" /><span class="avatar-crop-guide" aria-hidden="true"></span></div><label class="avatar-crop-zoom">缩放<input type="range" min="1" max="4" step="0.01" value="1" aria-label="缩放头像" /></label><p class="avatar-crop-error" role="alert"></p><div class="avatar-crop-actions"><button type="button" class="outline-button" data-action="reset">重置</button><span></span><button type="button" class="outline-button" data-action="cancel">取消</button><button type="button" class="primary" data-action="confirm">确认并上传</button></div>`
      const stage = dialog.querySelector('.avatar-crop-stage')
      const preview = stage.querySelector('img')
      preview.src = source
      const range = dialog.querySelector('input')
      const confirm = dialog.querySelector('[data-action="confirm"]')
      let zoom = 1, x = 0, y = 0, pointer, done = false
      function render() {
        const bounds = cropBounds(image.naturalWidth, image.naturalHeight, zoom, x, y)
        x = ((image.naturalWidth - bounds.size) / 2 - bounds.left) / bounds.size
        y = ((image.naturalHeight - bounds.size) / 2 - bounds.top) / bounds.size
        preview.style.width = `${image.naturalWidth / bounds.size * 100}%`
        preview.style.height = `${image.naturalHeight / bounds.size * 100}%`
        preview.style.left = `${-bounds.left / bounds.size * 100}%`
        preview.style.top = `${-bounds.top / bounds.size * 100}%`
      }
      function finish(result) {
        if (done) return
        done = true
        signal?.removeEventListener('abort', cancelled)
        dialog.close(); dialog.remove(); previousFocus?.focus?.(); resolve(result)
      }
      const cancelled = () => finish(null)
      dialog.querySelector('[aria-label="关闭裁剪"]').onclick = cancelled
      dialog.querySelector('[data-action="cancel"]').onclick = cancelled
      dialog.addEventListener('cancel', event => { event.preventDefault(); cancelled() })
      dialog.addEventListener('keydown', event => {
        if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); cancelled() }
      })
      signal?.addEventListener('abort', cancelled, { once: true })
      range.oninput = () => { zoom = Number(range.value); render() }
      dialog.querySelector('[data-action="reset"]').onclick = () => { zoom = 1; x = y = 0; range.value = '1'; render() }
      stage.onpointerdown = event => { if (event.button !== 0) return; event.preventDefault(); stage.focus(); pointer = { id: event.pointerId, x: event.clientX, y: event.clientY }; stage.setPointerCapture?.(event.pointerId) }
      stage.onpointermove = event => {
        if (!pointer || pointer.id !== event.pointerId) return
        const width = stage.getBoundingClientRect().width || 280
        x += (event.clientX - pointer.x) / width; y += (event.clientY - pointer.y) / width
        pointer.x = event.clientX; pointer.y = event.clientY; render()
      }
      stage.onpointerup = stage.onpointercancel = stage.onlostpointercapture = () => { pointer = null }
      stage.onkeydown = event => {
        const delta = { ArrowLeft: [-.02, 0], ArrowRight: [.02, 0], ArrowUp: [0, -.02], ArrowDown: [0, .02] }[event.key]
        if (!delta) return
        event.preventDefault(); x += delta[0]; y += delta[1]; render()
      }
      confirm.onclick = () => {
        confirm.disabled = true
        try {
          const bounds = cropBounds(image.naturalWidth, image.naturalHeight, zoom, x, y)
          const canvas = document.createElement('canvas')
          canvas.width = canvas.height = 512
          const context = canvas.getContext('2d')
          if (!context) throw new Error('当前环境无法裁剪图片')
          context.fillStyle = '#ffffff'; context.fillRect(0, 0, 512, 512)
          context.drawImage(image, bounds.left, bounds.top, bounds.size, bounds.size, 0, 0, 512, 512)
          canvas.toBlob(blob => {
            if (done) return
            if (!blob) { confirm.disabled = false; dialog.querySelector('.avatar-crop-error').textContent = '无法生成头像，请重试'; return }
            finish(new File([blob], 'avatar.jpg', { type: 'image/jpeg' }))
          }, 'image/jpeg', .92)
        } catch (cause) { confirm.disabled = false; dialog.querySelector('.avatar-crop-error').textContent = cause.message }
      }
      document.body.append(dialog); render(); dialog.showModal(); stage.focus()
    })
  } catch (cause) {
    if (cause.name === 'AbortError') return null
    throw cause
  } finally { URL.revokeObjectURL(source) }
}
