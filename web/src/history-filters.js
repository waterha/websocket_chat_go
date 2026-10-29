// Calendar inputs use the viewer's local timezone. The API receives instants,
// with an inclusive start and an exclusive midnight after the final day.
function midnight(value) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) throw new Error('请选择有效日期')
  const date = new Date(`${value}T00:00:00`)
  const [year, month, day] = value.split('-').map(Number)
  if (!Number.isFinite(date.getTime()) || date.getFullYear() !== year || date.getMonth() + 1 !== month || date.getDate() !== day) {
    throw new Error('请选择有效日期')
  }
  return date
}

export function historySearchParams({ query = '', sender = '', type = 'all', start = '', end = '' } = {}) {
  if ([...query.trim()].length > 100) throw new Error('关键词最多 100 个字符')
  if (!['all', 'images', 'links', 'files'].includes(type)) throw new Error('请选择有效的消息类型')
  if (start && end && start > end) throw new Error('开始日期不能晚于结束日期')
  const params = new URLSearchParams({ q: query.trim(), limit: '30' })
  if (sender) params.set('sender_id', sender)
  if (type !== 'all') params.set('type', type)
  if (start) params.set('from', midnight(start).toISOString())
  if (end) {
    const until = midnight(end)
    until.setDate(until.getDate() + 1)
    params.set('to', until.toISOString())
  }
  return params
}

export function historyMessagePreview(message) {
  if (message.recalled_at) return '消息已撤回'
  if (message.blocks?.length) {
    return message.blocks.map(block => block.type === 'image' ? '[表情/图片]' : block.text || '').join(' ')
  }
  const content = message.content || ''
  if (message.kind === 'image') return `[表情/图片] ${content}`.trim()
  if (message.kind === 'file') return `[文件] ${message.media?.file_name || ''} ${content}`.trim()
  if (message.kind === 'audio') return `[语音] ${content}`.trim()
  if (message.kind === 'video') return `[视频] ${content}`.trim()
  return content
}
