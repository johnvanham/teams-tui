package ui

import (
	"strings"

	"github.com/jvh/teams-tui/internal/graph"
)

// emojiPickerMax caps how many suggestions the inline popup shows at once.
const emojiPickerMax = 8

// activeEmojiToken inspects the composer's current line up to the cursor and
// returns the in-progress ":name" token the user is typing, if any. It returns
// the token name (without the leading ':') and the rune column where the ':'
// sits. ok is false when the cursor isn't inside a colon-token. A valid token
// has no spaces or extra ':' between the opening ':' and the cursor.
func (m Model) activeEmojiToken() (name string, colonCol int, ok bool) {
	// The text of the line the cursor is on, truncated at the cursor column.
	line := m.currentComposeLine()
	col := m.compose.Column()
	runes := []rune(line)
	if col > len(runes) {
		col = len(runes)
	}
	before := runes[:col]

	// Find the last ':' before the cursor.
	colon := -1
	for i := len(before) - 1; i >= 0; i-- {
		if before[i] == ':' {
			colon = i
			break
		}
		// A space or another colon ends the candidate token region.
		if before[i] == ' ' || before[i] == '\t' {
			return "", 0, false
		}
	}
	if colon < 0 {
		return "", 0, false
	}
	tok := string(before[colon+1:])
	// Only valid token characters (mirrors shortcodeRe's character class).
	for _, r := range tok {
		if !isShortcodeRune(r) {
			return "", 0, false
		}
	}
	return tok, colon, true
}

// currentComposeLine returns the text of the line the cursor is currently on.
func (m Model) currentComposeLine() string {
	lines := strings.Split(m.compose.Value(), "\n")
	row := m.compose.Line()
	if row < 0 || row >= len(lines) {
		return ""
	}
	return lines[row]
}

func isShortcodeRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_' || r == '+' || r == '-'
}

// refreshEmojiPicker recomputes the inline picker state from the composer's
// current token. It opens the popup once the typed token reaches two
// characters (matching the desktop Teams trigger of ':' followed by 2 chars)
// and there is at least one matching emoji; otherwise it closes the popup. This
// is called after every keystroke routed to the composer and never blocks
// typing.
func (m *Model) refreshEmojiPicker() {
	name, _, ok := m.activeEmojiToken()
	if !ok || len([]rune(name)) < 2 {
		m.closeEmojiPicker()
		return
	}
	matches := graph.MatchShortcodePrefix(name, emojiPickerMax)
	if len(matches) == 0 {
		m.closeEmojiPicker()
		return
	}
	// Preserve the highlighted index when the same query is being narrowed,
	// otherwise reset to the top.
	if m.emojiQuery != name {
		m.emojiSel = 0
	}
	if m.emojiSel >= len(matches) {
		m.emojiSel = len(matches) - 1
	}
	m.emojiPicker = true
	m.emojiMatches = matches
	m.emojiQuery = name
}

// closeEmojiPicker hides the popup and clears its transient state.
func (m *Model) closeEmojiPicker() {
	m.emojiPicker = false
	m.emojiMatches = nil
	m.emojiSel = 0
	m.emojiQuery = ""
}

// emojiPickerMove moves the selection within the open popup by delta, wrapping
// at the ends so navigation feels continuous.
func (m *Model) emojiPickerMove(delta int) {
	n := len(m.emojiMatches)
	if n == 0 {
		return
	}
	m.emojiSel = (m.emojiSel + delta + n) % n
}

// applyEmojiSelection replaces the active ":name" token in the composer with
// the highlighted emoji glyph and positions the cursor right after it. It
// returns false (leaving the composer untouched) if there is no current token,
// so callers can fall back to default key handling.
func (m *Model) applyEmojiSelection() bool {
	if !m.emojiPicker || len(m.emojiMatches) == 0 {
		return false
	}
	_, colonCol, ok := m.activeEmojiToken()
	if !ok {
		return false
	}
	glyph := m.emojiMatches[m.emojiSel].Emoji

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
	// Replace runes[colonCol:col] (":name") with the glyph.
	newLine := string(runes[:colonCol]) + glyph + string(runes[col:])
	lines[row] = newLine
	m.compose.SetValue(strings.Join(lines, "\n"))

	// Place the cursor immediately after the inserted glyph. SetValue resets
	// the cursor to the end of the buffer, so walk it back to the right row,
	// then set the column (colon position + the glyph's rune width).
	m.compose.MoveToBegin()
	for i := 0; i < row; i++ {
		m.compose.CursorDown()
	}
	m.compose.SetCursorColumn(colonCol + len([]rune(glyph)))
	m.closeEmojiPicker()
	return true
}

