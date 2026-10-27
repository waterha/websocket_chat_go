import { onMounted, onUnmounted, ref } from 'vue/dist/vue.esm-bundler.js'

export function panelWidth(requested, total, other) {
  return Math.max(180, Math.min(420, total - other - 360, requested))
}

export const LayoutDivider = {
  props: { side: { type: String, required: true } },
  setup(props) {
    const handle = ref(null)
    const width = ref(0)
    let observer
    let dragging = false
    let startX = 0
    let startWidth = 0
    let preferred
    const key = `chat.panel-width.${props.side}`
    const variable = `--${props.side}-width`
    function apply(value) {
      const panel = handle.value?.parentElement
      const layout = panel?.closest('.layout')
      if (!layout) return
      const other = layout.querySelector(props.side === 'sidebar' ? '.detail-panel' : '.sidebar')
      const otherWidth = other?.getBoundingClientRect().width || 0
      width.value = panelWidth(value, layout.clientWidth, otherWidth)
      layout.style.setProperty(variable, `${width.value}px`)
    }
    function save() {
      preferred = width.value
      try { localStorage.setItem(key, String(preferred)) } catch {}
    }
    function down(event) {
      if (event.button !== 0) return
      event.preventDefault()
      dragging = true
      startX = event.clientX
      startWidth = handle.value.parentElement.getBoundingClientRect().width
      handle.value.setPointerCapture(event.pointerId)
    }
    function move(event) {
      if (dragging) apply(startWidth + (event.clientX - startX) * (props.side === 'sidebar' ? 1 : -1))
    }
    function finish() { if (dragging) { dragging = false; save() } }
    function keyboard(event) {
      if (!['ArrowLeft', 'ArrowRight', 'Home'].includes(event.key)) return
      event.preventDefault()
      const current = handle.value.parentElement.getBoundingClientRect().width
      apply(event.key === 'Home' ? 180 : current + (event.key === 'ArrowRight' ? 16 : -16) * (props.side === 'sidebar' ? 1 : -1))
      save()
    }
    onMounted(() => {
      try { preferred = Number(localStorage.getItem(key)) || undefined } catch {}
      width.value = handle.value.parentElement.getBoundingClientRect().width
      if (preferred) apply(preferred)
      if (typeof ResizeObserver !== 'undefined') {
        observer = new ResizeObserver(() => apply(preferred || width.value))
        observer.observe(handle.value.closest('.layout'))
      }
    })
    onUnmounted(() => observer?.disconnect())
    return { handle, width, down, move, finish, keyboard }
  },
  template: `<div ref="handle" class="layout-divider" :class="side" role="separator" aria-orientation="vertical" :aria-label="side === 'sidebar' ? '调整消息列表宽度' : '调整群信息宽度'" tabindex="0" :aria-valuenow="Math.round(width)" aria-valuemin="180" aria-valuemax="420" @pointerdown="down" @pointermove="move" @pointerup="finish" @pointercancel="finish" @lostpointercapture="finish" @keydown="keyboard"></div>`,
}
