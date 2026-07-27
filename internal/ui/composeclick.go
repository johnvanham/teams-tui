package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// This file bridges screen cells and the compose textarea's own coordinates, so
// the mouse can drive the caret and text selection inside it.
//
// bubbles' textarea has no mouse support: it addresses the caret as a (logical
// line, rune column) pair, while the screen hands us a (row, display column)
// cell. The two differ because a long logical line soft-wraps across several
// screen rows and because a rune can be two cells wide. The textarea also keeps
// its wrapping private, exposing only MoveToBegin/CursorUp/CursorDown (which
// step one *display* row at a time) and LineInfo (which describes the display
// row the caret currently sits on). So we recover the mapping by walking the
// caret over the box's visible rows once and recording what each row shows.

// composeRow describes one visible display row of the compose box: the logical
// line of the compose value it belongs to and the half-open range of that
// line's runes it displays. Consecutive rows sharing a line are soft wraps.
type composeRow struct {
	line int // index of the logical line in the compose value
	from int // first rune of the line shown on this row
	to   int // one past the last rune shown on this row
}

// composeTextLeft is the screen X of the first character cell inside the
// compose box: past the sidebar, the box border and its 1-cell left padding.
// It matches the messages pane's geometry in selectionAt.
func (m Model) composeTextLeft() int {
	return sidebarWidth + 1 /*border*/ + 1 /*padding*/
}

// composeTextTop is the screen Y of the compose box's first text row (just
// below its top border).
func (m Model) composeTextTop() int {
	return m.composeTop() + 1 /*border*/
}

// composeCellAt maps a screen coordinate to a cell of the compose box's text
// area, as a visible row index and a display column. Coordinates are clamped
// into the box, so a click on its border or past the end of a line still lands
// on the nearest character rather than being dropped.
func (m Model) composeCellAt(x, y int) (row, col int) {
	row = y - m.composeTextTop()
	if row < 0 {
		row = 0
	}
	if height := m.compose.Height(); row > height-1 {
		row = height - 1
	}
	if row < 0 {
		row = 0
	}
	col = x - m.composeTextLeft()
	if col < 0 {
		col = 0
	}
	return row, col
}

// moveComposeCursorTo places the caret on the given cell of the compose box.
func (m *Model) moveComposeCursorTo(row, col int) {
	rows := m.composeSeek(row)
	if len(rows) == 0 {
		return
	}
	if row > len(rows)-1 {
		row = len(rows) - 1
	}
	r := rows[row]
	// Landing on the very end of a soft-wrapped row would drop the caret onto
	// the next row, since that rune index is also where that row starts. Stop
	// one short so the caret stays on the row the user clicked.
	if row+1 < len(rows) && rows[row+1].line == r.line && r.to > r.from {
		r.to--
	}
	m.compose.SetCursorColumn(composeRuneAt(m.composeLineRunes(), r, col))
}

// composeRows returns the compose box's visible display rows, leaving the caret
// and scroll offset where they were.
func (m *Model) composeRows() []composeRow {
	return m.composeScan(-1)
}

// composeSeek returns the visible display rows and parks the caret at the start
// of row want (clamped to the rows that exist).
func (m *Model) composeSeek(want int) []composeRow {
	return m.composeScan(want)
}

