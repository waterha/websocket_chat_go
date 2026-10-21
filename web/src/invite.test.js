import test from 'node:test'
import assert from 'node:assert/strict'
import {
  availableInitials, filterCandidates, groupByInitial, initialOf,
  keepSelectable, OTHER_INITIAL, selectAllCandidates, selectionSummary, toggleSelection,
} from './invite.js'

const person = (id, name, email = `${id}@test`) => ({ id, name, email })

test('a name is grouped by its first Latin letter, everything else separately', () => {
  assert.equal(initialOf('alice'), 'A')
  assert.equal(initialOf('Bob'), 'B')
  assert.equal(initialOf('  zoe'), 'Z')
  // Diacritics fall back to the base letter rather than creating a new bucket.
  assert.equal(initialOf('Élodie'), 'E')
  // Non-Latin names are grouped explicitly instead of being guessed by pinyin.
  assert.equal(initialOf('张伟'), OTHER_INITIAL)
  assert.equal(initialOf('李娜'), OTHER_INITIAL)
  assert.equal(initialOf('123'), OTHER_INITIAL)
  assert.equal(initialOf(''), OTHER_INITIAL)
  assert.equal(initialOf(undefined), OTHER_INITIAL)
})

test('groups are ordered A–Z with the non-Latin bucket last', () => {
  const people = [person('1', '张伟'), person('2', 'bob'), person('3', 'Alice'), person('4', '李娜'), person('5', 'adam')]
  const groups = groupByInitial(people)
  assert.deepEqual(availableInitials(groups), ['A', 'B', OTHER_INITIAL])
  assert.deepEqual(groups[0].people.map(entry => entry.name), ['adam', 'Alice'])
  // Within the non-Latin bucket the names are still sorted with the locale
  // collator, which orders Chinese by pinyin: 李 (li) before 张 (zhang).
  const bucket = groups[2].people.map(entry => entry.name)
  assert.deepEqual(bucket.slice().sort((left, right) => left.localeCompare(right, 'zh-CN')), bucket)
  assert.deepEqual(bucket.slice().sort(), ['张伟', '李娜'])
})

test('search and initial apply together', () => {
  const people = [person('1', 'alice'), person('2', 'albert'), person('3', 'bob'), person('4', '张伟')]
  assert.deepEqual(filterCandidates(people, 'al').map(entry => entry.id), ['1', '2'])
  assert.deepEqual(filterCandidates(people, 'al', 'A').map(entry => entry.id), ['1', '2'])
  // A specific initial narrows even when the search matches another bucket.
  assert.deepEqual(filterCandidates(people, '', 'B').map(entry => entry.id), ['3'])
  // The non-Latin bucket can be selected explicitly.
  assert.deepEqual(filterCandidates(people, '', OTHER_INITIAL).map(entry => entry.id), ['4'])
  // Email is searchable too.
  assert.deepEqual(filterCandidates(people, '2@test').map(entry => entry.id), ['2'])
  assert.deepEqual(filterCandidates(people, '', '').length, 4)
})

test('selection toggles without mutating the source array', () => {
  const selected = ['a']
  const added = toggleSelection(selected, 'b')
  assert.deepEqual(added, ['a', 'b'])
  assert.deepEqual(selected, ['a'])
  assert.deepEqual(toggleSelection(added, 'a'), ['b'])
  assert.deepEqual(toggleSelection(undefined, 'x'), ['x'])
})

test('selections hidden by a filter survive, but people who became members do not', () => {
  const candidates = [person('a', 'alice'), person('b', 'bob')]
  // 'c' is not a candidate any more, so it is dropped.
  assert.deepEqual(keepSelectable(['a', 'c'], candidates), ['a'])
  // Nothing selected is fine.
  assert.deepEqual(keepSelectable([], candidates), [])
})

test('select all respects the remaining room under the cap', () => {
  const visible = [person('a', 'alice'), person('b', 'bob'), person('c', 'carol')]
  assert.deepEqual(selectAllCandidates([], visible, 997, 1000), ['a', 'b', 'c'])
  // Only two places left, so only two people are added.
  assert.deepEqual(selectAllCandidates([], visible, 998, 1000), ['a', 'b'])
  // Someone already selected is not added twice.
  assert.deepEqual(selectAllCandidates(['c'], visible, 997, 1000), ['c', 'a', 'b'])
  // A full group adds nobody.
  assert.deepEqual(selectAllCandidates([], visible, 1000, 1000), [])
})

test('the button label states the count and the cap', () => {
  assert.deepEqual(selectionSummary([], 10, 1000), { label: '邀请加入群聊', disabled: true, count: 0, remaining: 990 })
  assert.deepEqual(selectionSummary(['a'], 10, 1000), { label: '邀请 1 人加入群聊', disabled: false, count: 1, remaining: 990 })
  assert.deepEqual(selectionSummary(['a', 'b'], 10, 1000), { label: '邀请 2 人加入群聊', disabled: false, count: 2, remaining: 990 })
  // Selecting more than the room left blocks the button with an explanation.
  assert.deepEqual(selectionSummary(['a', 'b'], 999, 1000), { label: '最多还能邀请 1 人', disabled: true, count: 2, remaining: 1 })
})
