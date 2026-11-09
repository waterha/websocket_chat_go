// Match complete emoji sequences, not ordinary digits, punctuation or mixed prose.
const emojiSequence = /^(?:\p{Extended_Pictographic}(?:\uFE0F|\p{Emoji_Modifier})?(?:\u200D\p{Extended_Pictographic}(?:\uFE0F|\p{Emoji_Modifier})?)*|\p{Regional_Indicator}{2}|[0-9#*]\uFE0F?\u20E3)+$/u
export function isEmojiOnly(text) {
  const compact = String(text || '').replace(/\s/gu, '')
  return compact.length > 0 && emojiSequence.test(compact)
}
