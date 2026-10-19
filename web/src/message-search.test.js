import test from 'node:test'
import assert from 'node:assert/strict'
import { highlightMatches } from './message-search.js'

test('highlights all case-insensitive matches without altering original text', () => {
  const content = 'Hello hello HELLO'
  const parts = highlightMatches(content, ' hello ')
  assert.equal(parts.map(part => part.text).join(''), content)
  assert.deepEqual(parts.filter(part => part.match).map(part => part.text), ['Hello', 'hello', 'HELLO'])
})

test('matches Chinese and emoji keywords', () => {
  assert.deepEqual(highlightMatches('消息搜索：消息', '消息').filter(part => part.match).map(part => part.text), ['消息', '消息'])
  assert.deepEqual(highlightMatches('a😀b😀', '😀').filter(part => part.match).map(part => part.text), ['😀', '😀'])
})

test('treats regex syntax and SQL wildcard characters literally', () => {
  for (const query of ['.*', '[a]', 'a+b', '(x)', '?', '^', '$', '|', '\\', '%_', '{2}']) {
    const content = `before ${query} after`
    const parts = highlightMatches(content, query)
    assert.deepEqual(parts.filter(part => part.match), [{ text: query, match: true }])
    assert.equal(parts.map(part => part.text).join(''), content)
  }
})

test('retains untrusted HTML as text segments', () => {
  const content = '<img src=x onerror=alert(1)>'
  const parts = highlightMatches(content, 'img')
  assert.deepEqual(parts, [{ text: '<', match: false }, { text: 'img', match: true }, { text: ' src=x onerror=alert(1)>', match: false }])
})

test('handles empty or absent matches and preserves newlines', () => {
  assert.deepEqual(highlightMatches('message', '   '), [{ text: 'message', match: false }])
  assert.deepEqual(highlightMatches('message', 'none'), [{ text: 'message', match: false }])
  const parts = highlightMatches('one\ntwo\n', 'two')
  assert.equal(parts.map(part => part.text).join(''), 'one\ntwo\n')
  assert.deepEqual(highlightMatches('', 'x'), [])
})
