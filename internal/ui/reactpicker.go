package ui

import "github.com/jvh/teams-tui/internal/graph"

// reactPickerMax caps how many emoji the reaction picker shows at once.
const reactPickerMax = 8

// openReactPicker shows the reaction emoji picker for the given message, seeded
// with a small default set of common reactions so the user can react instantly
// without typing.
func (m *Model) openReactPicker(msgID string) {
	m.reactPicker = true
	m.reactMsgID = msgID
	m.reactQuery = ""
	m.reactSel = 0
	m.refreshReactMatches()
}

// closeReactPicker hides the reaction picker and clears its transient state.
func (m *Model) closeReactPicker() {
	m.reactPicker = false
	m.reactMsgID = ""
	m.reactQuery = ""
	m.reactMatches = nil
	m.reactSel = 0
}

// refreshReactMatches recomputes the emoji list from the current query. With an
// empty query it shows a curated set of common reactions; otherwise it filters
// the full shortcode table by prefix (the same source the composer's :shortcode:
// autocomplete uses).
func (m *Model) refreshReactMatches() {
	if m.reactQuery == "" {
		m.reactMatches = defaultReactions()
	} else {
		m.reactMatches = graph.MatchShortcodePrefix(m.reactQuery, reactPickerMax)
	}
	if m.reactSel >= len(m.reactMatches) {
		m.reactSel = len(m.reactMatches) - 1
	}
	if m.reactSel < 0 {
		m.reactSel = 0
	}
}

// reactPickerMove moves the highlighted emoji by delta, wrapping at the ends.
func (m *Model) reactPickerMove(delta int) {
	n := len(m.reactMatches)
	if n == 0 {
		return
	}
	m.reactSel = (m.reactSel + delta + n) % n
}

// selectedReaction returns the currently highlighted emoji glyph, if any.
func (m Model) selectedReaction() (string, bool) {
	if m.reactSel < 0 || m.reactSel >= len(m.reactMatches) {
		return "", false
	}
	return m.reactMatches[m.reactSel].Emoji, true
}

// defaultReactions is the quick-pick set shown when the reaction picker opens
// with no search text: Teams' standard six reactions plus a couple of common
// extras. Names match shortcodes so typing narrows naturally.
func defaultReactions() []graph.EmojiShortcode {
	return []graph.EmojiShortcode{
		{Name: "thumbsup", Emoji: "👍"},
		{Name: "heart", Emoji: "❤️"},
		{Name: "laughing", Emoji: "😆"},
		{Name: "open_mouth", Emoji: "😮"},
		{Name: "cry", Emoji: "😢"},
		{Name: "rage", Emoji: "😡"},
	}
}
