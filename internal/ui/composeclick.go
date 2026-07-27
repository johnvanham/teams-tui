package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// This file maps a mouse click on the compose box to a cursor position inside
// the textarea, so clicking a character jumps the caret there.
//
// The textarea has no mouse support of its own, and its cursor is addressed as
// a (logical line, rune column) pair while the screen gives us a (row, display
// column) cell. Bridging the two means walking the textarea's *visual* lines —
// a long logical line soft-wraps across several screen rows — using the only
// public levers it offers: MoveToBegin/CursorUp/CursorDown (which step one
// visual line at a time) and LineInfo (which describes the visual line the
// cursor currently sits on).

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

// moveComposeCursorTo places the textarea cursor at the character under the
// given screen coordinate. Clicks on the box's border or padding, or past the
// end of a line, clamp to the nearest text position, so every click inside the
// box lands somewhere sensible rather than being ignored.
func (m *Model) moveComposeCursorTo(x, y int) {
	height := m.compose.Height()
	if height < 1 {
		return
	}

	// Row of the click within the visible text area, clamped so clicks on the
	// borders snap to the first/last visible line.
	row := y - m.composeTextTop()
	if row < 0 {
		row = 0
	}
	if row > height-1 {
		row = height - 1
	}
	col := x - m.composeTextLeft()
	if col < 0 {
		col = 0
	}

	// Absolute visual-line indices: the one the user clicked, and the last one
	// currently visible. Both are relative to the textarea's own scroll offset,
	// which we must read before moving the cursor (moving it scrolls the view).
	target := m.compose.ScrollYOffset() + row
	bottom := m.compose.ScrollYOffset() + height - 1

	m.moveComposeToVisualLine(target, bottom)
	m.setComposeColumnAt(col)
}

// moveComposeToVisualLine parks the cursor on the given absolute visual line.
//
// The only way to address a visual line is to walk from the top, but that
// scrolls the textarea's view to follow the cursor and would leave the box
// scrolled somewhere else than where the user clicked. So we deliberately walk
// past the target down to the last currently-visible line first — which restores
// exactly the original scroll offset, since the textarea scrolls just far enough
// to keep the cursor on screen — and only then step back up to the target, which
// is by definition already visible and so cannot scroll the view again.
func (m *Model) moveComposeToVisualLine(target, bottom int) {
	m.compose.MoveToBegin()
	cur := 0
	for cur < bottom {
		row, col := m.compose.Line(), m.compose.Column()
		m.compose.CursorDown()
		if m.compose.Line() == row && m.compose.Column() == col {
			// End of the content: nothing below to move to.
			break
		}
		cur++
	}
	if target > cur {
		target = cur
	}
	for ; cur > target; cur-- {
		m.compose.CursorUp()
	}
}

// setComposeColumnAt moves the cursor to the given display column of the visual
// line it is already on. It walks the line's runes accumulating their display
// widths so double-width characters are counted as the two cells they occupy,
// and places the cursor before the character covering that cell.
func (m *Model) setComposeColumnAt(col int) {
	li := m.compose.LineInfo()
	runes := m.composeLineRunes()

	// Last rune index that still belongs to this visual line. The textarea's
	// wrapping appends a phantom trailing space to the final segment, so clamp
	// to the real content too. On a soft-wrapped segment we stop one short of
	// the break: that position is also the *start* of the next visual line, and
	// landing there would visibly drop the cursor onto the following row.
	end := li.StartColumn + li.Width
	if li.RowOffset < li.Height-1 && end > li.StartColumn {
		end--
	}
	if end > len(runes) {
		end = len(runes)
	}

	idx, w := li.StartColumn, 0
	for idx < end {
		rw := ansi.StringWidth(string(runes[idx]))
		if w+rw > col {
			break
		}
		w += rw
		idx++
	}
	m.compose.SetCursorColumn(idx)
}

// composeLineRunes returns the runes of the logical line the compose cursor is
// on. LineInfo reports columns as rune indices into that line, so we need it to
// measure their display widths.
func (m Model) composeLineRunes() []rune {
	lines := strings.Split(m.compose.Value(), "\n")
	row := m.compose.Line()
	if row < 0 || row >= len(lines) {
		return nil
	}
	return []rune(lines[row])
}
