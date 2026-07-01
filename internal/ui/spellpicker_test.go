package ui

import (
	"reflect"
	"testing"

	"charm.land/bubbles/v2/textarea"

	"github.com/jvh/teams-tui/internal/spell"
)

func TestBuildSpellCandidates(t *testing.T) {
	miss := []spell.Misspelling{
		{Word: "teh", Suggestions: []string{"the", "tea", "ten"}},
		{Word: "nogo"}, // no suggestions -> skipped
		{Word: "recieve", Suggestions: []string{"receive"}},
	}
	got := buildSpellCandidates(miss)
	want := []spellCandidate{
		{Word: "teh", Suggestion: "the"},
		{Word: "teh", Suggestion: "tea"},
		{Word: "teh", Suggestion: "ten"},
		{Word: "recieve", Suggestion: "receive"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildSpellCandidates() = %#v, want %#v", got, want)
	}
}

func TestBuildSpellCandidatesCapsSuggestions(t *testing.T) {
	sugs := []string{"a", "b", "c", "d", "e", "f", "g"}
	got := buildSpellCandidates([]spell.Misspelling{{Word: "x", Suggestions: sugs}})
	if len(got) != spellSuggestMax {
		t.Errorf("got %d candidates, want cap of %d", len(got), spellSuggestMax)
	}
}

func TestFindWord(t *testing.T) {
	tests := []struct {
		name             string
		lines            []string
		word             string
		wantRow, wantCol int
		wantOK           bool
	}{
		{"simple", []string{"teh cat"}, "teh", 0, 0, true},
		{"mid line", []string{"the teh cat"}, "teh", 0, 4, true},
		{"second line", []string{"all good", "teh cat"}, "teh", 1, 0, true},
		{"whole word only", []string{"theh teh"}, "teh", 0, 5, true},
		{"substring ignored", []string{"tehcat"}, "teh", 0, 0, false},
		{"not found", []string{"all good"}, "teh", 0, 0, false},
		{"with punctuation", []string{"teh, cat"}, "teh", 0, 0, true},
		{"empty word", []string{"x"}, "", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row, col, ok := findWord(tt.lines, tt.word)
			if ok != tt.wantOK || (ok && (row != tt.wantRow || col != tt.wantCol)) {
				t.Errorf("findWord() = (%d,%d,%v), want (%d,%d,%v)",
					row, col, ok, tt.wantRow, tt.wantCol, tt.wantOK)
			}
		})
	}
}

// newSpellPickerModel returns a Model with the compose box set to value and the
// correction picker open on the given candidate.
func newSpellPickerModel(value string, cands []spellCandidate, sel int) Model {
	ta := textarea.New()
	ta.SetWidth(60)
	ta.SetValue(value)
	ta.MoveToEnd()
	return Model{
		compose:         ta,
		spellPicker:     true,
		spellCandidates: cands,
		spellPickerSel:  sel,
	}
}

func TestApplySpellCandidate(t *testing.T) {
	m := newSpellPickerModel("teh cat", []spellCandidate{{Word: "teh", Suggestion: "the"}}, 0)
	if !m.applySpellCandidate() {
		t.Fatal("applySpellCandidate returned false")
	}
	if got, want := m.compose.Value(), "the cat"; got != want {
		t.Errorf("compose = %q, want %q", got, want)
	}
}

func TestApplySpellCandidateMidText(t *testing.T) {
	m := newSpellPickerModel("please recieve this", []spellCandidate{{Word: "recieve", Suggestion: "receive"}}, 0)
	if !m.applySpellCandidate() {
		t.Fatal("applySpellCandidate returned false")
	}
	if got, want := m.compose.Value(), "please receive this"; got != want {
		t.Errorf("compose = %q, want %q", got, want)
	}
}

func TestApplySpellCandidateMultiline(t *testing.T) {
	m := newSpellPickerModel("first line\nteh second", []spellCandidate{{Word: "teh", Suggestion: "the"}}, 0)
	if !m.applySpellCandidate() {
		t.Fatal("applySpellCandidate returned false")
	}
	if got, want := m.compose.Value(), "first line\nthe second"; got != want {
		t.Errorf("compose = %q, want %q", got, want)
	}
}

func TestApplySpellCandidateWordGone(t *testing.T) {
	// The misspelled word is no longer present (user edited it): apply is a
	// no-op and leaves the text untouched.
	m := newSpellPickerModel("all fixed now", []spellCandidate{{Word: "teh", Suggestion: "the"}}, 0)
	if m.applySpellCandidate() {
		t.Error("expected false when word not found")
	}
	if got, want := m.compose.Value(), "all fixed now"; got != want {
		t.Errorf("compose = %q, want %q", got, want)
	}
}

func TestApplySpellCandidateNoSelection(t *testing.T) {
	m := newSpellPickerModel("teh cat", nil, 0)
	if m.applySpellCandidate() {
		t.Error("expected false with no candidates")
	}
}

func TestOpenSpellPicker(t *testing.T) {
	m := Model{spellMisspell: []spell.Misspelling{
		{Word: "teh", Suggestions: []string{"the"}},
	}}
	if !m.openSpellPicker() {
		t.Fatal("openSpellPicker returned false with correctable words")
	}
	if !m.spellPicker || len(m.spellCandidates) != 1 {
		t.Errorf("picker not opened correctly: open=%v cands=%d", m.spellPicker, len(m.spellCandidates))
	}
}

func TestOpenSpellPickerNothingToCorrect(t *testing.T) {
	// Misspellings with no suggestions -> nothing to pick.
	m := Model{spellMisspell: []spell.Misspelling{{Word: "xyzzyq"}}}
	if m.openSpellPicker() {
		t.Error("expected false when no candidates have suggestions")
	}
	if m.spellPicker {
		t.Error("picker should not be open")
	}
}

func TestSpellPickerMoveWraps(t *testing.T) {
	m := Model{spellCandidates: []spellCandidate{{}, {}, {}}, spellPickerSel: 0}
	m.spellPickerMove(-1)
	if m.spellPickerSel != 2 {
		t.Errorf("wrap up: sel = %d, want 2", m.spellPickerSel)
	}
	m.spellPickerMove(1)
	if m.spellPickerSel != 0 {
		t.Errorf("wrap down: sel = %d, want 0", m.spellPickerSel)
	}
}
