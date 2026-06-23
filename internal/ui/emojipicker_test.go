package ui

import (
	"testing"

	"charm.land/bubbles/v2/textarea"
)

// newComposeModel returns a Model with just enough wired up to exercise the
// inline emoji picker logic against the compose textarea.
func newComposeModel(value string) Model {
	ta := textarea.New()
	ta.SetWidth(40)
	ta.SetValue(value)
	ta.MoveToEnd() // cursor at end, as if the user just typed `value`
	return Model{compose: ta}
}

func TestActiveEmojiToken(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantName  string
		wantOK    bool
		wantColon int
	}{
		{"simple token", "hi :th", "th", true, 3},
		{"token at start", ":sm", "sm", true, 0},
		{"no colon", "hello", "", false, 0},
		{"space breaks token", "hi : th", "", false, 0},
		{"completed colon then word", "done :ok: more", "", false, 0},
		{"single char token still detected", "x :a", "a", true, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newComposeModel(tt.value)
			name, colon, ok := m.activeEmojiToken()
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (name=%q)", ok, tt.wantOK, name)
			}
			if ok {
				if name != tt.wantName {
					t.Errorf("name = %q, want %q", name, tt.wantName)
				}
				if colon != tt.wantColon {
					t.Errorf("colon col = %d, want %d", colon, tt.wantColon)
				}
			}
		})
	}
}

func TestRefreshEmojiPickerTrigger(t *testing.T) {
	// One char after the colon: too short, popup stays closed.
	m := newComposeModel("hi :t")
	m.refreshEmojiPicker()
	if m.emojiPicker {
		t.Errorf("picker should be closed for single-char token")
	}

	// Two chars: popup opens with matches.
	m = newComposeModel("hi :th")
	m.refreshEmojiPicker()
	if !m.emojiPicker {
		t.Fatalf("picker should be open for two-char token")
	}
	if len(m.emojiMatches) == 0 {
		t.Errorf("expected matches for ':th'")
	}

	// Unknown token: popup closed.
	m = newComposeModel("hi :qx")
	m.refreshEmojiPicker()
	if m.emojiPicker {
		t.Errorf("picker should be closed for non-matching token")
	}
}

func TestAutoReplaceEmoticon(t *testing.T) {
	// autoReplaceEmoticon fires on every keystroke and only handles non-colon
	// emoticons; colon-led ones (":-)", ":p") defer to a word boundary so they
	// don't pre-empt :shortcode: tokens (see TestColonEmoticonBoundary).
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"heart converts immediately", "love <3", "love ❤️"},
		{"shrug converts immediately", `\o/`, "🙌"},
		{"colon emoticon NOT replaced inline", "hello :-)", "hello :-)"},
		{"colon emoticon at start NOT replaced inline", ":-)", ":-)"},
		{"no emoticon untouched", "hello :", "hello :"},
		{"incomplete emoticon untouched", "hello :-", "hello :-"},
		{"text after emoticon not replaced", "<3 ok", "<3 ok"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newComposeModel(tt.value)
			m.autoReplaceEmoticon()
			if got := m.compose.Value(); got != tt.want {
				t.Errorf("value = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestColonEmoticonBoundary(t *testing.T) {
	// A colon emoticon converts only once a space finalizes it.
	t.Run("space finalizes colon emoticon", func(t *testing.T) {
		m := newComposeModel(":-) ") // user typed ":-)" then a space
		m.replaceColonEmoticonBeforeCursor()
		if got := m.compose.Value(); got != "🙂 " {
			t.Errorf("value = %q, want %q", got, "🙂 ")
		}
	})
	t.Run("colon shortcode prefix is not eaten", func(t *testing.T) {
		// ":party" must survive: it shares the ":p" prefix with an emoticon but
		// is not itself one, so the boundary replacement leaves it intact.
		m := newComposeModel(":party ")
		m.replaceColonEmoticonBeforeCursor()
		if got := m.compose.Value(); got != ":party " {
			t.Errorf("value = %q, want %q", got, ":party ")
		}
	})
	t.Run("typing p after colon does not convert", func(t *testing.T) {
		// Mid-word: ":p" with no trailing boundary stays put so ":party" can be
		// finished.
		m := newComposeModel("hi :p")
		m.autoReplaceEmoticon() // simulate keystroke handling
		if got := m.compose.Value(); got != "hi :p" {
			t.Errorf("value = %q, want %q", got, "hi :p")
		}
	})
}

func TestApplyEmojiSelection(t *testing.T) {
	m := newComposeModel("hi :thu")
	m.refreshEmojiPicker()
	if !m.emojiPicker || len(m.emojiMatches) == 0 {
		t.Fatalf("expected open picker with matches")
	}
	want := "hi " + m.emojiMatches[m.emojiSel].Emoji
	if !m.applyEmojiSelection() {
		t.Fatalf("applyEmojiSelection returned false")
	}
	if got := m.compose.Value(); got != want {
		t.Errorf("compose value = %q, want %q", got, want)
	}
	if m.emojiPicker {
		t.Errorf("picker should close after selection")
	}
}
