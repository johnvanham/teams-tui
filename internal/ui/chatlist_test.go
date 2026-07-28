package ui

import (
	"testing"
	"time"

	"charm.land/bubbles/v2/list"

	"github.com/jvh/teams-tui/internal/graph"
)

// newChatListModel returns a Model with a usable sidebar list holding the given
// chats (in the given order), with chatID open.
func newChatListModel(t *testing.T, open string, ids ...string) *Model {
	t.Helper()
	l := list.New(nil, newChatDelegate(), 0, 0)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowTitle(false)
	l.SetFilteringEnabled(true)
	l.SetSize(30, 40)

	m := &Model{
		list:        l,
		chats:       make(map[string]graph.Chat),
		chatOrder:   append([]string(nil), ids...),
		currentChat: open,
		readUntil:   make(map[string]time.Time),
	}
	now := time.Now()
	for i, id := range ids {
		m.chats[id] = graph.Chat{
			ID:       id,
			ChatType: graph.ChatOneOnOne,
			// Distinct activity times so the ordering is unambiguous.
			LastUpdatedDateTime: now.Add(time.Duration(-i) * time.Minute),
			Members: []graph.ConversationMember{
				{UserID: id, DisplayName: id},
			},
		}
	}
	m.rebuildChatList()
	return m
}

// highlighted returns the chat ID of the row the sidebar currently highlights.
func highlighted(m *Model) string {
	it, ok := m.list.SelectedItem().(chatItem)
	if !ok {
		return ""
	}
	return it.chat.ID
}

// A poll that reorders the chats (they sort by last activity, so any incoming
// message reshuffles them) must not leave the highlight on a different chat
// than the one the message pane shows.
func TestRebuildChatListKeepsHighlightOnOpenChat(t *testing.T) {
	m := newChatListModel(t, "b", "a", "b", "c")
	m.selectChatInList("b")
	if got := highlighted(m); got != "b" {
		t.Fatalf("setup: highlighted = %q, want %q", got, "b")
	}

	// "c" gets a new message and jumps to the top: b moves from row 1 to row 2.
	m.chatOrder = []string{"c", "a", "b"}
	m.rebuildChatList()

	if got := highlighted(m); got != "b" {
		t.Errorf("after reorder: highlighted = %q, want %q (currentChat)", got, "b")
	}
	if got := m.list.Index(); got != 2 {
		t.Errorf("after reorder: index = %d, want 2", got)
	}
}

// A new chat appearing at the top of the list shifts every other row down; the
// highlight must follow the open chat rather than stay on its old row number.
func TestRebuildChatListFollowsOpenChatOnInsert(t *testing.T) {
	m := newChatListModel(t, "a", "a", "b")
	m.selectChatInList("a")

	m.chatOrder = []string{"new", "a", "b"}
	m.chats["new"] = graph.Chat{ID: "new", ChatType: graph.ChatOneOnOne}
	m.rebuildChatList()

	if got := highlighted(m); got != "a" {
		t.Errorf("highlighted = %q, want %q", got, "a")
	}
}

// With no chat open (startup, before the first auto-open) the highlight is left
// where it is rather than being reset.
func TestRebuildChatListNoOpenChatLeavesHighlight(t *testing.T) {
	m := newChatListModel(t, "", "a", "b", "c")
	m.list.Select(2)

	m.chatOrder = []string{"c", "a", "b"}
	m.rebuildChatList()

	if got := m.list.Index(); got != 2 {
		t.Errorf("index = %d, want 2 (unchanged)", got)
	}
}

// selectChatInList indexes the visible (filtered) items, not the full chat
// order — those diverge as soon as a filter is applied.
func TestSelectChatInListUsesVisibleItems(t *testing.T) {
	m := newChatListModel(t, "", "alpha", "beta", "gamma")
	// Filter down to "gamma" only; it is row 0 of the filtered view but row 2
	// of m.chatOrder.
	m.list.SetFilterText("gamma")

	m.selectChatInList("gamma")

	if got := highlighted(m); got != "gamma" {
		t.Errorf("highlighted = %q, want %q", got, "gamma")
	}
}

// Graph can hand back the same chat on two pages of one listing (it pages over
// a mutable sort key), and rebuildChatList emits one row per chatOrder entry —
// so handleChats must collapse repeats or the sidebar shows the conversation
// twice and every row below it shifts.
func TestHandleChatsDedupesRepeatedChatIDs(t *testing.T) {
	m := newChatListModel(t, "a", "a", "b")
	m.phase = phaseReady
	m.notifiedUntil = make(map[string]time.Time)
	m.lastSync = make(map[string]time.Time)

	now := time.Now()
	chat := func(id string, ago time.Duration) graph.Chat {
		return graph.Chat{
			ID:                  id,
			ChatType:            graph.ChatOneOnOne,
			LastUpdatedDateTime: now.Add(-ago),
			Members:             []graph.ConversationMember{{UserID: id, DisplayName: id}},
		}
	}
	next, _ := m.handleChats(chatsMsg{chats: []graph.Chat{
		chat("a", time.Minute),
		chat("b", 2*time.Minute),
		chat("a", time.Minute), // same chat repeated by the server
	}})
	got := next.(Model)

	if len(got.chatOrder) != 2 {
		t.Fatalf("chatOrder = %v, want 2 entries", got.chatOrder)
	}
	if got.chatOrder[0] != "a" || got.chatOrder[1] != "b" {
		t.Errorf("chatOrder = %v, want [a b]", got.chatOrder)
	}
	if n := len(got.list.Items()); n != 2 {
		t.Errorf("list items = %d, want 2", n)
	}
}

// A chat that isn't visible can't be highlighted; selecting it must be a no-op
// rather than moving the cursor to an unrelated row.
func TestSelectChatInListUnknownChatIsNoop(t *testing.T) {
	m := newChatListModel(t, "", "a", "b", "c")
	m.list.Select(1)

	m.selectChatInList("missing")

	if got := m.list.Index(); got != 1 {
		t.Errorf("index = %d, want 1 (unchanged)", got)
	}
}