// autoReplaceEmoticon converts a classic text emoticon (":-)", "<3", …) to its
// Unicode glyph the moment it is completed at the cursor, mirroring the desktop
// Teams client's inline auto-replace. It inspects the current line up to the
// cursor; if it ends with a recognized emoticon (preceded by whitespace or the
// line start), the emoticon is swapped for the glyph and the cursor is left
// just after it. No-ops otherwise, so it is safe to call after every keystroke.
func (m *Model) autoReplaceEmoticon() {
	line := m.currentComposeLine()
	col := m.compose.Column()
	runes := []rune(line)
	if col > len(runes) {
		col = len(runes)
	}
	before := string(runes[:col])

	glyph, matchLen, ok := graph.MatchEmoticonSuffix(before)
	if !ok {
		return
	}
	m.spliceEmoticon(before, col, runes, glyph, matchLen)
}

// replaceColonEmoticonBeforeCursor converts a just-completed colon-led emoticon
// (":)", ":p", ":-D", …) to its glyph when the user types a word boundary. The
// cursor is expected to sit immediately after the boundary character (a space or
// newline the textarea has already inserted); this inspects the text before that
// boundary and, if it ends in a colon-emoticon, swaps it for the glyph. This is
// what lets ":p" become 😛 only once finished, so ":party" can be typed first.
func (m *Model) replaceColonEmoticonBeforeCursor() {
	line := m.currentComposeLine()
	col := m.compose.Column()
	runes := []rune(line)
	if col > len(runes) {
		col = len(runes)
	}
	if col == 0 {
		return
	}
	// The boundary is the rune just before the cursor; examine what precedes it.
	boundary := runes[col-1]
	if boundary != ' ' && boundary != '\t' {
		return // only spaces/tabs land here; newlines are handled by the caller
	}
	before := string(runes[:col-1])

	glyph, matchLen, ok := graph.MatchEmoticonBeforeBoundary(before)
	if !ok {
		return
	}
	// Splice the emoticon (which ends at col-1, just before the boundary) but
	// keep the boundary character the user typed.
	emoticon := before[len(before)-matchLen:]
	emoticonRunes := len([]rune(emoticon))
	startCol := (col - 1) - emoticonRunes
	if startCol < 0 {
		return
	}

	lines := strings.Split(m.compose.Value(), "\n")
	row := m.compose.Line()
	if row < 0 || row >= len(lines) {
		return
	}
	newLine := string(runes[:startCol]) + glyph + string(runes[col-1:])
	lines[row] = newLine
	m.compose.SetValue(strings.Join(lines, "\n"))

	m.compose.MoveToBegin()
	for i := 0; i < row; i++ {
		m.compose.CursorDown()
	}
	// Cursor goes after the glyph + the preserved boundary character.
	m.compose.SetCursorColumn(startCol + len([]rune(glyph)) + 1)
}

// spliceEmoticon replaces the matched emoticon ending at col on the current line
// with glyph and repositions the cursor just after it. before/runes/col describe
// the line up to the cursor; matchLen is the emoticon's byte length within
// before.
func (m *Model) spliceEmoticon(before string, col int, runes []rune, glyph string, matchLen int) {
	emoticon := before[len(before)-matchLen:]
	emoticonRunes := len([]rune(emoticon))
	startCol := col - emoticonRunes

	lines := strings.Split(m.compose.Value(), "\n")
	row := m.compose.Line()
	if row < 0 || row >= len(lines) {
		return
	}
	newLine := string(runes[:startCol]) + glyph + string(runes[col:])
	lines[row] = newLine
	m.compose.SetValue(strings.Join(lines, "\n"))

	// SetValue parks the cursor at the buffer end; walk it back to just after
	// the inserted glyph.
	m.compose.MoveToBegin()
	for i := 0; i < row; i++ {
		m.compose.CursorDown()
	}
	m.compose.SetCursorColumn(startCol + len([]rune(glyph)))
}
