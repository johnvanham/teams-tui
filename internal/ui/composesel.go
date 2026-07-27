package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/jvh/teams-tui/internal/ui/styles"
)

// This file implements mouse text selection inside the compose box: press and
// drag to highlight a run of characters, then copy it (ctrl+c) or replace it by
// typing, pasting or deleting. bubbles' textarea has no notion of a selection,
// so we keep one alongside it and splice its value when the selection is
// replaced.
//
// Endpoints are (visible display row, display column) — the same coordinates
// composeCellAt produces from a mouse position. Screen rows are a safe handle
// because a selection cannot outlive the text under it: every edit and every
// caret move clears it, and those are the only things that reflow or scroll the
// box.

// composePos is a position in the compose value: a logical line and a rune
// column within it.
type composePos struct {
	line int
	col  int
}

// hasComposeSelection reports whether a non-empty compose selection is held.
//
// A selection is pinned to the text it was taken against and to the compose
// pane: rather than hunting down every path that rewrites the box (sending,
// editing, the emoji/mention/spelling pickers, …) to clear it, we simply treat
// it as gone the moment the text changes or focus moves elsewhere.
func (m Model) hasComposeSelection() bool {
	return m.composeSelecting &&
		m.focus == focusCompose &&
		m.composeAnchor != m.composeCur &&
		m.compose.Value() == m.composeSelValue
}

// clearComposeSelection drops any compose selection (and its highlight).
func (m *Model) clearComposeSelection() {
	m.composeSelecting = false
	m.composeAnchor = selPoint{}
	m.composeCur = selPoint{}
	m.composeSelValue = ""
}

// composeSelectionBounds returns the selection's endpoints in reading order.
func (m Model) composeSelectionBounds() (selPoint, selPoint) {
	if m.composeCur.less(m.composeAnchor) {
		return m.composeCur, m.composeAnchor
	}
	return m.composeAnchor, m.composeCur
}

// composeSelectionSpan resolves the on-screen selection to a range of the
// compose value. ok is false when nothing is selected.
func (m *Model) composeSelectionSpan() (start, end composePos, ok bool) {
	if !m.hasComposeSelection() {
		return composePos{}, composePos{}, false
	}
	a, b := m.composeSelectionBounds()
	rows := m.composeRows()
	if len(rows) == 0 {
		return composePos{}, composePos{}, false
	}
	lines := composeLines(m.compose.Value())
	return composePosAt(rows, lines, a), composePosAt(rows, lines, b), true
}

// composePosAt converts a selection endpoint to a position in the value.
func composePosAt(rows []composeRow, lines [][]rune, p selPoint) composePos {
	i := p.line
	if i > len(rows)-1 {
		i = len(rows) - 1
	}
	if i < 0 {
		i = 0
	}
	r := rows[i]
	var runes []rune
	if r.line >= 0 && r.line < len(lines) {
		runes = lines[r.line]
	}
	return composePos{line: r.line, col: composeRuneAt(runes, r, p.col)}
}

// composeSelectionText returns the selected text. Rows that are soft wraps of
// one logical line rejoin seamlessly (their runes are contiguous in the value);
// only real newlines in the compose box become newlines here.
func (m *Model) composeSelectionText() string {
	start, end, ok := m.composeSelectionSpan()
	if !ok {
		return ""
	}
	return composeSlice(composeLines(m.compose.Value()), start, end)
}

// composeSlice returns the text between two positions in the compose value.
func composeSlice(lines [][]rune, start, end composePos) string {
	if start.line < 0 || start.line >= len(lines) || end.line < 0 || end.line >= len(lines) {
		return ""
	}
	if start.line == end.line {
		return string(clampRunes(lines[start.line], start.col, end.col))
	}
	var b strings.Builder
	b.WriteString(string(clampRunes(lines[start.line], start.col, len(lines[start.line]))))
	for i := start.line + 1; i < end.line; i++ {
		b.WriteByte('\n')
		b.WriteString(string(lines[i]))
	}
	b.WriteByte('\n')
	b.WriteString(string(clampRunes(lines[end.line], 0, end.col)))
	return b.String()
}

// clampRunes returns runes[lo:hi] with both bounds forced into range.
func clampRunes(runes []rune, lo, hi int) []rune {
	if lo < 0 {
		lo = 0
	}
	if hi > len(runes) {
		hi = len(runes)
	}
	if lo > hi {
		return nil
	}
	return runes[lo:hi]
}

// deleteComposeSelection removes the selected text from the compose box and
// leaves the caret where it began, so that typing or pasting straight after
// replaces the selection. It reports whether anything was deleted.
func (m *Model) deleteComposeSelection() bool {
	start, end, ok := m.composeSelectionSpan()
	m.clearComposeSelection()
	if !ok || start == end {
		return false
	}
	lines := composeLines(m.compose.Value())
	if start.line < 0 || start.line >= len(lines) || end.line < 0 || end.line >= len(lines) {
		return false
	}
	head := string(clampRunes(lines[start.line], 0, start.col))
	tail := string(clampRunes(lines[end.line], end.col, len(lines[end.line])))
	kept := make([]string, 0, len(lines))
	for i := 0; i < start.line; i++ {
		kept = append(kept, string(lines[i]))
	}
	kept = append(kept, head+tail)
	for i := end.line + 1; i < len(lines); i++ {
		kept = append(kept, string(lines[i]))
	}
	// SetValue parks the caret at the end of the buffer, so put it back at the
	// start of the removed range.
	m.compose.SetValue(strings.Join(kept, "\n"))
	m.setComposeCursor(start.line, start.col)
	return true
}

// applyComposeHighlight overlays the selection highlight onto the textarea's
// rendered view. Rows outside the selection pass through untouched. Spans are
// cut on display columns with ansi.Cut so the textarea's own styling (and its
// cursor) survive either side of the highlight.
func (m Model) applyComposeHighlight(view string) string {
	if !m.hasComposeSelection() {
		return view
	}
	start, end := m.composeSelectionBounds()
	rows := strings.Split(view, "\n")
	for i := start.line; i <= end.line && i < len(rows); i++ {
		if i < 0 {
			continue
		}
		src := rows[i]
		width := ansi.StringWidth(src)
		lo, hi := 0, width
		if i == start.line {
			lo = start.col
		}
		if i == end.line {
			hi = end.col
		}
		if lo > width {
			lo = width
		}
		if hi > width {
			hi = width
		}
		if lo >= hi {
			continue
		}
		before := ansi.Cut(src, 0, lo)
		mid := ansi.Strip(ansi.Cut(src, lo, hi))
		after := ansi.Cut(src, hi, width)
		rows[i] = before + styles.SelectionHighlight.Render(mid) + after
	}
	return strings.Join(rows, "\n")
}

// startComposeSelection anchors a new selection at the given screen position
// and moves the caret there.
func (m *Model) startComposeSelection(x, y int) {
	row, col := m.composeCellAt(x, y)
	m.clearComposeSelection()
	m.moveComposeCursorTo(row, col)
	m.composeSelecting = true
	m.composeAnchor = selPoint{line: row, col: col}
	m.composeCur = m.composeAnchor
	m.composeSelValue = m.compose.Value()
}

// extendComposeSelection moves the selection's free end to the given screen
// position as the user drags.
func (m *Model) extendComposeSelection(x, y int) {
	row, col := m.composeCellAt(x, y)
	m.composeCur = selPoint{line: row, col: col}
}
