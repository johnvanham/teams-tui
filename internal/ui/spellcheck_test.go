package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/jvh/teams-tui/internal/spell"
)

func TestSpellStripHeight(t *testing.T) {
	var m Model
	if got := m.spellStripHeight(); got != 0 {
		t.Errorf("no misspellings: height = %d, want 0", got)
	}
	m.spellMisspell = []spell.Misspelling{{Word: "teh"}}
	if got := m.spellStripHeight(); got != 1 {
		t.Errorf("with misspellings: height = %d, want 1", got)
	}
}

func TestViewSpellStrip(t *testing.T) {
	m := Model{spellMisspell: []spell.Misspelling{
		{Word: "teh", Suggestions: []string{"the", "tea"}},
		{Word: "recieve", Suggestions: []string{"receive"}},
		{Word: "xyzzyq"}, // no suggestions
	}}
	out := ansi.Strip(m.viewSpellStrip(120))
	for _, want := range []string{"teh", "the", "recieve", "receive", "xyzzyq"} {
		if !strings.Contains(out, want) {
			t.Errorf("strip %q missing %q", out, want)
		}
	}
}

func TestViewSpellStripTruncates(t *testing.T) {
	// Many misspellings in a narrow width should show at least the first and an
	// overflow indicator, and must not exceed the width.
	var words []spell.Misspelling
	for _, w := range []string{"aaaa", "bbbb", "cccc", "dddd", "eeee", "ffff"} {
		words = append(words, spell.Misspelling{Word: w})
	}
	m := Model{spellMisspell: words}
	raw := m.viewSpellStrip(24)
	out := ansi.Strip(raw)
	if !strings.Contains(out, "aaaa") {
		t.Errorf("expected first word shown, got %q", out)
	}
	if !strings.Contains(out, "(+") {
		t.Errorf("expected overflow indicator, got %q", out)
	}
	// The visible width must not exceed the budget.
	if w := ansi.StringWidth(raw); w > 24 {
		t.Errorf("strip width = %d, want <= 24", w)
	}
}

func TestScheduleSpellCheckDisabled(t *testing.T) {
	// With no checker (feature off/unavailable) scheduling is a no-op: no
	// generation bump, no command.
	var m Model
	cmd := m.scheduleSpellCheck()
	if cmd != nil {
		t.Error("expected nil cmd when speller unavailable")
	}
	if m.spellGen != 0 {
		t.Errorf("spellGen = %d, want 0 (no bump when disabled)", m.spellGen)
	}
}

func TestHandleSpellCheckedIgnoresStale(t *testing.T) {
	m := Model{spellGen: 5}
	// A result tagged with an older generation must be ignored.
	updated, _ := m.handleSpellChecked(spellCheckedMsg{gen: 3, words: []spell.Misspelling{{Word: "teh"}}})
	if got := updated.(Model).spellMisspell; got != nil {
		t.Errorf("stale result applied: %v", got)
	}
	// A current-generation result is stored.
	updated, _ = m.handleSpellChecked(spellCheckedMsg{gen: 5, words: []spell.Misspelling{{Word: "teh"}}})
	if got := updated.(Model).spellMisspell; len(got) != 1 || got[0].Word != "teh" {
		t.Errorf("current result not stored: %v", got)
	}
}

func TestHandleSpellDebounceIgnoresStale(t *testing.T) {
	// A debounce tick from a superseded generation should not launch a check
	// (returns a nil command).
	m := Model{spellGen: 9}
	_, cmd := m.handleSpellDebounce(spellDebounceMsg{gen: 4})
	if cmd != nil {
		t.Error("expected nil cmd for stale debounce generation")
	}
}

func TestClearSpellCheck(t *testing.T) {
	m := Model{spellGen: 2, spellMisspell: []spell.Misspelling{{Word: "teh"}}}
	m.clearSpellCheck()
	if m.spellMisspell != nil {
		t.Errorf("misspellings not cleared: %v", m.spellMisspell)
	}
	if m.spellGen != 3 {
		t.Errorf("spellGen = %d, want 3 (bumped to invalidate in-flight)", m.spellGen)
	}
}
