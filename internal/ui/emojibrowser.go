package ui

import (
	"strings"

	"github.com/jvh/teams-tui/internal/graph"
)

// emojiBrowserMax caps how many emoji rows the browser shows at once; the list
// scrolls to keep the highlighted row visible when there are more matches.
const emojiBrowserMax = 8

// openEmojiBrowser shows the full emoji browser, seeded with the complete emoji
// table so the user can scroll or type to filter. It is opened with "ctrl+:"
// while the compose box is focused; the chosen glyph is inserted at the cursor.
func (m *Model) openEmojiBrowser() {
	m.emojiBrowser = true
	m.browserQuery = ""
	m.browserSel = 0
	m.refreshBrowserMatches()
}

// closeEmojiBrowser hides the browser and clears its transient state.
func (m *Model) closeEmojiBrowser() {
	m.emojiBrowser = false
	m.browserQuery = ""
	m.browserMatches = nil
	m.browserSel = 0
}

// refreshBrowserMatches recomputes the emoji list from the current query. With
// an empty query it lists every emoji (deduplicated by glyph); otherwise it
// filters the full shortcode table by prefix, the same source the composer's
// :shortcode: autocomplete and the reaction picker use. The whole filtered set
// is kept (no cap) so the scrolling view can page through long results.
func (m *Model) refreshBrowserMatches() {
	if m.browserQuery == "" {
		m.browserMatches = graph.AllShortcodes()
	} else {
		m.browserMatches = graph.MatchShortcodePrefix(m.browserQuery, 0)
	}
	if m.browserSel >= len(m.browserMatches) {
		m.browserSel = len(m.browserMatches) - 1
	}
	if m.browserSel < 0 {
		m.browserSel = 0
	}
}

// browserMove moves the highlighted emoji by delta, wrapping at the ends so
// navigation feels continuous.
func (m *Model) browserMove(delta int) {
	n := len(m.browserMatches)
	if n == 0 {
		return
	}
	m.browserSel = (m.browserSel + delta + n) % n
}

// browserWindow returns the slice of matches currently visible and the index of
// the highlighted row within that slice, scrolling so the selection stays on
// screen. It mirrors a simple "keep the cursor visible" viewport.
func (m Model) browserWindow() (rows []graph.EmojiShortcode, selInWindow int) {
	n := len(m.browserMatches)
	if n == 0 {
		return nil, 0
	}
	if n <= emojiBrowserMax {
		return m.browserMatches, m.browserSel
	}
	// Center the selection where possible, clamped to the list bounds.
	start := m.browserSel - emojiBrowserMax/2
	if start < 0 {
		start = 0
	}
	if start > n-emojiBrowserMax {
		start = n - emojiBrowserMax
	}
	return m.browserMatches[start : start+emojiBrowserMax], m.browserSel - start
}

// applyBrowserSelection inserts the highlighted emoji glyph at the compose
// cursor and closes the browser. It returns false (leaving the composer
// untouched) when there is no current match.
func (m *Model) applyBrowserSelection() bool {
	if m.browserSel < 0 || m.browserSel >= len(m.browserMatches) {
		return false
	}
	glyph := m.browserMatches[m.browserSel].Emoji

	// Insert at the current cursor position within the composer, mirroring how
	// applyEmojiSelection splices a glyph into the active line.
	lines := strings.Split(m.compose.Value(), "\n")
	row := m.compose.Line()
	if row < 0 || row >= len(lines) {
		return false
	}
	runes := []rune(lines[row])
	col := m.compose.Column()
	if col > len(runes) {
		col = len(runes)
	}
	lines[row] = string(runes[:col]) + glyph + string(runes[col:])
	m.compose.SetValue(strings.Join(lines, "\n"))

	// SetValue parks the cursor at the buffer end; walk it back to just after
	// the inserted glyph.
	m.compose.MoveToBegin()
	for i := 0; i < row; i++ {
		m.compose.CursorDown()
	}
	m.compose.SetCursorColumn(col + len([]rune(glyph)))
	m.closeEmojiBrowser()
	return true
}
