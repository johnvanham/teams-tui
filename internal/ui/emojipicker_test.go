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
	m = newComposeModel("hi :zz")
	m.refreshEmojiPicker()
	if m.emojiPicker {
		t.Errorf("picker should be closed for non-matching token")
	}
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
