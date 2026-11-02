import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'
import { ref } from 'vue/dist/vue.esm-bundler.js'
import { useContacts } from './composables/use-contacts.js'
import { useGlobalSearch } from './composables/use-global-search.js'
import { useThreadWindow } from './composables/use-thread-window.js'
import { useHistorySearch } from './composables/use-history-search.js'
import { useComposerUndo } from './composables/use-composer-undo.js'

const original = { document: globalThis.document, window: globalThis.window, fetch: globalThis.fetch }
let dom
test.beforeEach(() => {
  dom = new JSDOM('<div id="editor" contenteditable="true">草稿</div>', { url: 'http://localhost' })
  globalThis.document = dom.window.document
  globalThis.window = dom.window
})
test.afterEach(() => {
  dom.window.close()
  Object.assign(globalThis, original)
})
const response = data => ({ ok: true, json: async () => ({ data }) })
const fail = error => { throw error }

test('contact state is isolated and saving a remark updates only its own friends', async () => {
  const friends = ref([{ id: 'bob', remark: '' }])
  const contacts = useContacts({ friends, showSettings: ref(true), showError: fail, getEpoch: () => 0 })
  const other = useContacts({ friends: ref([]), showSettings: ref(false), showError: fail, getEpoch: () => 0 })
  contacts.startRemark(friends.value[0])
  contacts.remarkDraft.value = '好友备注'
  globalThis.fetch = async (url, options) => {
    assert.equal(url, '/api/v1/friends/bob')
    assert.equal(JSON.parse(options.body).remark, '好友备注')
    return response({ remark: '好友备注' })
  }
  await contacts.saveRemark(friends.value[0])
  assert.equal(friends.value[0].remark, '好友备注')
  assert.equal(contacts.remarkEditingID.value, '')
  assert.equal(contacts.contactBusy.value, false)
  assert.equal(other.remarkDraft.value, '')
})

test('contact loading still ignores an answer belonging to an old session', async () => {
  let epoch = 1
  let resolve
  globalThis.fetch = () => new Promise(done => { resolve = done })
  const state = useContacts({ friends: ref([]), showSettings: ref(true), showError: fail, getEpoch: () => epoch })
  const loading = state.openContacts()
  epoch++
  resolve(response([{ id: 'blocked' }]))
  await loading
  assert.deepEqual(state.blockedUsers.value, [])
})

test('global search preserves cursor pagination and resets independently', async () => {
  const state = useGlobalSearch()
  const requests = []
  globalThis.fetch = async url => {
    requests.push(new URL(url, 'http://localhost'))
    return response({ results: [{ id: requests.length }], has_more: requests.length === 1, next_before_time: 'stamp', next_before_id: 'cursor' })
  }
  state.globalQuery.value = '  消息  '
  await state.searchAll()
  await state.searchAll(true)
  assert.equal(requests[0].searchParams.get('q'), '消息')
  assert.equal(requests[1].searchParams.get('before_id'), 'cursor')
  assert.equal(requests[1].searchParams.get('before_time'), 'stamp')
  assert.equal(state.globalResults.value.length, 2)
  state.closeGlobalSearch()
  assert.deepEqual(state.globalResults.value, [])
  assert.equal(state.globalQuery.value, '')
})

test('thread sending targets the resolved root and synchronizes the conversation', async () => {
  const calls = []
  const synced = []
  const state = useThreadWindow({ active: ref('chat'), getEpoch: () => 0, showError: fail, syncConversation: async id => synced.push(id) })
  globalThis.fetch = async (url, options) => {
    calls.push({ url, options })
    return response({ root: { id: 'root' }, replies: [], reply_count: 0, has_more: false })
  }
  await state.openThread({ id: 'reply', thread_root_id: 'root' })
  state.threadDraft.value = '  回复  '
  await state.sendInThread()
  const send = calls.find(call => call.options.method === 'POST')
  assert.equal(JSON.parse(send.options.body).reply_to_id, 'root')
  assert.equal(JSON.parse(send.options.body).content, '回复')
  assert.deepEqual(synced, ['chat'])
  state.closeThread()
  assert.equal(state.threadDraft.value, '')
  assert.equal(state.threadRoot.value, null)
})

test('history reset cancels requests and invalidates message location through its callback', async () => {
  let signal
  let resolve
  let resets = 0
  globalThis.fetch = (url, options) => { signal = options.signal; return new Promise(done => { resolve = done }) }
  const state = useHistorySearch({ active: ref('chat'), current: ref({ members: [] }), onReset: () => { resets++ } })
  const searching = state.searchMessages()
  state.closeSearch()
  assert.equal(signal.aborted, true)
  resolve(response({ messages: [{ id: 'late' }], has_more: false, next_before_seq: 0 }))
  await searching
  assert.deepEqual(state.searchResults.value, [])
  assert.equal(state.searchBusy.value, false)
  assert.equal(resets, 2)
})

test('composer exposes live IME state to the send and deletion handlers', () => {
  const root = document.querySelector('#editor')
  const state = useComposerUndo({ draftArea: ref(root), composerImages: ref([]), uploadBusy: ref(false), draft: ref('草稿'), updateMentionState() {} })
  state.installComposerUndo(root)
  assert.equal(state.isComposerComposing(), false)
  root.dispatchEvent(new dom.window.Event('compositionstart'))
  assert.equal(state.isComposerComposing(), true)
  root.dispatchEvent(new dom.window.Event('compositionend'))
  assert.equal(state.isComposerComposing(), false)
})
