import test from 'node:test'
import assert from 'node:assert/strict'
import { compile } from '@vue/compiler-dom'
import { template } from './template.js'

// The component template is a runtime-compiled string, so a successful build
// only proves it parses as JavaScript. This compiles it with the real Vue
// compiler, which is what the browser does at runtime, so a malformed tag or
// directive fails here instead of at first render.

const problems = []
compile(template, { mode: 'function', onError: error => problems.push(error) })

test('the component template compiles without errors', () => {
  assert.deepEqual(problems.map(problem => String(problem.message || problem)), [])
})

test('the template still contains the panels the client relies on', () => {
  // A few structural anchors: if one is renamed or dropped, the matching logic
  // in main.js would silently stop binding.
  for (const anchor of ['v-if="!me"', 'class="layout"', 'ref="stream"', 'class="composer"', 'class="sidebar-footer"', 'ref="mediaInput"', 'ref="draftArea"']) {
    assert.ok(template.includes(anchor), `template is missing ${anchor}`)
  }
})

test('the template was extracted intact rather than truncated', () => {
  assert.ok(template.length > 25000, `template looks truncated: ${template.length} chars`)
  assert.ok(template.trimEnd().endsWith('</div>'), 'template should end with a closing element')
})
