import test from 'node:test'
import assert from 'node:assert/strict'
import { register } from 'node:module'
import { JSDOM } from 'jsdom'
register('../css-hook.mjs', import.meta.url)
const dom = new JSDOM('<!doctype html><html><body></body></html>')
for (const key of ['window', 'document', 'HTMLElement', 'Element', 'Node', 'SVGElement', 'Text', 'Comment']) globalThis[key] = key === 'window' ? dom.window : key === 'document' ? dom.window.document : dom.window[key]
const { createApp, reactive, nextTick } = await import('vue/dist/vue.esm-bundler.js')
const { fileType, FileTypeIcon } = await import('./file-type-icon.js')
const { SavedMessageContent } = await import('./saved-message-content.js')
const composer = (await import('./templates/message-composer.js')).default
const stream = (await import('./templates/message-stream.js')).default

test('file types handle Office formats, uppercase extensions, paths and unknown files', () => {
  for (const name of ['预算.xlsx', '报表.XLS', 'data.csv', 'macros.xlsm', '表格.ods']) assert.equal(fileType(name), 'spreadsheet')
  for (const [name, kind] of [['文档.docx', 'document'], ['讲义.pptx', 'presentation'], ['说明.PDF', 'pdf'], ['backup.tar.gz', 'archive'], ['a.png', 'image'], ['a.mp3', 'audio'], ['a.mp4', 'video'], ['a.go', 'code'], ['a.txt', 'text'], ['README', 'file'], ['xlsx', 'file'], ['a.xlsx.exe', 'file'], ['', 'file']]) assert.equal(fileType(name), kind)
  assert.equal(fileType('C:\\folder.with.dot\\资料.XLSX'), 'spreadsheet')
  assert.equal(fileType('/folder.xlsx/README'), 'file')
  assert.equal(fileType(null), 'file')
})

test('icons show a real table glyph and accessible file type and react to a changed filename', async () => {
  const host = document.createElement('div'), state = reactive({ name: '年度预算.xlsx' })
  const app = createApp({ components: { FileTypeIcon }, setup: () => ({ state }), template: '<FileTypeIcon :filename="state.name" />' })
  app.mount(host)
  try {
    assert.ok(host.querySelector('[data-file-type="spreadsheet"] svg.lucide-file-spreadsheet'))
    assert.equal(host.querySelector('[role="img"]').getAttribute('aria-label'), '表格文件')
    state.name = '合同.pdf'; await nextTick()
    assert.equal(host.querySelector('[data-file-type="pdf"] small').textContent, 'PDF')
    state.name = '<script>.unknown'; await nextTick()
    assert.ok(host.querySelector('[data-file-type="file"]'))
    assert.equal(host.querySelector('script'), null)
  } finally { app.unmount() }
})

test('saved file cards use type icons without changing private download links', () => {
  const host = document.createElement('div')
  const app = createApp(SavedMessageContent, { favorite: { id: 'fav', message: { kind: 'file', media_id: 'media', media: { file_name: '预算.xlsx', byte_size: 12 } } } })
  app.mount(host)
  try {
    assert.ok(host.querySelector('.media-file [data-file-type="spreadsheet"]'))
    assert.equal(host.querySelector('.media-file').getAttribute('href'), '/api/v1/favorites/fav/media/media')
    assert.equal(host.querySelector('.media-file').getAttribute('download'), '预算.xlsx')
  } finally { app.unmount() }
})

test('upload previews and sent messages bind icons to the actual attachment filename', () => {
  assert.match(composer, /<FileTypeIcon :filename="attachment.file_name"/)
  assert.match(stream, /<FileTypeIcon :filename="item.media\?\.file_name/)
  assert.match(stream, /<FileTypeIcon :filename="segment.fileName"/)
})
test.after(() => dom.window.close())
