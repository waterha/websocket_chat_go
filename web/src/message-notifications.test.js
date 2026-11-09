import test from 'node:test'
import assert from 'node:assert/strict'
import { effectScope, ref } from 'vue/dist/vue.esm-bundler.js'
import { useMessageNotifications } from './message-notifications.js'

function fixture(t) {
  const old = { window: globalThis.window, document: globalThis.document, Notification: globalThis.Notification }
  const popups = []
  globalThis.window = { focus() {} }
  globalThis.document = { visibilityState: 'hidden', hasFocus: () => false }
  globalThis.Notification = class {
    static permission = 'granted'
    static async requestPermission() { return 'granted' }
    constructor(title, options) { this.title = title; this.options = options; popups.push(this) }
    close() { this.closed = true }
  }
  const me = ref({ id: 'me' }), conversations = ref([{ id: 'chat', last_seq: 10, muted: false, members: [{ id: 'bob', name: '小明' }] }])
  const scope = effectScope(), opened = [], viewedConversation = ref(null)
  let state
  scope.run(() => { state = useMessageNotifications({ me, conversations, title: () => '测试会话', isViewingConversation: id => viewedConversation.value === id, onOpen: id => opened.push(id), showError: error => { throw error }, clock: () => 100000 }) })
  state.prime(conversations.value)
  t.after(() => { scope.stop(); Object.assign(globalThis, old) })
  return { state, me, conversations, popups, opened, viewedConversation }
}
const message = (seq, extra = {}) => ({ id: `m${seq}`, seq, sender_id: 'bob', kind: 'user', content: '新消息', created_at: new Date(101000).toISOString(), ...extra })

test('only new incoming messages notify; duplicates, history, own messages and recalls do not', t => {
  const { state, popups } = fixture(t)
  state.receive('chat', [message(9), message(10), message(11, { sender_id: 'me' }), message(12, { kind: 'system' }), message(13, { recalled_at: 'now' })])
  assert.equal(state.notices.value.length, 0)
  state.receive('chat', [message(14), message(14)])
  assert.equal(state.notices.value.length, 1)
  assert.equal(state.notices.value[0].count, 1)
  assert.equal(popups.length, 1)
  state.receive('chat', [message(14)])
  assert.equal(popups.length, 1)
})
test('muted messages never notify or replay after unmuting', t => {
  const { state, conversations, popups } = fixture(t)
  conversations.value[0].muted = true
  state.receive('chat', [message(11)])
  conversations.value[0].muted = false
  state.receive('chat', [message(11)])
  assert.equal(state.notices.value.length, 0)
  assert.equal(popups.length, 0)
  state.receive('chat', [message(12)])
  assert.equal(popups.length, 1)
})
test('a new conversation ignores historical pages and notification click opens its chat', t => {
  const { state, conversations, opened } = fixture(t)
  conversations.value.push({ id: 'new', muted: false, members: [] })
  state.receive('new', [message(1, { created_at: new Date(1000).toISOString() })])
  state.receive('new', [message(2, { created_at: new Date(2000).toISOString() })])
  assert.equal(state.notices.value.length, 0)
  state.receive('new', [message(3)])
  state.openNotice(state.notices.value[0])
  assert.deepEqual(opened, ['new'])
  assert.equal(state.notices.value.length, 0)
})
test('a visible focused conversation receives messages without either kind of notice or replay after blur', t => {
  const { state, popups, viewedConversation } = fixture(t)
  viewedConversation.value = 'chat'
  document.visibilityState = 'visible'
  document.hasFocus = () => true
  state.receive('chat', [message(11)])
  assert.equal(state.notices.value.length, 0)
  assert.equal(popups.length, 0)
  document.hasFocus = () => false
  state.receive('chat', [message(11)])
  assert.equal(state.notices.value.length, 0)
  state.receive('chat', [message(12)])
  assert.equal(state.notices.value.length, 1)
  assert.equal(state.notices.value[0].count, 1)
  assert.equal(popups.length, 1)
})
test('viewing another conversation in a focused window still shows an app notice', t => {
  const { state, popups, viewedConversation } = fixture(t)
  viewedConversation.value = 'other'
  document.visibilityState = 'visible'
  document.hasFocus = () => true
  state.receive('chat', [message(11)])
  assert.equal(state.notices.value.length, 1)
  assert.equal(popups.length, 0)
})
test('a hidden page still notifies even if the active conversation and focus report match', t => {
  const { state, popups, viewedConversation } = fixture(t)
  viewedConversation.value = 'chat'
  document.hasFocus = () => true
  state.receive('chat', [message(11)])
  assert.equal(state.notices.value.length, 1)
  assert.equal(popups.length, 1)
})
test('a focused page without an open message view still notifies', t => {
  const { state, popups } = fixture(t)
  document.visibilityState = 'visible'
  document.hasFocus = () => true
  state.receive('chat', [message(11)])
  assert.equal(state.notices.value.length, 1)
  assert.equal(popups.length, 0)
})
test('app notices work without native Notification permission and reset on logout', t => {
  const { state, me, popups } = fixture(t)
  globalThis.Notification.permission = 'default'
  state.receive('chat', [message(11)])
  assert.equal(state.notices.value.length, 1)
  assert.equal(popups.length, 0)
  me.value = null
  assert.equal(state.notices.value.length, 0)
})
