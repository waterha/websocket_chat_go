import test from 'node:test'
import assert from 'node:assert/strict'
import { register } from 'node:module'
import { readFileSync } from 'node:fs'
import { JSDOM } from 'jsdom'
register('../css-hook.mjs', import.meta.url)
const dom = new JSDOM('<!doctype html><html><head></head><body></body></html>')
for (const key of ['window', 'document', 'HTMLElement', 'Element', 'Node', 'SVGElement', 'Text', 'Comment']) globalThis[key] = key === 'window' ? dom.window : key === 'document' ? dom.window.document : dom.window[key]
const { createApp } = await import('vue/dist/vue.esm-bundler.js')
const { messagePresentation, isEmojiOnly } = await import('./message-presentation.js')
const { SavedMessageContent } = await import('./saved-message-content.js')

test('emoji-only detection supports faces, skin tones, ZWJ families, flags and keycaps without swallowing prose or numbers', () => {
  for (const text of ['😀', '😀 😂', '👍🏽', '👨‍👩‍👧‍👦', '🇨🇳', '1️⃣', '❤️\n🥰']) assert.equal(isEmojiOnly(text), true, text)
  for (const text of ['', ' ', '123', '#', '*', '你好😀', '😀!', 'abc', '\u200d', '🏽']) assert.equal(isEmojiOnly(text), false, text)
})
test('only images or only emoji lose their bubble; captions, files, recalls and mixed blocks do not', () => {
  assert.equal(messagePresentation({ kind: 'image', media_id: 'photo', content: '' })['borderless-image'], true)
  assert.equal(messagePresentation({ blocks: [{ type: 'text', text: '\n ' }, { type: 'image', media_id: 'a' }, { type: 'image', media_id: 'b' }] })['borderless-image'], true)
  assert.equal(messagePresentation({ content: '👍🏽' })['borderless-emoji'], true)
  assert.equal(messagePresentation({ blocks: [{ type: 'text', text: '😀' }, { type: 'text', text: '🎉' }] })['borderless-emoji'], true)
  for (const message of [null, { kind: 'image', content: '图片说明' }, { blocks: [{ type: 'image', media_id: 'a' }, { type: 'text', text: '😀' }] }, { kind: 'file', media_id: 'file', content: '😀' }, { kind: 'video', content: '' }, { kind: 'system', content: '😀' }, { content: '😀', recalled_at: 'now' }, { content: 'hello 😀' }]) assert.ok(Object.values(messagePresentation(message)).every(value => !value))
})
test('sent and received images and emoji render without a bubble background even with media theme rules', () => {
  const style = document.createElement('style')
  // Deliberately load normal media styles after these overrides to cover import ordering.
  style.textContent = readFileSync(new URL('./message-presentation.css', import.meta.url), 'utf8') + '\n' + readFileSync(new URL('./styles/message-media.css', import.meta.url), 'utf8')
  document.head.append(style)
  for (const isOwn of [false, true]) for (const message of [
    { kind: 'image', media_id: 'photo', media: { file_name: 'photo.png' }, content: '' },
    { kind: 'user', blocks: [{ type: 'image', media_id: 'photo', file_name: 'photo.png' }] },
    { kind: 'user', content: '😀' },
  ]) {
    const host = document.createElement('div'); document.body.append(host)
    const app = createApp(SavedMessageContent, { favorite: { id: 'saved', is_own: isOwn, message } }); app.mount(host)
    try {
      const bubble = host.querySelector('.bubble'), css = dom.window.getComputedStyle(bubble)
      assert.ok(bubble.classList.contains(message.content === '😀' ? 'borderless-emoji' : 'borderless-image'))
      assert.equal(css.backgroundColor, 'rgba(0, 0, 0, 0)')
      assert.equal(css.padding, '0px')
      assert.equal(css.boxShadow, 'none')
      if (message.media_id) assert.equal(host.querySelector('img').getAttribute('src'), '/api/v1/favorites/saved/media/photo')
    } finally { app.unmount(); host.remove() }
  }
  style.remove()
})
test.after(() => dom.window.close())
