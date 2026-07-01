package ui

import (
	"strings"

	"github.com/jvh/teams-tui/internal/spell"
)

// spellSuggestMax caps how many suggestions per misspelled word the correction
// picker lists, so a word with many candidates doesn't flood the popup.
const spellSuggestMax = 5

// spellCandidate is one selectable correction in the picker: replace the
// misspelled Word (its first occurrence in the compose text) with Suggestion.
type spellCandidate struct {
	Word       string
	Suggestion string
}

// openSpellPicker builds the correction candidates from the current
// misspellings and shows the picker. It returns false (showing nothing) when
// there is nothing correctable — no misspellings, or none with a suggestion.
func (m *Model) openSpellPicker() bool {
	cands := buildSpellCandidates(m.spellMisspell)
	if len(cands) == 0 {
		return false
	}
	m.spellPicker = true
	m.spellCandidates = cands
	m.spellPickerSel = 0
	return true
}

// closeSpellPicker hides the correction picker and clears its transient state.
func (m *Model) closeSpellPicker() {
	m.spellPicker = false
	m.spellCandidates = nil
	m.spellPickerSel = 0
}

// spellPickerMove moves the highlighted candidate by delta, wrapping at the
// ends.
func (m *Model) spellPickerMove(delta int) {
	n := len(m.spellCandidates)
	if n == 0 {
		return
	}
	m.spellPickerSel = (m.spellPickerSel + delta + n) % n
}

// buildSpellCandidates flattens misspellings (each with its suggestions) into a
// selectable list, keeping word order and capping suggestions per word. Words
// with no suggestions are skipped since there's nothing to apply.
func buildSpellCandidates(miss []spell.Misspelling) []spellCandidate {
	var cands []spellCandidate
	for _, ms := range miss {
		n := len(ms.Suggestions)
		if n > spellSuggestMax {
			n = spellSuggestMax
		}
		for i := 0; i < n; i++ {
			cands = append(cands, spellCandidate{Word: ms.Word, Suggestion: ms.Suggestions[i]})
		}
	}
	return cands
}

// applySpellCandidate replaces the first whole-word occurrence of the
// highlighted candidate's misspelled word with its suggestion, preserving the
// rest of the compose text and repositioning the cursor after the replacement.
// It returns false (leaving the composer untouched) when there's no valid
// selection or the word can no longer be found (e.g. the user edited it away).
func (m *Model) applySpellCandidate() bool {
	if !m.spellPicker || m.spellPickerSel < 0 || m.spellPickerSel >= len(m.spellCandidates) {
		return false
	}
	cand := m.spellCandidates[m.spellPickerSel]

	lines := strings.Split(m.compose.Value(), "\n")
	row, col, ok := findWord(lines, cand.Word)
	if !ok {
		return false
	}
	runes := []rune(lines[row])
	wordLen := len([]rune(cand.Word))
	lines[row] = string(runes[:col]) + cand.Suggestion + string(runes[col+wordLen:])
	m.compose.SetValue(strings.Join(lines, "\n"))

	// Reposition the cursor just after the inserted suggestion (SetValue parks
	// it at the buffer end), mirroring the emoji/mention pickers.
	m.compose.MoveToBegin()
	for i := 0; i < row; i++ {
		m.compose.CursorDown()
	}
	m.compose.SetCursorColumn(col + len([]rune(cand.Suggestion)))
	return true
}

// findWord locates the first whole-word occurrence of word across lines,
// returning its line index and rune column. "Whole word" means the match isn't
// flanked by letters/digits, so correcting "teh" won't hit "theh" and a
// substring of a larger token is ignored. ok is false when it isn't found.
func findWord(lines []string, word string) (row, col int, ok bool) {
	target := []rune(word)
	if len(target) == 0 {
		return 0, 0, false
	}
	for r, line := range lines {
		runes := []rune(line)
		for c := 0; c+len(target) <= len(runes); c++ {
			if !runesEqual(runes[c:c+len(target)], target) {
				continue
			}
			// Boundary check: the rune before and after must not be word chars.
			if c > 0 && isWordRune(runes[c-1]) {
				continue
			}
			if c+len(target) < len(runes) && isWordRune(runes[c+len(target)]) {
				continue
			}
			return r, c, true
		}
	}
	return 0, 0, false
}

func runesEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// isWordRune reports whether r is part of a word for boundary detection. Letters
// and digits count; an apostrophe does too so contractions like "don't" stay
// whole.
func isWordRune(r rune) bool {
	return r == '\'' ||
		(r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r >= 0x80 // treat non-ASCII (accented letters, etc.) as word chars
}
