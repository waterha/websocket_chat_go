import test from 'node:test'
import assert from 'node:assert/strict'
import { useGlobalSearch } from './use-global-search.js'

const original = { fetch: globalThis.fetch, document: globalThis.document }
const page = (id, more = false) => ({ results: [{ message: { id } }], has_more: more, next_before_time: more ? '2026-01-01T00:00:00Z' : '', next_before_id: more ? id : '' })
const response = data => ({ ok: true, json: async () => ({ data }) })
let requests

test.beforeEach(() => {
  requests = []
  globalThis.document = { cookie: '' }
  // Deliberately do not reject on abort: response parsing may already be in flight.
  globalThis.fetch = (url, options) => new Promise((resolve, reject) => {
    requests.push({ url: new URL(url, 'http://localhost'), signal: options.signal, resolve: data => resolve(response(data)), reject })
  })
})
test.afterEach(() => Object.assign(globalThis, original))

test('repeated submits and load-more calls issue only one request while busy', async () => {
  const state = useGlobalSearch()
  state.globalQuery.value = '  预算  '
  const first = state.searchAll()
  await state.searchAll()
  assert.equal(requests.length, 1)
  assert.equal(requests[0].url.searchParams.get('q'), '预算')
  assert.equal(requests[0].url.searchParams.get('limit'), '20')
  requests[0].resolve(page('first', true))
  await first
  const more = state.searchAll(true)
  await state.searchAll(true)
  assert.equal(requests.length, 2)
  requests[1].resolve(page('second'))
  await more
  assert.deepEqual(state.globalResults.value.map(hit => hit.message.id), ['first', 'second'])
})

test('pagination retains the submitted query and cursor after input is edited', async () => {
  const state = useGlobalSearch()
  state.globalQuery.value = '预算'
  const first = state.searchAll()
  requests[0].resolve(page('first', true))
  await first
  state.globalQuery.value = '新关键词'
  const more = state.searchAll(true)
  assert.equal(requests[1].url.searchParams.get('q'), '预算')
  assert.equal(requests[1].url.searchParams.get('before_id'), 'first')
  assert.equal(requests[1].url.searchParams.get('before_time'), '2026-01-01T00:00:00Z')
  requests[1].resolve(page('second'))
  await more
  const restart = state.searchAll()
  assert.equal(requests[2].url.searchParams.get('q'), '新关键词')
  assert.equal(requests[2].url.searchParams.has('before_id'), false)
  assert.deepEqual(state.globalResults.value, [])
  requests[2].resolve(page('new'))
  await restart
})

test('a new query aborts old work and late responses cannot replace new results', async () => {
  const state = useGlobalSearch()
  state.globalQuery.value = 'old'
  const old = state.searchAll()
  state.globalQuery.value = 'new'
  const current = state.searchAll()
  assert.equal(requests[0].signal.aborted, true)
  requests[1].resolve(page('new'))
  await current
  requests[0].resolve(page('old', true))
  await old
  assert.deepEqual(state.globalResults.value.map(hit => hit.message.id), ['new'])
  assert.equal(state.globalHasMore.value, false)
  assert.equal(state.globalBusy.value, false)
})

test('an obsolete request cannot clear current busy state or show an error', async () => {
  const state = useGlobalSearch()
  state.globalQuery.value = 'old'
  const old = state.searchAll()
  state.globalQuery.value = 'new'
  const current = state.searchAll()
  requests[0].reject(new Error('obsolete error'))
  await old
  assert.equal(state.globalBusy.value, true)
  assert.equal(state.globalError.value, '')
  requests[1].resolve(page('new'))
  await current
})

test('closing aborts search and prevents late results from repopulating state', async () => {
  const state = useGlobalSearch()
  await state.openGlobalSearch()
  state.globalQuery.value = '预算'
  const searching = state.searchAll()
  state.closeGlobalSearch()
  assert.equal(requests[0].signal.aborted, true)
  assert.equal(state.globalBusy.value, false)
  requests[0].resolve(page('late', true))
  await searching
  assert.equal(state.showGlobalSearch.value, false)
  assert.equal(state.globalQuery.value, '')
  assert.deepEqual(state.globalResults.value, [])
  assert.equal(state.globalHasMore.value, false)
  assert.equal(state.globalStarted.value, false)
  assert.deepEqual(state.globalCursor.value, { time: '', id: '' })
})

test('superseded pagination cannot append to a new search', async () => {
  const state = useGlobalSearch()
  state.globalQuery.value = 'old'
  const first = state.searchAll()
  requests[0].resolve(page('first', true))
  await first
  const more = state.searchAll(true)
  state.globalQuery.value = 'new'
  const current = state.searchAll()
  assert.equal(requests[1].signal.aborted, true)
  requests[2].resolve(page('new'))
  await current
  requests[1].resolve(page('late'))
  await more
  assert.deepEqual(state.globalResults.value.map(hit => hit.message.id), ['new'])
})

test('current errors remain visible and pagination can be retried without duplicates', async () => {
  const state = useGlobalSearch()
  state.globalQuery.value = '预算'
  const first = state.searchAll()
  requests[0].resolve(page('first', true))
  await first
  const failed = state.searchAll(true)
  requests[1].reject(new Error('搜索出错'))
  await failed
  assert.equal(state.globalError.value, '搜索出错')
  assert.equal(state.globalBusy.value, false)
  assert.equal(state.globalHasMore.value, true)
  const retry = state.searchAll(true)
  assert.equal(state.globalError.value, '')
  assert.equal(requests[2].url.searchParams.get('before_id'), 'first')
  requests[2].resolve(page('second'))
  await retry
  assert.deepEqual(state.globalResults.value.map(hit => hit.message.id), ['first', 'second'])
  await state.searchAll(true)
  assert.equal(requests.length, 3)
})

test('empty and overlong queries do not send requests and original Unicode limit is retained', async () => {
  const state = useGlobalSearch()
  await state.searchAll()
  state.globalQuery.value = '😀'.repeat(101)
  await state.searchAll()
  assert.equal(requests.length, 0)
  assert.equal(state.globalError.value, '关键词最多 100 个字符')
  state.globalQuery.value = '😀'.repeat(100)
  const searching = state.searchAll()
  assert.equal(requests.length, 1)
  requests[0].resolve(page('allowed'))
  await searching
})
