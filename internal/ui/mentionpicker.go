package ui

import (
	"sort"
	"strings"

	"github.com/jvh/teams-tui/internal/graph"
)

// mentionPickerMax caps how many name suggestions the inline popup shows.
const mentionPickerMax = 8

// mentionTriggerMax bounds how far back from the cursor we look for the "@"
// trigger. Display names can contain spaces, so we can't stop at the first
// space; instead we cap the in-progress query length to keep the scan cheap and
// avoid treating an "@" far up the line as an open mention.
const mentionTriggerMax = 40

// activeMentionToken inspects the composer's current line up to the cursor and
// returns the in-progress "@query" the user is typing, if any. It returns the
// query text (without the leading "@") and the rune column where the "@" sits.
// ok is false when the cursor isn't inside a mention token.
//
// Because display names contain spaces, the query may include spaces; the scan
// walks back to the nearest "@" that is at the start of the line or preceded by
// whitespace (so emails like "a@b" are not treated as mentions) and within
// mentionTriggerMax runes of the cursor.
func (m Model) activeMentionToken() (query string, atCol int, ok bool) {
	line := m.currentComposeLine()
	col := m.compose.Column()
	runes := []rune(line)
	if col > len(runes) {
		col = len(runes)
	}
	before := runes[:col]

	lo := len(before) - mentionTriggerMax
	if lo < 0 {
		lo = 0
	}
	for i := len(before) - 1; i >= lo; i-- {
		if before[i] == '@' {
			// The '@' must start a word: at line start or after whitespace.
			if i == 0 || before[i-1] == ' ' || before[i-1] == '\t' {
				return string(before[i+1:]), i, true
			}
			return "", 0, false
		}
		if before[i] == '\n' {
			break
		}
	}
	return "", 0, false
}

// refreshMentionPicker recomputes the mention popup from the composer's current
// "@query". It opens once an "@" is present (even with an empty query, so the
// full participant list is offered immediately) in a group chat with other
// members and there is at least one name match; otherwise it closes. Called
// after every keystroke routed to the composer.
func (m *Model) refreshMentionPicker() {
	query, _, ok := m.activeMentionToken()
	if !ok {
		m.closeMentionPicker()
		return
	}
	matches := m.matchMembers(query, mentionPickerMax)
	if len(matches) == 0 {
		m.closeMentionPicker()
		return
	}
	if m.mentionQuery != query {
		m.mentionSel = 0
	}
	if m.mentionSel >= len(matches) {
		m.mentionSel = len(matches) - 1
	}
	m.mentionPicker = true
	m.mentionMatches = matches
	m.mentionQuery = query
}

// matchMembers returns up to limit participants of the current chat whose
// display name matches query (case-insensitive). An empty query lists everyone;
// otherwise names that match on a word boundary (start of the name or any
// internal word) rank first, then any substring match. The signed-in user is
// excluded. Results are de-duplicated by user id.
func (m Model) matchMembers(query string, limit int) []graph.ConversationMember {
	chat, ok := m.chats[m.currentChat]
	if !ok {
		return nil
	}
	// Mentions only make sense in multi-party chats.
	if chat.ChatType == graph.ChatOneOnOne {
		return nil
	}
	self := m.selfID()
	q := strings.ToLower(strings.TrimSpace(query))

	type ranked struct {
		mem  graph.ConversationMember
		rank int // 0 = word-boundary match, 1 = substring match
	}
	var out []ranked
	seen := make(map[string]bool)
	for _, mem := range chat.Members {
		if mem.DisplayName == "" || mem.UserID == "" || mem.UserID == self || seen[mem.UserID] {
			continue
		}
		name := strings.ToLower(mem.DisplayName)
		rank := -1
		switch {
		case q == "":
			rank = 0
		case wordPrefixMatch(name, q):
			rank = 0
		case strings.Contains(name, q):
			rank = 1
		}
		if rank < 0 {
			continue
		}
		seen[mem.UserID] = true
		out = append(out, ranked{mem, rank})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].rank != out[j].rank {
			return out[i].rank < out[j].rank
		}
		return out[i].mem.DisplayName < out[j].mem.DisplayName
	})
	res := make([]graph.ConversationMember, 0, len(out))
	for _, r := range out {
		res = append(res, r.mem)
		if limit > 0 && len(res) >= limit {
			break
		}
	}
	return res
}

// wordPrefixMatch reports whether q is a prefix of name or of any word within
// name (so "lov" matches "Ada Lovelace"). name and q must already be lowercased.
func wordPrefixMatch(name, q string) bool {
	if strings.HasPrefix(name, q) {
		return true
	}
	for _, word := range strings.Fields(name) {
		if strings.HasPrefix(word, q) {
			return true
		}
	}
	return false
}

// closeMentionPicker hides the popup and clears its transient state. It does not
// touch the accumulated mentions slice (those persist until the message sends).
func (m *Model) closeMentionPicker() {
	m.mentionPicker = false
	m.mentionMatches = nil
	m.mentionSel = 0
	m.mentionQuery = ""
}

// mentionPickerMove moves the selection within the open popup by delta, wrapping
// at the ends.
func (m *Model) mentionPickerMove(delta int) {
	n := len(m.mentionMatches)
	if n == 0 {
		return
	}
	m.mentionSel = (m.mentionSel + delta + n) % n
}

// applyMentionSelection replaces the active "@query" token with the highlighted
// participant's "@DisplayName " (note the trailing space) and records the
// mention so a Graph @-mention is attached when the message is sent. It returns
// false (leaving the composer untouched) if there is no current token.
func (m *Model) applyMentionSelection() bool {
	if !m.mentionPicker || len(m.mentionMatches) == 0 {
		return false
	}
	_, atCol, ok := m.activeMentionToken()
	if !ok {
		return false
	}
	mem := m.mentionMatches[m.mentionSel]
	insert := "@" + mem.DisplayName + " "

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
	// Replace runes[atCol:col] ("@query") with "@DisplayName ".
	lines[row] = string(runes[:atCol]) + insert + string(runes[col:])
	m.compose.SetValue(strings.Join(lines, "\n"))

	// Reposition the cursor right after the inserted text (SetValue parks it at
	// the buffer end).
	m.compose.MoveToBegin()
	for i := 0; i < row; i++ {
		m.compose.CursorDown()
	}
	m.compose.SetCursorColumn(atCol + len([]rune(insert)))

	m.recordMention(mem)
	m.closeMentionPicker()
	return true
}

// recordMention adds a mention for the given member to the pending-message set,
// de-duplicating by user id so completing the same name twice doesn't add two
// payload entries for one visible tag (ComposeHTMLWithMentions still emits one
// <at> per textual occurrence).
func (m *Model) recordMention(mem graph.ConversationMember) {
	for _, existing := range m.mentions {
		if existing.UserID == mem.UserID {
			return
		}
	}
	m.mentions = append(m.mentions, graph.Mention{
		DisplayName: mem.DisplayName,
		UserID:      mem.UserID,
	})
}

// clearMentions drops the pending-message mention set, called whenever the
// compose box is reset (after a send or an esc clear).
func (m *Model) clearMentions() {
	m.mentions = nil
}
