package graph

import "testing"

func TestReplaceEmojiTags(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "alt holds unicode",
			content: `hi <emoji id="smile" alt="😄" title="Smile"></emoji>`,
			want:    "hi 😄",
		},
		{
			name:    "self closing tag with alt",
			content: `<emoji id="laugh" alt="😆"/> nice`,
			want:    "😆 nice",
		},
		{
			name:    "inner text fallback",
			content: `done <emoji id="check">✅</emoji>`,
			want:    "done ✅",
		},
		{
			name:    "entity in alt is decoded",
			content: `<emoji alt="&#128512;"></emoji>`,
			want:    "😀",
		},
		{
			name:    "keyword id fallback",
			content: `<emoji id="party"></emoji> woo`,
			want:    "🎉 woo",
		},
		{
			name:    "title fallback when no glyph available",
			content: `<emoji id="unknown-thing" title="Mystery"></emoji>`,
			want:    ":Mystery:",
		},
		{
			name:    "multiple emoji in one string",
			content: `<emoji alt="👍"></emoji><emoji alt="🔥"></emoji>`,
			want:    "👍🔥",
		},
		{
			name:    "no emoji tags untouched",
			content: `plain text <b>bold</b>`,
			want:    `plain text <b>bold</b>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replaceEmojiTags(tt.content)
			if got != tt.want {
				t.Errorf("replaceEmojiTags(%q) = %q, want %q", tt.content, got, tt.want)
			}
		})
	}
}
