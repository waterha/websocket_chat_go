import { File, FileSpreadsheet, FileText, Presentation, FileArchive, FileImage, FileAudio, FileVideo, FileCode } from 'lucide-vue-next'
import './file-type-icon.css'

const types = {
  spreadsheet: { icon: FileSpreadsheet, label: '表格文件', badge: 'XLS', color: '#18834b' },
  document: { icon: FileText, label: '文档文件', badge: 'DOC', color: '#2867bd' },
  presentation: { icon: Presentation, label: '演示文稿', badge: 'PPT', color: '#c76325' },
  pdf: { icon: FileText, label: 'PDF 文件', badge: 'PDF', color: '#cf4444' },
  archive: { icon: FileArchive, label: '压缩文件', badge: 'ZIP', color: '#8b60b4' },
  image: { icon: FileImage, label: '图片文件', badge: 'IMG', color: '#39968e' },
  audio: { icon: FileAudio, label: '音频文件', badge: '音频', color: '#a34b94' },
  video: { icon: FileVideo, label: '视频文件', badge: '视频', color: '#6866c4' },
  code: { icon: FileCode, label: '代码文件', badge: '代码', color: '#497d94' },
  text: { icon: FileText, label: '文本文件', badge: 'TXT', color: '#617385' },
  file: { icon: File, label: '文件', badge: 'FILE', color: '#75808c' },
}
const extensions = {
  spreadsheet: 'xls xlsx xlsm xlsb csv ods et', document: 'doc docx docm odt rtf wps',
  presentation: 'ppt pptx pptm odp dps', pdf: 'pdf', archive: 'zip rar 7z tar gz bz2 xz tgz',
  image: 'png jpg jpeg gif webp bmp svg ico avif heic tiff tif', audio: 'mp3 wav ogg m4a flac aac opus',
  video: 'mp4 mov avi mkv webm wmv m4v', code: 'js ts jsx tsx go py java c cpp h cs html css json xml yaml yml sql sh ps1', text: 'txt md log',
}
const byExtension = new Map(Object.entries(extensions).flatMap(([kind, list]) => list.split(' ').map(extension => [extension, kind])))

// Display hints only; never use a filename extension to authorize or execute content.
export function fileType(filename = '') {
  const basename = String(filename || '').trim().split(/[\\/]/).at(-1)
  const extension = basename.includes('.') ? basename.split('.').at(-1).toLowerCase() : ''
  return byExtension.get(extension) || 'file'
}
export const FileTypeIcon = {
  props: { filename: { type: String, default: '' }, size: { type: Number, default: 36 } },
  computed: { kind() { return fileType(this.filename) }, appearance() { return types[this.kind] } },
  template: `<span class="file-type-icon" :class="'file-type-' + kind" :data-file-type="kind" :style="{ color: appearance.color }" role="img" :aria-label="appearance.label" :title="appearance.label"><component :is="appearance.icon" :size="size" aria-hidden="true" /><small aria-hidden="true">{{ appearance.badge }}</small></span>`,
}
