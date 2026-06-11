package graph

import "testing"

func TestReplaceShortcodes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"named shortcode", "nice :thumbsup:", "nice 👍"},
		{"alias shortcode", "great :+1:", "great 👍"},
		{"case insensitive name", "yay :TADA:", "yay 🎉"},
		{"multiple shortcodes", ":fire::rocket:", "🔥🚀"},
		{"unknown left untouched", "ratio 3:4 and :nope:", "ratio 3:4 and :nope:"},
		{"emoticon smiley", "hello :-)", "hello 🙂"},
		{"emoticon longest wins", ":-)", "🙂"},
		{"emoticon heart", "love <3", "love ❤️"},
		{"no emoji", "plain text", "plain text"},
		{"shortcode and emoticon", ":wave: :-)", "👋 🙂"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReplaceShortcodes(tt.in); got != tt.want {
				t.Errorf("ReplaceShortcodes(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMatchShortcodePrefix(t *testing.T) {
	t.Run("empty prefix returns nil", func(t *testing.T) {
		if got := MatchShortcodePrefix("", 5); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("prefix matches and exact sorts first", func(t *testing.T) {
		got := MatchShortcodePrefix("smile", 10)
		if len(got) == 0 {
			t.Fatalf("expected matches for 'smile'")
		}
		if got[0].Name != "smile" {
			t.Errorf("first match = %q, want exact 'smile'", got[0].Name)
		}
		for _, e := range got {
			if e.Emoji == "" {
				t.Errorf("match %q has empty emoji", e.Name)
			}
		}
	})

	t.Run("respects limit", func(t *testing.T) {
		got := MatchShortcodePrefix("s", 3)
		if len(got) > 3 {
			t.Errorf("got %d matches, want <= 3", len(got))
		}
	})

	t.Run("no match", func(t *testing.T) {
		if got := MatchShortcodePrefix("zzzznotanemoji", 5); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}
