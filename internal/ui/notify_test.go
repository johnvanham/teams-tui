package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvh/teams-tui/internal/config"
	"github.com/jvh/teams-tui/internal/graph"
	"github.com/jvh/teams-tui/internal/notify"
)

// newTestModel returns a minimal Model wired with a capturing notifier, enough
// to exercise the notification gating logic.
func newTestModel() (*Model, *[]notify.Capture) {
	n, got := notify.NewCapturing()
	m := &Model{
		notifier:      n,
		focused:       true,
		me:            &graph.User{ID: "me"},
		chats:         make(map[string]graph.Chat),
		notifiedUntil: make(map[string]time.Time),
		cfg:           &config.Config{FocusCommand: "-"}, // disable window raising in tests
		sender:        &programSender{},
	}
	return m, got
}

func chatWithPreview(id, fromID, body string, at time.Time) graph.Chat {
	return graph.Chat{
		ID:       id,
		ChatType: graph.ChatOneOnOne,
		Members: []graph.ConversationMember{
			{UserID: "me", DisplayName: "Me"},
			{UserID: "other", DisplayName: "Other Person"},
		},
		LastMessagePreview: &graph.MessagePreview{
			CreatedAt: at,
			Body:      graph.MessageBody{ContentType: "text", Content: body},
			From:      &graph.From{User: &graph.User{ID: fromID, DisplayName: "Other Person"}},
		},
	}
}

func TestNotifyNewChatMessages(t *testing.T) {
	t0 := time.Now()

	t.Run("baseline poll does not notify", func(t *testing.T) {
		m, got := newTestModel()
		chats := []graph.Chat{chatWithPreview("c1", "other", "hi", t0)}
		m.notifyNewChatMessages(chats, true)
		if len(*got) != 0 {
			t.Fatalf("baseline poll notified %d times, want 0", len(*got))
		}
	})

	t.Run("new message from other notifies once", func(t *testing.T) {
		m, got := newTestModel()
		base := []graph.Chat{chatWithPreview("c1", "other", "hi", t0)}
		m.notifyNewChatMessages(base, true) // baseline

		next := []graph.Chat{chatWithPreview("c1", "other", "are you there?", t0.Add(time.Minute))}
		m.notifyNewChatMessages(next, false)
		if len(*got) != 1 {
			t.Fatalf("got %d notifications, want 1", len(*got))
		}
		if (*got)[0].Message != "are you there?" {
			t.Errorf("message = %q, want %q", (*got)[0].Message, "are you there?")
		}
		if (*got)[0].Title == "" {
			t.Error("title should be the chat name, got empty")
		}

		// Re-polling the same preview must not notify again.
		m.notifyNewChatMessages(next, false)
		if len(*got) != 1 {
			t.Fatalf("duplicate poll notified again: %d total, want 1", len(*got))
		}
	})

	t.Run("own message does not notify", func(t *testing.T) {
		m, got := newTestModel()
		base := []graph.Chat{chatWithPreview("c1", "other", "hi", t0)}
		m.notifyNewChatMessages(base, true)

		mine := []graph.Chat{chatWithPreview("c1", "me", "my reply", t0.Add(time.Minute))}
		m.notifyNewChatMessages(mine, false)
		if len(*got) != 0 {
			t.Fatalf("own message notified %d times, want 0", len(*got))
		}
	})

	t.Run("active focused chat is skipped", func(t *testing.T) {
		m, got := newTestModel()
		m.currentChat = "c1"
		m.focused = true
		base := []graph.Chat{chatWithPreview("c1", "other", "hi", t0)}
		m.notifyNewChatMessages(base, true)

		next := []graph.Chat{chatWithPreview("c1", "other", "new", t0.Add(time.Minute))}
		m.notifyNewChatMessages(next, false)
		if len(*got) != 0 {
			t.Fatalf("active chat notified %d times, want 0", len(*got))
		}
	})

	t.Run("current chat while unfocused still notifies", func(t *testing.T) {
		m, got := newTestModel()
		m.currentChat = "c1"
		m.focused = false
		base := []graph.Chat{chatWithPreview("c1", "other", "hi", t0)}
		m.notifyNewChatMessages(base, true)

		next := []graph.Chat{chatWithPreview("c1", "other", "new", t0.Add(time.Minute))}
		m.notifyNewChatMessages(next, false)
		if len(*got) != 1 {
			t.Fatalf("unfocused current chat notified %d times, want 1", len(*got))
		}
	})

	t.Run("clicking the notification sends an open-chat message", func(t *testing.T) {
		m, got := newTestModel()
		var sent []tea.Msg
		m.sender.send = func(msg tea.Msg) { sent = append(sent, msg) }

		base := []graph.Chat{chatWithPreview("c9", "other", "hi", t0)}
		m.notifyNewChatMessages(base, true)
		next := []graph.Chat{chatWithPreview("c9", "other", "ping", t0.Add(time.Minute))}
		m.notifyNewChatMessages(next, false)

		if len(*got) != 1 || (*got)[0].Action == nil {
			t.Fatalf("expected 1 actionable notification, got %d", len(*got))
		}
		// Simulate a click with a stub activation token; focus command is "-"
		// (disabled) and we're not in tmux, so only the Send should fire.
		(*got)[0].Action("token")

		if len(sent) != 1 {
			t.Fatalf("click sent %d messages, want 1", len(sent))
		}
		open, ok := sent[0].(openChatByIDMsg)
		if !ok {
			t.Fatalf("sent message type = %T, want openChatByIDMsg", sent[0])
		}
		if open.chatID != "c9" {
			t.Errorf("openChatByIDMsg.chatID = %q, want %q", open.chatID, "c9")
		}
	})
}
