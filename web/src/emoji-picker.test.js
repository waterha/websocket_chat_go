import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { JSDOM } from 'jsdom'
import { emojiCategories, originalEmojiCategories, installEmojiPicker } from './emoji-picker.js'

const settle = () => new Promise(resolve => setImmediate(resolve))
const chooseFile = (dom, picker, file) => {
  const input = picker.querySelector('input[type="file"]')
  Object.defineProperty(input, 'files', { configurable: true, value: file ? [file] : [] })
  input.dispatchEvent(new dom.window.Event('change', { bubbles: true }))
  return input
}
const contextMenu = (dom, picker) => {
  const event = new dom.window.MouseEvent('contextmenu', { bubbles: true, cancelable: true, button: 2 })
  picker.querySelector('.emoji-sticker-option').dispatchEvent(event)
  assert.equal(event.defaultPrevented, true)
  return picker.querySelector('[role="menuitem"]')
}
const deferred = () => {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

async function withPicker(options, run) {
  const dom = new JSDOM('<form class="composer"><div contenteditable="true">AB</div><div class="composer-toolbar"></div></form>')
  const originalDocument = globalThis.document
  const originalWindow = globalThis.window
  globalThis.document = dom.window.document
  globalThis.window = dom.window
  const editor = document.querySelector('[contenteditable]')
  try {
    installEmojiPicker(editor, options)
    const picker = document.querySelector('.emoji-picker')
    await run({ dom, editor, picker, toggle: picker.querySelector('.emoji-picker-toggle'),
      tabs: () => [...picker.querySelectorAll('[role="tab"]')],
      custom: () => picker.querySelectorAll('[role="tab"]')[1].click() })
  } finally {
    globalThis.document = originalDocument
    globalThis.window = originalWindow
    dom.window.close()
  }
}

// jsdom does not synchronously fetch CSS imports from an inline style.
function readStyles(url) {
  return readFileSync(url, 'utf8').replace(/@import\s+["']([^"']+)["'];/g, (_, path) => readStyles(new URL(path, url)))
}

test('emoji picker stays hidden until opened and inserts an emoji at the saved caret', () => {
  const dom = new JSDOM('<form class="composer"><div contenteditable="true">AB</div><div><span>Send</span></div></form>')
  const style = dom.window.document.createElement('style')
  style.textContent = readStyles(new URL('./style.css', import.meta.url))
  dom.window.document.head.append(style)
  const originalDocument = globalThis.document
  const originalWindow = globalThis.window
  globalThis.document = dom.window.document
  globalThis.window = dom.window
  const editor = dom.window.document.querySelector('[contenteditable]')
  const range = dom.window.document.createRange()
  range.setStart(editor.firstChild, 1)
  range.collapse(true)
  dom.window.getSelection().addRange(range)
  let inputEvents = 0
  editor.addEventListener('input', () => inputEvents++)

  try {
    installEmojiPicker(editor)
    const picker = dom.window.document.querySelector('.emoji-picker')
    const toggle = picker.querySelector('.emoji-picker-toggle')
    const panel = picker.querySelector('.emoji-picker-panel')
    assert.equal(toggle.getAttribute('aria-expanded'), 'false')
    assert.equal(dom.window.getComputedStyle(panel).display, 'none')
    const down = new dom.window.MouseEvent('pointerdown', { bubbles: true, cancelable: true })
    toggle.dispatchEvent(down)
    assert.equal(down.defaultPrevented, true)
    toggle.click()
    assert.equal(toggle.getAttribute('aria-expanded'), 'true')
    assert.equal(dom.window.getComputedStyle(panel).display, 'block')
    const panelStyle = dom.window.getComputedStyle(panel)
    assert.equal(dom.window.getComputedStyle(picker).position, 'relative')
    assert.equal(panelStyle.top, 'auto')
    assert.equal(panelStyle.bottom, 'calc(100% + 8px)')
    assert.equal(panelStyle.left, '0px')
    // Both tabs use the same upward-opening panel.
    picker.querySelectorAll('[role="tab"]')[1].click()
    assert.equal(dom.window.getComputedStyle(panel).bottom, 'calc(100% + 8px)')
    picker.querySelectorAll('[role="tab"]')[0].click()
    toggle.click()
    assert.equal(toggle.getAttribute('aria-expanded'), 'false')
    assert.equal(dom.window.getComputedStyle(panel).display, 'none')
    toggle.click()
    picker.querySelector('.emoji-option').click()

    assert.equal(editor.textContent, 'A😀B')
    assert.equal(inputEvents, 1)
    assert.equal(picker.classList.contains('open'), false)
    assert.equal(toggle.getAttribute('aria-expanded'), 'false')
    assert.equal(dom.window.getComputedStyle(panel).display, 'none')
  } finally {
    globalThis.document = originalDocument
    globalThis.window = originalWindow
    dom.window.close()
  }
})

test('two tabs merge every original emoji exactly once and preserve the caret on tabs', async () => {
  await withPicker({}, async ({ dom, picker, toggle, tabs, editor }) => {
    assert.deepEqual(tabs().map(tab => tab.getAttribute('aria-label')), ['emoji表情', '表情'])
    assert.deepEqual(tabs().map(tab => tab.title), ['emoji表情', '表情'])
    assert.ok(tabs().every(tab => tab.textContent === '' && tab.querySelector('svg[aria-hidden="true"]')))
    assert.ok(tabs()[0].querySelector('circle'))
    assert.ok(tabs()[1].querySelector('path'))
    const expected = [...new Set(originalEmojiCategories.flatMap(category => category.emojis))]
    assert.equal(expected.length, 149)
    assert.equal(emojiCategories.length, 1)
    assert.deepEqual([...picker.querySelectorAll('.emoji-option')].map(button => button.textContent), expected)
    toggle.click()
    const down = new dom.window.MouseEvent('pointerdown', { bubbles: true, cancelable: true })
    tabs()[1].dispatchEvent(down)
    assert.equal(down.defaultPrevented, true)
    editor.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    assert.equal(toggle.getAttribute('aria-expanded'), 'false')
  })
})

test('private tab loads only when opened, keeps state on reinstall and reloads on reopen', async () => {
  let calls = 0
  const options = { getStickers: async () => { calls++; return [] } }
  await withPicker(options, async ({ editor, picker, toggle, custom, tabs }) => {
    assert.equal(calls, 0)
    toggle.click()
    assert.equal(calls, 0)
    custom()
    assert.match(picker.textContent, /加载中/)
    await settle()
    assert.equal(calls, 1)
    assert.match(picker.textContent, /还没有保存的表情/)
    installEmojiPicker(editor, options)
    assert.equal(calls, 1)
    assert.equal(tabs()[1].getAttribute('aria-selected'), 'true')
    assert.equal(toggle.getAttribute('aria-expanded'), 'true')
    toggle.click()
    toggle.click()
    await settle()
    assert.equal(calls, 2)
    tabs()[0].click()
    custom()
    await settle()
    assert.equal(calls, 3)
  })
})

test('private images are lazy and both image and emoji use the async callback with duplicate prevention', async () => {
  const image = { id: 'private /?#', kind: 'image', file_name: 'cat.png' }
  const emoji = { id: 'emoji', kind: 'emoji', text: '🥰' }
  const selected = []
  let pending = deferred()
  await withPicker({ getStickers: async () => [image, emoji], onSelectSticker: sticker => {
    selected.push(sticker)
    return pending.promise
  } }, async ({ picker, toggle, custom, editor }) => {
    toggle.click()
    custom()
    await settle()
    const img = picker.querySelector('img')
    assert.equal(img.getAttribute('src'), '/api/v1/stickers/private%20%2F%3F%23/media')
    assert.equal(img.getAttribute('loading'), 'lazy')
    const first = picker.querySelector('.emoji-sticker-option')
    first.click()
    first.click()
    assert.deepEqual(selected, [image])
    assert.equal(first.disabled, true)
    pending.resolve()
    await settle()
    assert.equal(toggle.getAttribute('aria-expanded'), 'false')
    pending = deferred()
    toggle.click()
    await settle()
    picker.querySelectorAll('.emoji-sticker-option')[1].click()
    assert.deepEqual(selected, [image, emoji])
    assert.equal(editor.textContent, 'AB')
    pending.resolve()
    await settle()
  })
})

test('failed private fetch reports an error and supports retry', async () => {
  const failure = new Error('offline')
  let calls = 0
  const errors = []
  await withPicker({ getStickers: async () => {
    if (++calls === 1) throw failure
    return [{ id: 'ok', kind: 'emoji', text: '✅' }]
  }, onError: error => errors.push(error) }, async ({ picker, toggle, custom }) => {
    toggle.click()
    custom()
    await settle()
    assert.deepEqual(errors, [failure])
    assert.match(picker.textContent, /加载表情失败/)
    picker.querySelector('.emoji-picker-retry').click()
    await settle()
    assert.equal(calls, 2)
    assert.equal(picker.querySelector('.emoji-sticker-option').textContent, '✅')
  })
})

test('account changes discard stale responses and existing private choices', async () => {
  let account = 'alice'
  let pending = deferred()
  let selected = 0
  const options = { getUserID: () => account, getStickers: () => pending.promise,
    onSelectSticker: async () => { selected++ } }
  await withPicker(options, async ({ editor, picker, toggle, custom }) => {
    toggle.click()
    custom()
    account = 'bob'
    pending.resolve([{ id: 'alice-secret', kind: 'emoji', text: '🔒' }])
    await settle()
    assert.equal(picker.querySelector('.emoji-sticker-option'), null)
    installEmojiPicker(editor, options)
    assert.equal(toggle.getAttribute('aria-expanded'), 'false')
    pending = deferred()
    toggle.click()
    custom()
    pending.resolve([{ id: 'bob-secret', kind: 'emoji', text: '🔑' }])
    await settle()
    const oldChoice = picker.querySelector('.emoji-sticker-option')
    account = 'charlie'
    oldChoice.click()
    assert.equal(selected, 0)
    installEmojiPicker(editor, options)
    assert.equal(picker.querySelector('.emoji-sticker-option'), null)
  })
})

test('closed, superseded and detached requests never populate private results', async () => {
  let pending = deferred()
  await withPicker({ getStickers: () => pending.promise }, async ({ editor, picker, toggle, custom, tabs }) => {
    toggle.click()
    custom()
    const first = pending
    tabs()[0].click()
    pending = deferred()
    custom()
    first.resolve([{ id: 'old', kind: 'emoji', text: '❌' }])
    await settle()
    assert.equal(picker.querySelector('.emoji-sticker-option'), null)
    editor.remove()
    pending.resolve([{ id: 'detached', kind: 'emoji', text: '❌' }])
    await settle()
    assert.equal(picker.querySelector('.emoji-sticker-option'), null)
  })
})

test('old-account pending selections neither block nor unlock the next account', async () => {
  let account = 'alice'
  const selections = []
  const errors = []
  const options = { getUserID: () => account,
    getStickers: async () => [{ id: account, kind: 'emoji', text: '😀' }],
    onSelectSticker: () => { const pending = deferred(); selections.push(pending); return pending.promise },
    onError: error => errors.push(error) }
  await withPicker(options, async ({ editor, picker, toggle, custom }) => {
    toggle.click()
    custom()
    await settle()
    picker.querySelector('.emoji-sticker-option').click()
    account = 'bob'
    installEmojiPicker(editor, options)
    toggle.click()
    custom()
    await settle()
    const bob = picker.querySelector('.emoji-sticker-option')
    assert.equal(bob.disabled, false)
    bob.click()
    assert.equal(selections.length, 2)
    selections[0].reject(new Error('stale upload'))
    await settle()
    assert.equal(bob.disabled, true)
    assert.equal(errors.length, 0)
    selections[1].resolve()
    await settle()
    assert.equal(toggle.getAttribute('aria-expanded'), 'false')
  })
})

test('both tabs share an area and upload is the first equal-size cell before stickers in added order', async () => {
  let items = [
    { id: 'later', kind: 'emoji', text: '🎉', created_at: '2026-11-10T02:00:00Z' },
    { id: 'first', kind: 'emoji', text: '😀', created_at: '2026-11-10T01:00:00Z' },
  ]
  await withPicker({ getStickers: async () => items, uploadSticker: async () => {
    items = [{ id: 'new', kind: 'emoji', text: '❤️', created_at: '2026-11-10T03:00:00Z' }, ...items]
  } }, async ({ dom, picker, toggle, custom }) => {
    const style = document.createElement('style')
    style.textContent = readStyles(new URL('./style.css', import.meta.url))
    document.head.append(style)
    const grid = picker.querySelector('.emoji-picker-grid')
    const before = dom.window.getComputedStyle(grid)
    assert.equal(before.width, '293px'); assert.equal(before.height, '246px')
    toggle.click(); custom(); await settle()
    const plus = picker.querySelector('.emoji-sticker-upload')
    assert.equal(grid.firstElementChild, plus)
    assert.deepEqual([...grid.children].map(node => node.textContent), ['+', '😀', '🎉'])
    assert.equal(dom.window.getComputedStyle(grid).width, before.width)
    assert.equal(dom.window.getComputedStyle(grid).height, before.height)
    const cell = dom.window.getComputedStyle(grid.querySelector('.emoji-sticker-option'))
    const uploadStyle = dom.window.getComputedStyle(plus)
    assert.equal(cell.width, uploadStyle.width)
    assert.equal(cell.aspectRatio, uploadStyle.aspectRatio)
    plus.click(); chooseFile(dom, picker, new dom.window.File(['x'], 'new.png', { type: 'image/png' })); await settle()
    assert.deepEqual([...grid.children].map(node => node.textContent), ['+', '😀', '🎉', '❤️'])
    assert.equal(grid.firstElementChild, plus)
  })
})

test('private upload stays in the first grid cell while loading, empty or failed and never auto-sends', async () => {
  const pending = deferred()
  const saved = { id: 'new', kind: 'image', file_name: 'cat.webp' }
  let fetches = 0
  let sends = 0
  const uploaded = []
  await withPicker({ getStickers: async () => {
    fetches++
    if (fetches === 1) return pending.promise
    return [saved]
  }, uploadSticker: file => { uploaded.push(file); return Promise.resolve(saved) },
  onSelectSticker: () => { sends++ } }, async ({ dom, picker, toggle, custom, tabs }) => {
    toggle.click()
    custom()
    const plus = picker.querySelector('.emoji-sticker-upload')
    assert.equal(plus.parentElement, picker.querySelector('.emoji-picker-grid'))
    assert.equal(plus.parentElement.firstElementChild, plus)
    assert.equal(plus.disabled, false)
    assert.match(picker.textContent, /加载中/)
    pending.resolve([])
    await settle()
    assert.match(picker.textContent, /还没有保存/)
    plus.click()
    chooseFile(dom, picker, null)
    assert.equal(uploaded.length, 0)
    const file = new dom.window.File(['image'], 'cat.webp', { type: 'image/webp' })
    plus.click()
    const input = chooseFile(dom, picker, file)
    assert.equal(input.value, '')
    assert.equal(plus.disabled, true)
    await settle()
    assert.deepEqual(uploaded, [file])
    assert.equal(fetches, 2)
    assert.equal(sends, 0)
    assert.equal(tabs()[1].getAttribute('aria-selected'), 'true')
    assert.equal(toggle.getAttribute('aria-expanded'), 'true')
    assert.equal(picker.querySelector('img').alt, 'cat.webp')
    plus.click()
    chooseFile(dom, picker, file)
    await settle()
    assert.deepEqual(uploaded, [file, file])
  })
})

test('upload failures call onError, release busy state and allow the same file retry', async () => {
  const failure = new Error('upload failed')
  const pending = deferred()
  const errors = []
  let uploads = 0
  await withPicker({ getStickers: async () => { throw new Error('fetch failed') },
    uploadSticker: () => { uploads++; return pending.promise },
    onError: error => errors.push(error) }, async ({ dom, picker, toggle, custom }) => {
    toggle.click()
    custom()
    await settle()
    const plus = picker.querySelector('.emoji-sticker-upload')
    assert.equal(plus.disabled, false)
    assert.match(picker.textContent, /加载表情失败/)
    const file = new dom.window.File(['x'], 'x.png', { type: 'image/png' })
    plus.click()
    chooseFile(dom, picker, file)
    chooseFile(dom, picker, file)
    assert.equal(uploads, 1)
    pending.reject(failure)
    await settle()
    assert.equal(errors.at(-1), failure)
    assert.equal(plus.disabled, false)
    assert.equal(toggle.getAttribute('aria-expanded'), 'true')
    plus.click()
    chooseFile(dom, picker, file)
    await settle()
    assert.equal(uploads, 2)
  })
})

test('right click only deletes through its menu, prevents duplicate deletes and refreshes', async () => {
  const sticker = { id: 'one', kind: 'emoji', text: '😀' }
  const pending = deferred()
  const removed = []
  let sends = 0
  let fetches = 0
  await withPicker({ getStickers: async () => ++fetches === 1 ? [sticker] : [],
    removeSticker: value => { removed.push(value); return pending.promise },
    onSelectSticker: () => { sends++ } }, async ({ dom, picker, toggle, custom, tabs }) => {
    dom.window.confirm = () => { throw new Error('confirm must not be used') }
    dom.window.prompt = () => { throw new Error('prompt must not be used') }
    toggle.click()
    custom()
    await settle()
    const remove = contextMenu(dom, picker)
    assert.equal(remove.textContent, '删除表情')
    assert.equal(picker.querySelectorAll('[role="menuitem"]').length, 1)
    assert.equal(sends, 0)
    remove.click()
    remove.click()
    assert.deepEqual(removed, [sticker])
    assert.match(remove.textContent, /删除中/)
    assert.equal(remove.disabled, true)
    assert.equal(picker.querySelector('.emoji-sticker-upload').disabled, true)
    picker.querySelector('.emoji-sticker-option').click()
    assert.equal(sends, 0)
    pending.resolve()
    await settle()
    assert.equal(fetches, 2)
    assert.equal(picker.querySelector('[role="menu"]'), null)
    assert.equal(picker.querySelector('.emoji-sticker-option'), null)
    assert.equal(tabs()[1].getAttribute('aria-selected'), 'true')
    assert.equal(toggle.getAttribute('aria-expanded'), 'true')
  })
})

test('failed delete reports error, keeps retry menu, and Esc or outside closes just the menu', async () => {
  const errors = []
  let deletes = 0
  await withPicker({ getStickers: async () => [{ id: 'one', kind: 'emoji', text: '😀' }],
    removeSticker: async () => { if (++deletes < 3) throw new Error('delete failed') },
    onError: error => errors.push(error) }, async ({ dom, picker, toggle, custom }) => {
    toggle.click()
    custom()
    await settle()
    let remove = contextMenu(dom, picker)
    remove.click()
    await settle()
    assert.equal(errors.length, 1)
    assert.equal(remove.disabled, false)
    assert.match(remove.textContent, /失败.*重试/)
    remove.click()
    await settle()
    assert.equal(deletes, 2)
    remove.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    assert.equal(picker.querySelector('[role="menu"]'), null)
    assert.equal(toggle.getAttribute('aria-expanded'), 'true')
    contextMenu(dom, picker)
    dom.window.document.body.dispatchEvent(new dom.window.MouseEvent('pointerdown', { bubbles: true }))
    assert.equal(picker.querySelector('[role="menu"]'), null)
    assert.equal(toggle.getAttribute('aria-expanded'), 'true')
    remove = contextMenu(dom, picker)
    remove.click()
    await settle()
    assert.equal(deletes, 3)
    assert.equal(picker.querySelector('[role="menu"]'), null)
  })
})

test('old-account upload completion cannot refresh or unlock a new-account operation', async () => {
  let account = 'alice'
  const pending = [deferred(), deferred()]
  let uploads = 0
  let fetches = 0
  const errors = []
  const options = { getUserID: () => account,
    getStickers: async () => { fetches++; return [] },
    uploadSticker: () => pending[uploads++].promise, onError: error => errors.push(error) }
  await withPicker(options, async ({ dom, editor, picker, toggle, custom }) => {
    const file = new dom.window.File(['x'], 'x.png', { type: 'image/png' })
    toggle.click()
    custom()
    await settle()
    let plus = picker.querySelector('.emoji-sticker-upload')
    plus.click()
    chooseFile(dom, picker, file)
    account = 'bob'
    installEmojiPicker(editor, options)
    toggle.click()
    custom()
    await settle()
    plus = picker.querySelector('.emoji-sticker-upload')
    assert.equal(plus.disabled, false)
    plus.click()
    chooseFile(dom, picker, file)
    const previousFetches = fetches
    pending[0].reject(new Error('alice stale'))
    await settle()
    assert.equal(errors.length, 0)
    assert.equal(fetches, previousFetches)
    assert.equal(plus.disabled, true)
    pending[1].resolve({ id: 'bob' })
    await settle()
    assert.equal(fetches, previousFetches + 1)
    assert.equal(plus.disabled, false)
  })
})

test('stale file chooser and detached upload never mutate or refresh another library', async () => {
  let account = 'alice'
  let uploads = 0
  let fetches = 0
  const pending = deferred()
  const errors = []
  const options = { getUserID: () => account, getStickers: async () => { fetches++; return [] },
    uploadSticker: () => { uploads++; return pending.promise }, onError: error => errors.push(error) }
  await withPicker(options, async ({ dom, editor, picker, toggle, custom }) => {
    const file = new dom.window.File(['x'], 'x.png', { type: 'image/png' })
    toggle.click()
    custom()
    await settle()
    const plus = picker.querySelector('.emoji-sticker-upload')
    plus.click()
    account = 'bob'
    chooseFile(dom, picker, file)
    assert.equal(uploads, 0)
    installEmojiPicker(editor, options)
    toggle.click()
    custom()
    await settle()
    plus.click()
    chooseFile(dom, picker, file)
    const previousFetches = fetches
    editor.remove()
    pending.reject(new Error('detached'))
    await settle()
    assert.equal(errors.length, 0)
    assert.equal(fetches, previousFetches)
  })
})

test('old-account delete failure and detached menus are cleaned up without affecting the next account', async () => {
  let account = 'alice'
  const pending = deferred()
  let fetches = 0
  const errors = []
  const options = { getUserID: () => account,
    getStickers: async () => { fetches++; return [{ id: account, kind: 'emoji', text: '😀' }] },
    removeSticker: () => pending.promise, onError: error => errors.push(error) }
  await withPicker(options, async ({ dom, editor, picker, toggle, custom }) => {
    toggle.click()
    custom()
    await settle()
    contextMenu(dom, picker).click()
    account = 'bob'
    installEmojiPicker(editor, options)
    toggle.click()
    custom()
    await settle()
    const previousFetches = fetches
    pending.reject(new Error('stale delete'))
    await settle()
    assert.equal(errors.length, 0)
    assert.equal(fetches, previousFetches)
    assert.equal(picker.querySelector('.emoji-sticker-option').disabled, false)
    contextMenu(dom, picker)
    editor.closest('.composer').remove()
    await settle()
    assert.equal(picker.querySelector('[role="menu"]'), null)
    const escape = new dom.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
    dom.window.document.body.dispatchEvent(escape)
    assert.equal(escape.defaultPrevented, false)
  })
})

test('selection failure reports error and releases the busy state for retry', async () => {
  const errors = []
  let calls = 0
  await withPicker({ getStickers: async () => [{ id: 'one', kind: 'emoji', text: '😀' }],
    onSelectSticker: async () => { if (++calls === 1) throw new Error('upload failed') },
    onError: error => errors.push(error) }, async ({ picker, toggle, custom }) => {
    toggle.click()
    custom()
    await settle()
    const option = picker.querySelector('.emoji-sticker-option')
    option.click()
    await settle()
    assert.equal(errors.length, 1)
    assert.equal(option.disabled, false)
    assert.equal(toggle.getAttribute('aria-expanded'), 'true')
    option.click()
    await settle()
    assert.equal(calls, 2)
    assert.equal(toggle.getAttribute('aria-expanded'), 'false')
  })
})
