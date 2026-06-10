package auth

import "testing"

func TestCoversScopes(t *testing.T) {
	required := []string{"offline_access", "openid", "profile", "User.Read", "Chat.ReadWrite", "Calendars.Read"}

	cases := []struct {
		name  string
		scope string
		want  bool
	}{
		{
			name:  "full graph-prefixed scopes",
			scope: "https://graph.microsoft.com/User.Read https://graph.microsoft.com/Chat.ReadWrite https://graph.microsoft.com/Calendars.Read profile openid email",
			want:  true,
		},
		{
			name:  "bare scopes",
			scope: "User.Read Chat.ReadWrite Calendars.Read",
			want:  true,
		},
		{
			name:  "missing Chat.ReadWrite (stale token)",
			scope: "https://graph.microsoft.com/User.Read profile openid",
			want:  false,
		},
		{
			name:  "case-insensitive",
			scope: "user.read chat.readwrite calendars.read",
			want:  true,
		},
		{
			name:  "empty",
			scope: "",
			want:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok := &Token{Scope: tc.scope}
			if got := tok.CoversScopes(required); got != tc.want {
				t.Errorf("CoversScopes(%q) = %v, want %v", tc.scope, got, tc.want)
			}
		})
	}
}
