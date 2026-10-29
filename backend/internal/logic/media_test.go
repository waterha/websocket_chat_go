package logic

import "testing"

func TestAudioMediaTypesAndLimits(t *testing.T) {
	for _, contentType := range []string{"audio/webm", "audio/ogg", "audio/mp4", "audio/mpeg", "audio/wav", " Audio/WebM;codecs=opus "} {
		t.Run(contentType, func(t *testing.T) {
			kind, ok := MediaKind(contentType)
			if !ok || kind != "audio" || MediaLimit(kind) != 10<<20 {
				t.Fatalf("audio classification/limit: %q %v %d", kind, ok, MediaLimit(kind))
			}
		})
	}
	for _, contentType := range []string{"", "audio", "audio/", "audio/og g", "audio/ogg\r\nX-Test: value"} {
		if kind, ok := MediaKind(contentType); ok {
			t.Errorf("malformed type %q accepted as %q", contentType, kind)
		}
	}
	for contentType, want := range map[string]string{"audio/flac": "file", "application/octet-stream": "file", "video/webm": "video", "image/png": "image"} {
		if got, ok := MediaKind(contentType); !ok || got != want {
			t.Errorf("MediaKind(%q) = %q, %v; want %q", contentType, got, ok, want)
		}
	}
	if MediaLimit("file") != 50<<20 || MediaLimit("video") != 100<<20 || MediaLimit("image") != 10<<20 {
		t.Fatal("existing media limits changed")
	}
}
