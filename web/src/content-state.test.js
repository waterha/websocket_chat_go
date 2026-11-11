import test from 'node:test'
import assert from 'node:assert/strict'
import { register } from 'node:module'
register('../css-hook.mjs', import.meta.url)
import { JSDOM } from 'jsdom'
const dom = new JSDOM('<div id="app"></div>')
for (const key of ['window', 'document', 'Element', 'SVGElement', 'HTMLElement', 'Node']) globalThis[key] = dom.window[key]
const { createApp, nextTick, ref } = await import('vue/dist/vue.esm-bundler.js')
const { default: ContentState } = await import('./content-state.js')

test('all states use common markup, contextual descriptions, accessible roles and retry controls', async () => {
  const kind = ref('empty'), disabled = ref(false), description = ref('还没有群文件。'), retries = []
  const app = createApp({ components: { ContentState }, setup: () => ({ kind, disabled, description, retry: () => retries.push(1) }), template: '<ContentState :kind="kind" :description="description" :retry-disabled="disabled" compact @retry="retry" />' })
  app.mount('#app')
  assert.equal(document.querySelector('.content-state__title').textContent, '暂无内容')
  assert.equal(document.querySelector('[role="status"]').getAttribute('aria-atomic'), 'true')
  assert.equal(document.querySelector('.content-state__icon').getAttribute('aria-hidden'), 'true')
  assert.match(document.body.textContent, /还没有群文件/)
  assert.equal(document.querySelector('button'), null)
  kind.value = 'loading'; description.value = '正在加载群文件…'; await nextTick()
  assert.equal(document.querySelector('.content-state__title').textContent, '加载中…')
  assert.equal(document.querySelector('[aria-busy="true"]').getAttribute('role'), 'status')
  assert.ok(document.querySelector('.content-state__spinner'))
  kind.value = 'error'; description.value = '<img src=x onerror=alert(1)>'; await nextTick()
  assert.equal(document.querySelector('[role="alert"] .content-state__title').textContent, '加载失败')
  assert.equal(document.querySelector('img'), null)
  const retry = document.querySelector('.content-state__retry')
  assert.equal(retry.textContent, '重试'); assert.equal(retry.type, 'button')
  retry.click(); assert.equal(retries.length, 1)
  disabled.value = true; await nextTick(); retry.click(); assert.equal(retries.length, 1)
  app.unmount()
})
test.after(() => dom.window.close())
