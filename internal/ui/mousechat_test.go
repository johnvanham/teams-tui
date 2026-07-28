package ui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// pagedChatListModel returns a Model whose sidebar holds n chats ("c0".."cN")
// spread over at least three pages, plus the resulting page size. The list's
// own pagination math decides how many rows fit, so the page size is read back
// rather than assumed.
func pagedChatListModel(t *testing.T, n, rows int) (*Model, int) {
	t.Helper()
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		ids = append(ids, fmt.Sprintf("c%d", i))
	}
	m := newChatListModel(t, ids[0], ids...)
	// Each item spans delegateRows() screen rows, so height sets the page size.
	m.list.SetSize(30, rows*m.delegateRows())
	perPage := m.list.Paginator.PerPage
	if perPage < 2 || m.list.Paginator.TotalPages < 3 {
		t.Fatalf("setup: PerPage = %d over %d pages, want >=2 over >=3",
			perPage, m.list.Paginator.TotalPages)
	}
	m.selectChatInList(ids[0])
	return m, perPage
}

// The horizontal wheel flips a whole page at a time and lands on the chat at
// the top of that page, opening it.
func TestPageChatsMovesAPageAtATime(t *testing.T) {
	m, perPage := pagedChatListModel(t, 20, 5)
	top := fmt.Sprintf("c%d", perPage) // first chat on page 2

	m.pageChats(1)
	if got := m.list.Paginator.Page; got != 1 {
		t.Errorf("after pageChats(1): page = %d, want 1", got)
	}
	if got := highlighted(m); got != top {
		t.Errorf("after pageChats(1): highlighted = %q, want %q (top of page 2)", got, top)
	}
	if m.currentChat != top {
		t.Errorf("after pageChats(1): currentChat = %q, want %q", m.currentChat, top)
	}

	m.pageChats(-1)
	if got := m.list.Paginator.Page; got != 0 {
		t.Errorf("after pageChats(-1): page = %d, want 0", got)
	}
	if got := highlighted(m); got != "c0" {
		t.Errorf("after pageChats(-1): highlighted = %q, want %q", got, "c0")
	}
}

// Paging past either end keeps the current page and chat rather than wrapping.
func TestPageChatsStopsAtEnds(t *testing.T) {
	m, _ := pagedChatListModel(t, 20, 5)

	if cmd := m.pageChats(-1); cmd != nil {
		t.Error("pageChats(-1) on the first page returned a command, want nil")
	}
	if m.currentChat != "c0" {
		t.Errorf("currentChat = %q, want %q (unchanged)", m.currentChat, "c0")
	}

	last := m.list.Paginator.TotalPages - 1
	for i := 0; i < last; i++ {
		m.pageChats(1)
	}
	if got := m.list.Paginator.Page; got != last {
		t.Fatalf("setup: page = %d, want %d", got, last)
	}
	if cmd := m.pageChats(1); cmd != nil {
		t.Error("pageChats(1) on the last page returned a command, want nil")
	}
	if got := m.list.Paginator.Page; got != last {
		t.Errorf("page = %d, want %d (unchanged)", got, last)
	}
}

// pageChats pages the filtered view: only matching chats are reachable.
func TestPageChatsRespectsFilter(t *testing.T) {
	m := newChatListModel(t, "alpha", "alpha", "beta", "gamma")
	m.list.SetFilterText("a")
	m.list.SetSize(30, m.delegateRows()) // one chat per page
	m.selectChatInList("alpha")

	m.pageChats(1)

	for _, it := range m.list.VisibleItems() {
		if c, ok := it.(chatItem); ok && c.chat.ID == m.currentChat {
			return // landed on a visible (matching) chat
		}
	}
	t.Errorf("currentChat = %q is not in the filtered view", m.currentChat)
}

// The wheel must not scroll the chat list: every cursor move there opens a
// chat, so scrolling would be a stream of message loads.
func TestSidebarWheelDoesNotMoveChatList(t *testing.T) {
	for _, btn := range []tea.MouseButton{tea.MouseWheelUp, tea.MouseWheelDown} {
		m, _ := pagedChatListModel(t, 20, 5)
		m.phase = phaseReady

		got, cmd := m.handleMouseWheel(tea.MouseWheelMsg{X: 5, Y: 3, Button: btn})
		mm, ok := got.(Model)
		if !ok {
			t.Fatalf("handleMouseWheel returned %T, want Model", got)
		}
		if cmd != nil {
			t.Errorf("button %v: returned a command, want nil", btn)
		}
		if mm.list.Index() != 0 || mm.currentChat != "c0" {
			t.Errorf("button %v: index = %d, currentChat = %q, want 0/%q",
				btn, mm.list.Index(), mm.currentChat, "c0")
		}
	}
}

// wheel feeds one wheel event through the update path, folding the returned
// model back into m the way the Bubble Tea runtime does.
func wheel(t *testing.T, m *Model, btn tea.MouseButton) tea.Cmd {
	t.Helper()
	got, cmd := m.handleMouseWheel(tea.MouseWheelMsg{X: 5, Y: 3, Button: btn})
	mm, ok := got.(Model)
	if !ok {
		t.Fatalf("handleMouseWheel returned %T, want Model", got)
	}
	*m = mm
	return cmd
}

// A trackpad reports one sideways swipe as a burst of notches (plus momentum),
// so only the first one may turn a page; paging resumes once the wheel goes
// quiet again.
func TestHorizontalWheelPagesOncePerSwipe(t *testing.T) {
	m, _ := pagedChatListModel(t, 20, 5)
	m.phase = phaseReady

	wheel(t, m, tea.MouseWheelRight)
	if got := m.list.Paginator.Page; got != 1 {
		t.Fatalf("after the first notch: page = %d, want 1", got)
	}
	// The rest of the burst arrives immediately and must be swallowed.
	for i := 0; i < 20; i++ {
		wheel(t, m, tea.MouseWheelRight)
	}
	if got := m.list.Paginator.Page; got != 1 {
		t.Errorf("after the burst: page = %d, want 1 (one page per swipe)", got)
	}

	// Once the gesture has settled, the next swipe pages again.
	m.lastChatPageWheel = time.Now().Add(-chatPageWheelQuiet)
	wheel(t, m, tea.MouseWheelRight)
	if got := m.list.Paginator.Page; got != 2 {
		t.Errorf("after the wheel settled: page = %d, want 2", got)
	}
}

// Swiping back the other way is a new gesture, not momentum, so it pages at
// once rather than waiting for the quiet period.
func TestHorizontalWheelReversalPagesImmediately(t *testing.T) {
	m, _ := pagedChatListModel(t, 20, 5)
	m.phase = phaseReady

	wheel(t, m, tea.MouseWheelRight)
	wheel(t, m, tea.MouseWheelRight) // swallowed
	if got := m.list.Paginator.Page; got != 1 {
		t.Fatalf("setup: page = %d, want 1", got)
	}

	wheel(t, m, tea.MouseWheelLeft)
	if got := m.list.Paginator.Page; got != 0 {
		t.Errorf("after reversing: page = %d, want 0", got)
	}
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
