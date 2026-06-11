package graph

import (
	"testing"
	"time"
)

func TestChatUnread(t *testing.T) {
	const self = "me-id"
	t0 := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	earlier := t0.Add(-time.Hour)
	later := t0.Add(time.Hour)

	otherFrom := &From{User: &User{ID: "other-id", DisplayName: "Other"}}
	selfFrom := &From{User: &User{ID: self, DisplayName: "Me"}}

	tests := []struct {
		name        string
		chat        Chat
		readHorizon time.Time
		want        bool
	}{
		{
			name: "no last message preview",
			chat: Chat{},
			want: false,
		},
		{
			name: "newer than server read marker, from other -> unread",
			chat: Chat{
				LastMessagePreview: &MessagePreview{CreatedAt: t0, From: otherFrom},
				Viewpoint:          &ChatViewpoint{LastMessageReadDateTime: earlier},
			},
			want: true,
		},
		{
			name: "read marker at or after last message -> read",
			chat: Chat{
				LastMessagePreview: &MessagePreview{CreatedAt: t0, From: otherFrom},
				Viewpoint:          &ChatViewpoint{LastMessageReadDateTime: later},
			},
			want: false,
		},
		{
			name: "own latest message is never unread",
			chat: Chat{
				LastMessagePreview: &MessagePreview{CreatedAt: t0, From: selfFrom},
				Viewpoint:          &ChatViewpoint{LastMessageReadDateTime: earlier},
			},
			want: false,
		},
		{
			name: "local read horizon overrides missing viewpoint",
			chat: Chat{
				LastMessagePreview: &MessagePreview{CreatedAt: t0, From: otherFrom},
			},
			readHorizon: later,
			want:        false,
		},
		{
			name: "no viewpoint and no horizon -> unread",
			chat: Chat{
				LastMessagePreview: &MessagePreview{CreatedAt: t0, From: otherFrom},
			},
			want: true,
		},
		{
			name: "zero last message time -> read",
			chat: Chat{
				LastMessagePreview: &MessagePreview{From: otherFrom},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.chat.Unread(self, tt.readHorizon); got != tt.want {
				t.Errorf("Unread() = %v, want %v", got, tt.want)
			}
		})
	}
}