// composeScan walks the caret over the compose box's visible display rows,
// recording each, then leaves it on row land (or back where it started, when
// land is negative).
//
// The only way to address a display row is to walk from the top, which scrolls
// the textarea to follow the caret. To put the view back we deliberately walk
// all the way down to the last visible row first: the textarea scrolls just far
// enough to keep the caret on screen, so ending on the bottom row restores
// exactly the offset we started with. Stepping back up from there is free,
// because every row we could land on is by definition already visible.
func (m *Model) composeScan(land int) []composeRow {
	height := m.compose.Height()
	if height < 1 {
		return nil
	}
	restore := land < 0
	offset := m.compose.ScrollYOffset()
	startLine, startCol := m.compose.Line(), m.compose.Column()
	lines := composeLines(m.compose.Value())

	m.compose.MoveToBegin()
	var rows []composeRow
	for i := 0; ; i++ {
		if i >= offset {
			rows = append(rows, m.composeRowAt(lines))
		}
		if i >= offset+height-1 {
			break
		}
		line, col := m.compose.Line(), m.compose.Column()
		m.compose.CursorDown()
		if m.compose.Line() == line && m.compose.Column() == col {
			break // end of the content: no row below to move to
		}
	}
	if len(rows) == 0 {
		return nil
	}

	// The caret is now on the last visible row and the view is back where it
	// was. Step up to the row we're meant to land on.
	if restore {
		land = composeRowOf(rows, startLine, startCol)
	}
	if land > len(rows)-1 {
		land = len(rows) - 1
	}
	if land < 0 {
		land = 0
	}
	for i := len(rows) - 1; i > land; i-- {
		m.compose.CursorUp()
	}
	if restore {
		m.compose.SetCursorColumn(startCol)
	}
	return rows
}

// composeRowAt describes the display row the caret currently sits on. It is
// only meaningful with the caret at the row's first rune, which is where the
// walk in composeScan keeps it.
func (m Model) composeRowAt(lines [][]rune) composeRow {
	li := m.compose.LineInfo()
	r := composeRow{line: m.compose.Line(), from: li.StartColumn, to: li.StartColumn + li.Width}
	// The textarea's wrapping pads the final row of a line with a phantom
	// trailing space; clamp to the runes that actually exist.
	if r.line >= 0 && r.line < len(lines) {
		if n := len(lines[r.line]); r.to > n {
			r.to = n
		}
	}
	if r.to < r.from {
		r.to = r.from
	}
	return r
}

// composeRowOf finds the display row holding the given logical position. When
// the position sits exactly on a soft-wrap boundary it belongs to the later
// row, matching where the textarea itself puts the caret.
func composeRowOf(rows []composeRow, line, col int) int {
	found := len(rows) - 1
	for i, r := range rows {
		if r.line == line && r.from <= col {
			found = i
		}
	}
	return found
}

// composeRuneAt returns the index of the rune whose cell covers display column
// col on the given row, walking the row's runes so that double-width
// characters count as the two cells they occupy. A column past the row's end
// yields the row's end.
func composeRuneAt(runes []rune, r composeRow, col int) int {
	idx, width := r.from, 0
	for idx < r.to && idx < len(runes) {
		w := ansi.StringWidth(string(runes[idx]))
		if width+w > col {
			break
		}
		width += w
		idx++
	}
	return idx
}

// setComposeCursor places the caret at a logical line and rune column. Use it
// after any SetValue, which parks the caret at the end of the buffer.
//
// The textarea can only step the caret one *display* row at a time, so we walk
// down until the logical line matches rather than stepping `line` times: a line
// that soft-wraps occupies several display rows, and counting rows would land
// short of the line we were asked for.
func (m *Model) setComposeCursor(line, col int) {
	m.compose.MoveToBegin()
	for m.compose.Line() < line {
		at, atCol := m.compose.Line(), m.compose.Column()
		m.compose.CursorDown()
		if m.compose.Line() == at && m.compose.Column() == atCol {
			break
		}
	}
	m.compose.SetCursorColumn(col)
}

// composeLineRunes returns the runes of the logical line the caret is on.
// LineInfo reports columns as rune indices into that line, so we need it to
// measure their display widths.
func (m Model) composeLineRunes() []rune {
	lines := composeLines(m.compose.Value())
	row := m.compose.Line()
	if row < 0 || row >= len(lines) {
		return nil
	}
	return lines[row]
}

// composeLines splits a compose value into its logical lines as runes, matching
// how the textarea indexes them.
func composeLines(value string) [][]rune {
	parts := strings.Split(value, "\n")
	lines := make([][]rune, len(parts))
	for i, p := range parts {
		lines[i] = []rune(p)
	}
	return lines
}
