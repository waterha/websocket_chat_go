export function onImageLinkClick(event) {
  const link = event.target instanceof Element ? event.target.closest('a.block-image, a.media-item') : null
  if (!link) return
  event.preventDefault()

  const image = link.querySelector('img')
  const dialog = document.createElement('dialog')
  dialog.className = 'image-viewer'
  dialog.setAttribute('aria-label', image?.alt || '图片预览')

  const close = document.createElement('button')
  close.className = 'image-viewer-close'
  close.type = 'button'
  close.setAttribute('aria-label', '关闭图片预览')
  close.textContent = '×'
  close.addEventListener('click', () => dialog.close())

  const preview = document.createElement('img')
  preview.src = link.href
  preview.alt = image?.alt || '图片'

  dialog.append(close, preview)
  dialog.addEventListener('click', click => {
    if (click.target === dialog) dialog.close()
  })
  dialog.addEventListener('close', () => dialog.remove(), { once: true })
  document.body.append(dialog)
  dialog.showModal()
}
