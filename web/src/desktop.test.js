import test from 'node:test'
import assert from 'node:assert/strict'
import { chatStorage, createSocket, isDesktop } from './desktop.js'

test('web builds keep normal HTTP websocket and browser storage', () => {
  const savedWindow = globalThis.window
  const savedLocation = globalThis.location
  const savedWebSocket = globalThis.WebSocket
  const savedLocal = globalThis.localStorage
  const savedSession = globalThis.sessionStorage
  const webSocket = class MockSocket { constructor(url) { this.url = url } }
  globalThis.window = {}
  globalThis.location = { protocol: 'https:', host: 'chat.example.com' }
  globalThis.WebSocket = webSocket
  globalThis.localStorage = { getItem() { return 'local' } }
  globalThis.sessionStorage = { getItem() { return 'session' } }
  try {
    assert.equal(isDesktop(), false)
    assert.equal(chatStorage().getItem('key'), 'session')
    assert.equal(chatStorage(true).getItem('key'), 'local')
    assert.equal(createSocket('device 1').url, 'wss://chat.example.com/ws?device_id=device%201')
  } finally {
    globalThis.window = savedWindow
    globalThis.location = savedLocation
    globalThis.WebSocket = savedWebSocket
    globalThis.localStorage = savedLocal
    globalThis.sessionStorage = savedSession
  }
})

test('desktop chat storage partitions data by the configured server', async () => {
  const savedWindow = globalThis.window
  const savedLocal = globalThis.localStorage
  const savedDocument = globalThis.document
  const values = new Map()
  globalThis.localStorage = {
    getItem(key) { return values.get(key) || null },
    setItem(key, value) { values.set(key, value) },
    removeItem(key) { values.delete(key) },
  }
  globalThis.document = { documentElement: { classList: { add() {} } } }
  try {
    globalThis.window = { go: { main: { App: { async Config() { return { serverURL: 'https://first.example.com', origin: 'https://web.example.com' } } } } } }
    const desktop = await import(`./desktop.js?first=${Date.now()}`)
    await desktop.initializeDesktop()
    desktop.chatStorage().setItem('im.device', 'first-device')
    assert.equal(globalThis.localStorage.getItem('im.desktop.https://first.example.com.im.device'), 'first-device')

    globalThis.window.go.main.App.Config = async () => ({ serverURL: 'https://second.example.com', origin: 'https://second.example.com' })
    await desktop.initializeDesktop()
    assert.equal(desktop.chatStorage().getItem('im.device'), null)
  } finally {
    globalThis.window = savedWindow
    globalThis.localStorage = savedLocal
    globalThis.document = savedDocument
  }
})

test('desktop sockets carry messages on the native connection', async () => {
  const savedWindow = globalThis.window
  const savedCrypto = Object.getOwnPropertyDescriptor(globalThis, 'crypto')
  let listener
  const sent = []
  const bridge = {
    EventsOn(name, callback) { assert.equal(name, 'desktop:socket'); listener = callback; return () => { listener = null } },
    async OpenSocket(id, device) { assert.equal(device, 'device-1') },
    async SendSocket(id, payload) { sent.push([id, payload]) },
    async CloseSocket() {},
  }
  Object.defineProperty(globalThis, 'crypto', { value: { randomUUID: () => 'socket-1' }, configurable: true })
  globalThis.window = { go: { main: { App: bridge } }, runtime: { EventsOn: bridge.EventsOn } }
  try {
    const { DesktopSocket } = await import(`./desktop.js?socket=${Date.now()}`)
    const socket = new DesktopSocket('device-1')
    let opened = 0
    let received
    socket.onopen = () => opened++
    socket.onmessage = event => { received = event.data }
    await socket.opening
    listener({ id: 'other-socket', type: 'message', data: 'ignored' })
    listener({ id: 'socket-1', type: 'open' })
    listener({ id: 'socket-1', type: 'message', data: 'ready' })
    assert.equal(opened, 1)
    assert.equal(received, 'ready')
    socket.send('chat message')
    await socket.writes
    assert.deepEqual(sent, [['socket-1', 'chat message']])
    socket.close()
    assert.equal(socket.readyState, 2)
  } finally {
    globalThis.window = savedWindow
    if (savedCrypto) Object.defineProperty(globalThis, 'crypto', savedCrypto)
    else delete globalThis.crypto
  }
})
