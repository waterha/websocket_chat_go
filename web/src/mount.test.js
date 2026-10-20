import test from 'node:test'
import assert from 'node:assert/strict'
import { register } from 'node:module'
import { JSDOM } from 'jsdom'

// Mounting the real component is the only check that proves the interface
// renders. A build and a template-compile test both passed while the page
// displayed nothing, because the setup function threw on an undefined binding:
// a runtime error in setup is invisible to a bundler and to unit tests that
// never mount. This test mounts the component graph in a DOM for that reason.
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
// The unauthenticated answer keeps the test on the login screen, which exercises
// setup, the template and the first render without needing a server.
globalThis.fetch = async () => ({ ok: false, status: 401, json: async () => ({ error: { code: 'unauthorized', message: 'unauthorized' } }) })
globalThis.URL.createObjectURL = () => 'blob:test'
globalThis.URL.revokeObjectURL = () => {}

const problems = []
const originalError = console.error
console.error = (...args) => { problems.push(args.map(value => String(value)).join(' ')); originalError(...args) }

// The component installs a refresh interval and a reconnect timer on mount, which
// would keep the test runner alive. Both are tracked here and cleared once the
// assertions finish.
const timers = new Set()
const realSetInterval = globalThis.setInterval
const realSetTimeout = globalThis.setTimeout
globalThis.setInterval = (handler, delay, ...args) => {
  const handle = realSetInterval(handler, delay, ...args)
  timers.add(handle)
  return handle
}
globalThis.setTimeout = (handler, delay, ...args) => {
  const handle = realSetTimeout(handler, delay, ...args)
  timers.add(handle)
  return handle
}

let renderError
try {
  await import('../src/main.js')
} catch (error) {
  renderError = error
}
// Let the async onMounted work settle.
await new Promise(resolve => realSetTimeout(resolve, 200))
console.error = originalError

test('the component mounts and renders the login screen', () => {
  assert.equal(renderError, undefined, `mounting threw: ${renderError}`)
  // Vue reports setup and render failures through console.error rather than by
  // throwing, so both channels are checked.
  const runtimeProblems = problems.filter(line => !line.startsWith('Debug'))
  assert.deepEqual(runtimeProblems, [], `runtime errors: ${runtimeProblems.join(' | ')}`)
  const app = document.querySelector('#app')
  assert.ok(app.innerHTML.length > 200, `the app rendered almost nothing: ${app.innerHTML.length} chars`)
  assert.ok(app.textContent.includes('登录'), 'the login screen did not render')
})

test.after(() => {
  for (const handle of timers) {
    clearInterval(handle)
    clearTimeout(handle)
  }
  globalThis.setInterval = realSetInterval
  globalThis.setTimeout = realSetTimeout
  dom.window.close()
})
