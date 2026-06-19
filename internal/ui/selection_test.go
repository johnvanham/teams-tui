package ui

import (
	"testing"
	"time"

	"github.com/jvh/teams-tui/internal/graph"
)

func msg(id, fromID, body string, at time.Time) graph.Message {
	return graph.Message{
		ID:        id,
		CreatedAt: at,
		Body:      graph.MessageBody{ContentType: "text", Content: body},
		From:      &graph.From{User: &graph.User{ID: fromID, DisplayName: fromID}},
	}
}

// modelWithMessages builds a Model whose open chat has the given messages
// already placed in convMsgs (display order), bypassing rendering.
func modelWithMessages(msgs ...graph.Message) Model {
	return Model{
		currentChat: "c1",
		me:          &graph.User{ID: "me"},
		convMsgs:    msgs,
		selectedMsg: len(msgs) - 1,
	}
}

func TestClampSelection(t *testing.T) {
	t0 := time.Now()
	tests := []struct {
		name string
		n    int
		sel  int
		want int
	}{
		{"empty -> none", 0, 0, -1},
		{"out of range high -> last", 2, 5, 1},
		{"negative -> last", 2, -1, 1},
		{"in range kept", 3, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msgs []graph.Message
			for i := 0; i < tt.n; i++ {
				msgs = append(msgs, msg("m", "other", "x", t0))
			}
			m := Model{convMsgs: msgs, selectedMsg: tt.sel}
			m.clampSelection()
			if m.selectedMsg != tt.want {
				t.Errorf("clampSelection() sel = %d, want %d", m.selectedMsg, tt.want)
			}
		})
	}
}

func TestMoveSelection(t *testing.T) {
	t0 := time.Now()
	m := modelWithMessages(
		msg("a", "other", "1", t0),
		msg("b", "other", "2", t0.Add(time.Minute)),
		msg("c", "other", "3", t0.Add(2*time.Minute)),
	)
	// Avoid touching the viewport/rendering: moveSelection calls
	// renderConversation, which needs currentChat in m.messages. Seed it.
	m.messages = map[string][]graph.Message{"c1": m.convMsgs}

	// Start at last (index 2); move up twice -> 0; clamp at top.
	mm, _ := m.moveSelection(-1)
	m = mm.(Model)
	if m.selectedMsg != 1 {
		t.Fatalf("after up: sel = %d, want 1", m.selectedMsg)
	}
	mm, _ = m.moveSelection(-5)
	m = mm.(Model)
	if m.selectedMsg != 0 {
		t.Fatalf("after big up: sel = %d, want 0 (clamped)", m.selectedMsg)
	}
	mm, _ = m.moveSelection(10)
	m = mm.(Model)
	if m.selectedMsg != 2 {
		t.Fatalf("after big down: sel = %d, want 2 (clamped)", m.selectedMsg)
	}
}

func TestEditableSelection(t *testing.T) {
	t0 := time.Now()
	own := msg("a", "me", "mine", t0)
	other := msg("b", "other", "theirs", t0)
	deleted := msg("c", "me", "gone", t0)
	dt := t0
	deleted.DeletedAt = &dt

	t.Run("own message is editable", func(t *testing.T) {
		m := modelWithMessages(other, own)
		m.selectedMsg = 1
		if _, ok := m.editableSelection(); !ok {
			t.Fatal("expected own selected message to be editable")
		}
	})
	t.Run("other's message is not editable", func(t *testing.T) {
		m := modelWithMessages(own, other)
		m.selectedMsg = 1
		if _, ok := m.editableSelection(); ok {
			t.Fatal("other's message should not be editable")
		}
	})
	t.Run("deleted own message is not editable", func(t *testing.T) {
		m := modelWithMessages(other, deleted)
		m.selectedMsg = 1
		if _, ok := m.editableSelection(); ok {
			t.Fatal("deleted message should not be editable")
		}
	})
}

func TestReactToggleDecision(t *testing.T) {
	t0 := time.Now()
	reacted := graph.Message{
		ID:   "a",
		From: &graph.From{User: &graph.User{ID: "other"}},
		Body: graph.MessageBody{ContentType: "text", Content: "hi"},
		Reactions: []graph.Reaction{
			{ReactionType: "👍", User: &graph.From{User: &graph.User{ID: "me"}}},
		},
		CreatedAt: t0,
	}
	// Already reacted with 👍 -> toggling 👍 should remove.
	if !reacted.UserReacted("me", "👍") {
		t.Fatal("expected UserReacted to be true for own 👍")
	}
	// Not reacted with ❤️ -> should add.
	if reacted.UserReacted("me", "❤️") {
		t.Fatal("expected UserReacted to be false for ❤️")
	}
}

func TestMsgAtYMapping(t *testing.T) {
	// Three messages whose headers start at content lines 0, 4, 9.
	m := Model{
		currentChat:  "c1",
		msgLineStart: []int{0, 4, 9},
	}
	// Viewport at offset 0, content rows start at messagesContentTop().
	top := m.messagesContentTop()
	// Force a viewport height so the bounds check passes.
	m.viewport.SetHeight(20)

	tests := []struct {
		y    int
		want int
		ok   bool
	}{
		{top + 0, 0, true},  // first header
		{top + 3, 0, true},  // still within first message's block
		{top + 4, 1, true},  // second header
		{top + 8, 1, true},  // within second
		{top + 9, 2, true},  // third header
		{top + 15, 2, true}, // below last start (still in viewport) -> last message
	}
	for _, tt := range tests {
		got, ok := m.msgAtY(tt.y)
		if ok != tt.ok || (ok && got != tt.want) {
			t.Errorf("msgAtY(y=%d) = (%d,%v), want (%d,%v)", tt.y, got, ok, tt.want, tt.ok)
		}
	}
}
