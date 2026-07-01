package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/jvh/teams-tui/internal/ui/styles"
)

// This file implements mouse text selection over the messages viewport: the
// user presses and drags to highlight a run of characters, which can then be
// copied to the OS clipboard (y) or quoted into a reply (q). Selection works on
// the rendered conversation kept in Model.selContent (one styled string per
// viewport content line). Coordinates are (line, col): a 0-based content-line
// index and a 0-based display column (terminal cell), matching the geometry
// used by the click-mapping helpers in update.go.

// selPoint is one endpoint of a selection: a content line and a display column.
type selPoint struct {
	line int
	col  int
}

// less reports whether point a comes before point b in reading order (earlier
// line, or same line and earlier column).
func (a selPoint) less(b selPoint) bool {
	if a.line != b.line {
		return a.line < b.line
	}
	return a.col < b.col
}

// selectionAt maps a screen coordinate to a selection point in the conversation
// content. It converts the screen row to a viewport content line (accounting for
// the scroll offset) and clamps the column to the line's rendered width so a
// click past the end of a short line selects to its end. ok is false when the
// point falls outside the messages viewport.
func (m Model) selectionAt(x, y int) (selPoint, bool) {
	top := m.messagesContentTop()
	// Content starts after the sidebar, the pane's left border, and its 1-cell
	// left padding.
	left := sidebarWidth + 1 /*border*/ + 1 /*padding*/
	rowInPane := y - top
	if rowInPane < 0 || rowInPane >= m.viewport.Height() {
		return selPoint{}, false
	}
	line := m.viewport.YOffset() + rowInPane
	if line < 0 {
		return selPoint{}, false
	}
	col := x - left
	if col < 0 {
		col = 0
	}
	// Clamp the column to the line's width so selecting past the end of a short
	// line stops at its last character rather than trailing into empty space.
	if line < len(m.selContent) {
		if w := ansi.StringWidth(m.selContent[line]); col > w {
			col = w
		}
	}
	return selPoint{line: line, col: col}, true
}

// selectionBounds returns the ordered (start, end) endpoints of the current
// selection, with start <= end in reading order.
func (m Model) selectionBounds() (selPoint, selPoint) {
	a := selPoint{line: m.selAnchorLn, col: m.selAnchorCol}
	b := selPoint{line: m.selCurLn, col: m.selCurCol}
	if b.less(a) {
		return b, a
	}
	return a, b
}

// hasSelection reports whether a non-empty selection is currently held (the
// anchor and cursor differ).
func (m Model) hasSelection() bool {
	if !m.selecting {
		return false
	}
	start, end := m.selectionBounds()
	return start != end
}

// clearSelection drops any active mouse selection.
func (m *Model) clearSelection() {
	m.selecting = false
	m.selAnchorLn, m.selAnchorCol = 0, 0
	m.selCurLn, m.selCurCol = 0, 0
}

// selectionText returns the plain text of the current selection. Content lines
// that are soft word-wrap continuations of the previous line (per selWrapCont)
// are rejoined with a single space rather than a newline, so copying wrapped
// prose yields the original single line; real newlines in the source are
// preserved. Returns "" when there is no selection.
func (m Model) selectionText() string {
	if !m.hasSelection() {
		return ""
	}
	start, end := m.selectionBounds()
	var b strings.Builder
	for line := start.line; line <= end.line && line < len(m.selContent); line++ {
		plain := ansi.Strip(m.selContent[line])
		width := ansi.StringWidth(plain)
		lo, hi := 0, width
		if line == start.line {
			lo = start.col
		}
		if line == end.line {
			hi = end.col
		}
		if lo > width {
			lo = width
		}
		if hi > width {
			hi = width
		}
		// Choose the separator before this line: a soft-wrap continuation
		// rejoins with a space; anything else is a real line break. The very
		// first selected line gets no separator.
		if line > start.line {
			if m.isWrapContinuation(line) {
				b.WriteByte(' ')
			} else {
				b.WriteByte('\n')
			}
		}
		if lo < hi {
			b.WriteString(ansi.Cut(plain, lo, hi))
		}
	}
	return b.String()
}

// isWrapContinuation reports whether content line i is a soft word-wrap
// continuation of the previous line (so it should rejoin with a space during
// text extraction). Bounds-checked so a stale index is treated as a hard break.
func (m Model) isWrapContinuation(i int) bool {
	return i >= 0 && i < len(m.selWrapCont) && m.selWrapCont[i]
}

// applySelectionHighlight overlays the selection highlight onto the rendered
// conversation content. For each content line touched by the selection it emits
// the pre-selection span, the selected span styled with SelectionHighlight, then
// the post-selection span. Lines outside the selection are passed through
// untouched, so syntax colouring elsewhere is preserved. Splitting is done with
// ansi.Cut so styled (escape-carrying) spans are cut on display columns without
// breaking escape sequences.
func (m Model) applySelectionHighlight(content string) string {
	start, end := m.selectionBounds()
	lines := strings.Split(content, "\n")
	for line := start.line; line <= end.line && line < len(lines); line++ {
		src := lines[line]
		width := ansi.StringWidth(src)
		lo, hi := 0, width
		if line == start.line {
			lo = start.col
		}
		if line == end.line {
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
		// Highlight the plain text of the selected span so the highlight colour
		// isn't fighting per-token syntax colours underneath it.
		mid := ansi.Strip(ansi.Cut(src, lo, hi))
		after := ansi.Cut(src, hi, width)
		lines[line] = before + styles.SelectionHighlight.Render(mid) + after
	}
	return strings.Join(lines, "\n")
}

// selectionWithinMessages reports whether a screen X coordinate falls in the
// messages viewport's content column (right of the sidebar, left of the scroll
// gutter). Used to decide whether a drag is a text selection.
func (m Model) selectionWithinMessages(x int) bool {
	left := sidebarWidth + 1 /*border*/ + 1 /*padding*/
	// The right edge of the selectable text is the viewport content width; past
	// it sit the scrollbar gutter, right padding, and border.
	right := left + m.viewport.Width()
	return x >= left && x < right
}
