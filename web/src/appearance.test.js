import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'
import { THEME_KEY, defaultColor, themePresets, normalizeColor, themeTokens, applyTheme, loadTheme, saveTheme } from './appearance-theme.js'

const dom = new JSDOM('<body><button id="opener">设置</button><div id="app"></div></body>', { url: 'http://127.0.0.1:4173' })
for (const key of ['window', 'document', 'Document', 'Element', 'HTMLElement', 'SVGElement', 'Node']) globalThis[key] = key === 'window' ? dom.window : dom.window[key]
globalThis.localStorage = dom.window.localStorage
const { createApp, nextTick } = await import('vue/dist/vue.esm-bundler.js')
const { AppearanceSettings } = await import('./appearance-settings.js')
let app

test.afterEach(() => { app?.unmount(); app = undefined; localStorage.clear(); applyTheme(defaultColor) })
test.after(() => dom.window.close())

test('stored colors validate, persist and recover from unavailable storage', () => {
  assert.equal(normalizeColor('#ABCDEF'), '#abcdef')
  assert.equal(normalizeColor('red; color: red'), defaultColor)
  assert.equal(loadTheme(), defaultColor)
  saveTheme('#3675b5')
  assert.equal(loadTheme(), '#3675b5')
  assert.equal(loadTheme({ getItem() { throw new Error('blocked') } }), defaultColor)
})

test('all theme surfaces update and restoring default removes overrides', () => {
  applyTheme('#8060b0')
  for (const [key, value] of Object.entries(themeTokens('#8060b0'))) assert.equal(document.documentElement.style.getPropertyValue(key), value)
  applyTheme(defaultColor)
  for (const key of Object.keys(themeTokens(defaultColor))) assert.equal(document.documentElement.style.getPropertyValue(key), '')
})

test('custom action colors keep white labels legible across the hue spectrum', () => {
  const colors = [...themePresets.map(preset => preset.color), '#ffffff', '#000000', '#ffff00', '#00ff00', '#00ffff']
  for (let hue = 0; hue < 360; hue += 5) {
    const rgb = [0, 8, 4].map(offset => {
      const k = (offset + hue / 30) % 12
      return Math.round(255 * (0.5 - 0.5 * Math.max(-1, Math.min(k - 3, 9 - k, 1))))
    })
    colors.push('#' + rgb.map(value => value.toString(16).padStart(2, '0')).join(''))
  }
  for (const color of colors) {
    const [, h, s, l] = themeTokens(color)['--accent'].match(/hsl\((\d+) (\d+)% (\d+)%\)/).map(Number)
    const channels = [0, 8, 4].map(offset => {
      const k = (offset + h / 30) % 12
      const value = l / 100 - s / 100 * Math.min(l / 100, 1 - l / 100) * Math.max(-1, Math.min(k - 3, 9 - k, 1))
      return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
    })
    const luminance = channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722
    assert.ok(1.05 / (luminance + 0.05) >= 4.5, color)
  }
})

test('dialog switches presets, saves custom input, restores defaults and traps focus', async () => {
  const opener = document.querySelector('#opener'); opener.focus()
  let closed = 0
  app = createApp(AppearanceSettings, { onClose: () => closed++ }); app.mount('#app')
  await nextTick()
  const buttons = [...document.querySelectorAll('[role="dialog"] button')]
  assert.equal(document.activeElement, buttons[0])
  document.querySelectorAll('.appearance-preset')[1].click(); await nextTick()
  assert.equal(localStorage.getItem(THEME_KEY), themePresets[1].color)
  assert.equal(document.querySelectorAll('.appearance-preset')[1].getAttribute('aria-pressed'), 'true')
  const input = document.querySelector('input[type="color"]')
  input.value = '#00ff00'; input.dispatchEvent(new window.Event('input', { bubbles: true })); await nextTick()
  assert.equal(loadTheme(), '#00ff00')
  assert.ok(document.documentElement.style.getPropertyValue('--accent'))
  buttons.at(-1).focus()
  buttons.at(-1).dispatchEvent(new window.KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
  assert.equal(document.activeElement, buttons[0])
  document.querySelector('.outline-button').click(); await nextTick()
  assert.equal(loadTheme(), defaultColor)
  assert.equal(document.documentElement.style.getPropertyValue('--accent'), '')
  document.querySelector('[role="dialog"]').dispatchEvent(new window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
  assert.equal(closed, 1)
  app.unmount(); app = undefined
  assert.equal(document.activeElement, opener)
})
