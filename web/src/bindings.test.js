import test from 'node:test'
import assert from 'node:assert/strict'
import { register } from 'node:module'
import { JSDOM } from 'jsdom'
import { compile } from '@vue/compiler-dom'

// Guards a class of bug that no other check catches: the template calls a binding
// that the setup function never returns. Vue only warns at render time, and only
// when that specific branch renders, so it can sit unnoticed for a long time — the
// reply banner, the pending-send notice and the thread panel have each shipped
// this way. Comparing the compiled template against the returned bindings catches
// every such name at once instead of one branch at a time.
register('../css-hook.mjs', import.meta.url)

const dom = new JSDOM('<!doctype html><html><body><div id="app"></div></body></html>', { url: 'http://127.0.0.1:4173' })
globalThis.window = dom.window
globalThis.document = dom.window.document
Object.defineProperty(globalThis, 'navigator', { value: dom.window.navigator, configurable: true })
globalThis.location = dom.window.location
globalThis.HTMLElement = dom.window.HTMLElement
globalThis.Element = dom.window.Element
globalThis.Node = dom.window.Node
globalThis.SVGElement = dom.window.SVGElement
globalThis.Text = dom.window.Text
globalThis.Comment = dom.window.Comment
globalThis.DocumentFragment = dom.window.DocumentFragment
globalThis.MutationObserver = dom.window.MutationObserver
globalThis.getComputedStyle = dom.window.getComputedStyle.bind(dom.window)
globalThis.localStorage = dom.window.localStorage
globalThis.sessionStorage = dom.window.sessionStorage
globalThis.WebSocket = class { constructor() { this.readyState = 0 } close() {} }
globalThis.fetch = async () => ({ ok: false, status: 401, json: async () => ({ error: { code: 'unauthorized', message: 'unauthorized' } }) })
globalThis.URL.createObjectURL = () => 'blob:test'
globalThis.URL.revokeObjectURL = () => {}

const timers = new Set()
const realSetInterval = globalThis.setInterval
const realSetTimeout = globalThis.setTimeout
globalThis.setInterval = (handler, delay, ...args) => { const handle = realSetInterval(handler, delay, ...args); timers.add(handle); return handle }
globalThis.setTimeout = (handler, delay, ...args) => { const handle = realSetTimeout(handler, delay, ...args); timers.add(handle); return handle }

const { template } = await import('./template.js')
let mountError
try {
  await import('./main.js')
} catch (error) {
  mountError = error
}
await new Promise(resolve => realSetTimeout(resolve, 200))

// Names the compiled render function may legally reach through the component
// instance without being part of the setup result.
const BUILT_IN = new Set(['$event', '$props', '$attrs', '$slots', '$emit', '$el', '$options', '$refs', '$parent', '$root', '$data', '$watch', '$nextTick', '$forceUpdate'])

test('the component mounts', () => {
  assert.equal(mountError, undefined, `mounting threw: ${mountError}`)
})

test('every binding the template uses is returned by setup', () => {
  const instance = document.querySelector('#app').__vue_app__?._instance
  assert.ok(instance, 'the component instance was not created')
  const provided = new Set(Object.keys(instance.setupState))
  // Module mode is required: the default function mode wraps the render body in
  // `with (_ctx)`, so bindings appear as bare identifiers and cannot be told apart
  // from locals. Module mode prefixes every binding with `_ctx.`.
  const { code } = compile(template, { mode: 'module' })
  const referenced = new Set()
  for (const match of code.matchAll(/\b_ctx\.([A-Za-z_$][\w$]*)/g)) referenced.add(match[1])
  const missing = [...referenced].filter(name => !provided.has(name) && !BUILT_IN.has(name)).sort()
  assert.deepEqual(missing, [], `the template uses bindings that setup does not return: ${missing.join(', ')}`)
  // A sanity floor: the template must reference many bindings, otherwise the scan
  // silently stopped matching and would pass on any input.
  assert.ok(referenced.size > 100, `the scan found only ${referenced.size} bindings, so it is not reading the template`)
})

test('a template binding that setup omits is detected', () => {
  // Proves the check actually fails when a binding is missing, rather than always
  // passing because the scan matched nothing.
  const sample = '<div><button @click="notProvidedAnywhere">x</button><span>{{ alsoMissing }}</span><i v-for="row in rows">{{ row.name }}</i></div>'
  const { code } = compile(sample, { mode: 'module' })
  const referenced = [...code.matchAll(/\b_ctx\.([A-Za-z_$][\w$]*)/g)].map(match => match[1])
  assert.deepEqual(referenced.sort(), ['alsoMissing', 'notProvidedAnywhere', 'rows'])
})

test.after(() => {
  for (const handle of timers) { clearInterval(handle); clearTimeout(handle) }
  globalThis.setInterval = realSetInterval
  globalThis.setTimeout = realSetTimeout
  dom.window.close()
})
