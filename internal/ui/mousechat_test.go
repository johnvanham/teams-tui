package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The horizontal wheel flips to the next/previous chat, moving both the
// highlight and the open conversation.
func TestStepChatMovesToAdjacentChat(t *testing.T) {
	m := newChatListModel(t, "a", "a", "b", "c")
	m.selectChatInList("a")

	m.stepChat(1)
	if got := highlighted(m); got != "b" {
		t.Errorf("after stepChat(1): highlighted = %q, want %q", got, "b")
	}
	if m.currentChat != "b" {
		t.Errorf("after stepChat(1): currentChat = %q, want %q", m.currentChat, "b")
	}

	m.stepChat(-1)
	if got := highlighted(m); got != "a" {
		t.Errorf("after stepChat(-1): highlighted = %q, want %q", got, "a")
	}
	if m.currentChat != "a" {
		t.Errorf("after stepChat(-1): currentChat = %q, want %q", m.currentChat, "a")
	}
}

// Scrolling past either end of the list keeps the current chat rather than
// wrapping around to the other end.
func TestStepChatStopsAtEnds(t *testing.T) {
	m := newChatListModel(t, "a", "a", "b")
	m.selectChatInList("a")
	if cmd := m.stepChat(-1); cmd != nil {
		t.Error("stepChat(-1) at the top returned a command, want nil")
	}
	if m.currentChat != "a" {
		t.Errorf("currentChat = %q, want %q (unchanged)", m.currentChat, "a")
	}

	m.stepChat(1) // -> b, the last chat
	if cmd := m.stepChat(1); cmd != nil {
		t.Error("stepChat(1) at the bottom returned a command, want nil")
	}
	if m.currentChat != "b" {
		t.Errorf("currentChat = %q, want %q (unchanged)", m.currentChat, "b")
	}
}

// stepChat walks the filtered view: with a filter applied, only matching chats
// are reachable.
func TestStepChatRespectsFilter(t *testing.T) {
	m := newChatListModel(t, "alpha", "alpha", "beta", "gamma")
	m.list.SetFilterText("a")
	m.selectChatInList("alpha")

	m.stepChat(1)
	if got := highlighted(m); got == "" {
		t.Fatal("highlight left the filtered view")
	}
	for _, it := range m.list.VisibleItems() {
		if c, ok := it.(chatItem); ok && c.chat.ID == m.currentChat {
			return // landed on a visible (matching) chat
		}
	}
	t.Errorf("currentChat = %q is not in the filtered view", m.currentChat)
}

// Clicking anywhere in the sidebar activates the chats pane, including the
// column's border cells and rows that hold no chat item.
func TestSidebarClickFocusesChats(t *testing.T) {
	for _, tc := range []struct {
		name string
		x, y int
	}{
		{"item row", 5, 3},
		{"left border", 0, 3},
		{"right border", sidebarWidth - 1, 3},
		{"header row, no item", 5, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newChatListModel(t, "a", "a", "b", "c")
			m.phase = phaseReady
			m.focus = focusMessages

			got, _ := m.handleMouseClick(tea.MouseClickMsg{X: tc.x, Y: tc.y, Button: tea.MouseLeft})
			mm, ok := got.(Model)
			if !ok {
				t.Fatalf("handleMouseClick returned %T, want Model", got)
			}
			if mm.focus != focusChats {
				t.Errorf("focus = %v, want focusChats", mm.focus)
			}
		})
	}
}

// A click on the chrome below the list (status bar, help, legend) shares the
// sidebar's X range but must not open a chat from the next page.
func TestChatIndexAtYBelowListIsNoItem(t *testing.T) {
	m := newChatListModel(t, "a", "a", "b", "c")
	below := m.list.Paginator.PerPage*m.delegateRows() + titleHeight + 1 + sidebarHeaderRows
	if got := m.chatIndexAtY(below); got != -1 {
		t.Errorf("chatIndexAtY(%d) = %d, want -1", below, got)
	}
}
