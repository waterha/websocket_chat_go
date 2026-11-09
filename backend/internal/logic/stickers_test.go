package logic

import (
	"strings"
	"testing"
)

func TestEmojiOnly(t *testing.T) {
	for _, text := range []string{"😀", "👩🏽‍💻", "❤️", "🇨🇳", "😀👍🏽🎉", "😀 👍🏽", "😀\n❤️", "1️⃣", "#️⃣", "©️"} {
		if !EmojiOnly(text) {
			t.Errorf("rejected emoji %q", text)
		}
	}
	for _, text := range []string{"", "hello", "😀hello", "<img src=x>", "https://example.com", "123", "#", " \n ", "\u200d😀", "😀\u200d", "\ufe0f", "🏽", "😀\u200d\ufe0f", strings.Repeat("😀", 65), string([]byte{0xff})} {
		if EmojiOnly(text) {
			t.Errorf("accepted non-emoji %q", text)
		}
	}
}
