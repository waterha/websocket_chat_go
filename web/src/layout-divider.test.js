import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'

const dom = new JSDOM('<div id="app"></div>', { url: 'http://localhost' })
for (const name of ['window', 'document', 'Element', 'HTMLElement', 'SVGElement', 'localStorage']) globalThis[name] = dom.window[name]
const { createApp, nextTick } = await import('vue/dist/vue.esm-bundler.js')
const { LayoutDivider, panelWidth } = await import('./layout-divider.js')

test('panel bounds preserve at least 360px of chat space', () => {
  assert.equal(panelWidth(50, 1280, 240), 180)
  assert.equal(panelWidth(800, 1280, 240), 420)
  assert.equal(panelWidth(420, 900, 280), 260)
})

test('both boundaries drag in the correct direction, support keyboard and persist', async () => {
  for (const side of ['sidebar', 'detail']) {
    localStorage.clear()
    const app = createApp({ components: { LayoutDivider }, template: `<div class="layout"><aside class="${side === 'sidebar' ? 'sidebar' : 'detail-panel'}"><LayoutDivider side="${side}" /></aside></div>` })
    app.mount('#app')
    const layout = document.querySelector('.layout')
    const panel = layout.querySelector('aside')
    Object.defineProperty(layout, 'clientWidth', { value: 1280 })
    panel.getBoundingClientRect = () => ({ width: parseFloat(layout.style.getPropertyValue(`--${side}-width`)) || 280 })
    const handle = panel.querySelector('[role=separator]')
    handle.setPointerCapture = () => {}
    handle.dispatchEvent(new window.MouseEvent('pointerdown', { button: 0, clientX: 500, bubbles: true }))
    handle.dispatchEvent(new window.MouseEvent('pointermove', { clientX: 550, bubbles: true }))
    handle.dispatchEvent(new window.MouseEvent('pointerup', { bubbles: true }))
    const expected = side === 'sidebar' ? 330 : 230
    assert.equal(layout.style.getPropertyValue(`--${side}-width`), `${expected}px`)
    assert.equal(localStorage.getItem(`chat.panel-width.${side}`), String(expected))
    handle.dispatchEvent(new window.KeyboardEvent('keydown', { key: 'Home', bubbles: true }))
    await nextTick()
    assert.equal(handle.getAttribute('aria-valuenow'), '180')
    app.unmount()
  }
})
