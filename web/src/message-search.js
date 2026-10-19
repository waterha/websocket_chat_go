// Return text segments, never HTML: message content must remain untrusted text.
export function highlightMatches(content, query) {
  const needle = query.trim()
  if (!needle) return [{ text: content, match: false }]
  const expression = new RegExp(needle.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'giu')
  const parts = []
  let start = 0
  for (const match of content.matchAll(expression)) {
    if (match.index > start) parts.push({ text: content.slice(start, match.index), match: false })
    parts.push({ text: match[0], match: true })
    start = match.index + match[0].length
  }
  if (start < content.length) parts.push({ text: content.slice(start), match: false })
  return parts
}
