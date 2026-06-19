package graph

import "testing"

func TestUserReacted(t *testing.T) {
	msg := Message{
		Reactions: []Reaction{
			{ReactionType: "👍", User: &From{User: &User{ID: "u1"}}},
			{ReactionType: "like", User: &From{User: &User{ID: "u2"}}}, // keyword form of 👍
			{ReactionType: "❤️", User: &From{User: &User{ID: "u1"}}},
		},
	}

	tests := []struct {
		name   string
		userID string
		emoji  string
		want   bool
	}{
		{"unicode match", "u1", "👍", true},
		{"keyword resolves to glyph", "u2", "👍", true},
		{"second emoji for same user", "u1", "❤️", true},
		{"wrong user", "u2", "❤️", false},
		{"emoji not present", "u1", "😡", false},
		{"empty user never matches", "", "👍", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := msg.UserReacted(tt.userID, tt.emoji); got != tt.want {
				t.Errorf("UserReacted(%q, %q) = %v, want %v", tt.userID, tt.emoji, got, tt.want)
			}
		})
	}
}
