import test from 'node:test'
import assert from 'node:assert/strict'
import { historyMessagePreview, historySearchParams } from './history-filters.js'

test('history filters combine without requiring a keyword', () => {
  const params = historySearchParams({ sender: 'member-1', type: 'images', start: '2026-10-01', end: '2026-10-29' })
  assert.equal(params.get('q'), '')
  assert.equal(params.get('sender_id'), 'member-1')
  assert.equal(params.get('type'), 'images')
  assert.equal(params.get('from'), new Date(2026, 9, 1).toISOString())
  assert.equal(params.get('to'), new Date(2026, 9, 30).toISOString())
  assert.equal(params.get('limit'), '30')
})

test('history end date includes its whole local calendar day', () => {
  const params = historySearchParams({ start: '2026-10-29', end: '2026-10-29' })
  assert.equal(new Date(params.get('from')).getDate(), 29)
  assert.equal(new Date(params.get('to')).getDate(), 30)
  assert.equal(new Date(params.get('to')).getHours(), 0)
})

test('history filters validate dates and ranges and preserve literal keywords', () => {
  assert.throws(() => historySearchParams({ start: '2026-02-30' }), /有效日期/)
  assert.throws(() => historySearchParams({ end: 'not-a-date' }), /有效日期/)
  assert.throws(() => historySearchParams({ start: '2026-10-30', end: '2026-10-29' }), /开始日期/)
  assert.throws(() => historySearchParams({ type: 'unknown' }), /消息类型/)
  assert.throws(() => historySearchParams({ query: '中'.repeat(101) }), /100/)
  const params = historySearchParams({ query: '  %_hello  ' })
  assert.equal(params.get('q'), '%_hello')
  assert.equal(params.has('type'), false)
  assert.equal(historySearchParams().get('q'), '')
})

test('history previews identify images, mixed blocks and filenames safely', () => {
  assert.equal(historyMessagePreview({ kind: 'image', content: '' }), '[表情/图片]')
  assert.match(historyMessagePreview({ kind: 'file', media: { file_name: 'report.pdf' } }), /report.pdf/)
  assert.equal(historyMessagePreview({ blocks: [{ type: 'text', text: 'hello' }, { type: 'image' }] }), 'hello [表情/图片]')
  assert.equal(historyMessagePreview({ content: 'secret', recalled_at: 'now' }), '消息已撤回')
})
