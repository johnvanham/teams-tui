package ui

import (
	"testing"

	"charm.land/bubbles/v2/textarea"

	"github.com/jvh/teams-tui/internal/graph"
)

// newMentionModel returns a Model wired with a group chat and three members,
// the compose box set to value with the cursor at column col.
func newMentionModel(value string, col int) Model {
	ta := textarea.New()
	ta.SetWidth(60)
	ta.SetValue(value)
	ta.MoveToBegin()
	ta.SetCursorColumn(col)
	chat := graph.Chat{
		ID:       "c1",
		ChatType: graph.ChatGroup,
		Members: []graph.ConversationMember{
			{DisplayName: "Ada Lovelace", UserID: "u-ada"},
			{DisplayName: "Bob Smith", UserID: "u-bob"},
			{DisplayName: "Alan Turing", UserID: "u-alan"},
		},
	}
	return Model{
		compose:     ta,
		currentChat: "c1",
		chats:       map[string]graph.Chat{"c1": chat},
	}
}

func TestActiveMentionToken(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		col    int
		wantQ  string
		wantAt int
		wantOK bool
	}{
		{"simple", "hi @Ad", 6, "Ad", 3, true},
		{"at start", "@bo", 3, "bo", 0, true},
		{"empty query right after at", "@", 1, "", 0, true},
		{"no at", "hello", 5, "", 0, false},
		{"email is not a mention", "a@b", 3, "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMentionModel(tt.value, tt.col)
			q, at, ok := m.activeMentionToken()
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (q=%q)", ok, tt.wantOK, q)
			}
			if ok && (q != tt.wantQ || at != tt.wantAt) {
				t.Errorf("q=%q at=%d, want q=%q at=%d", q, at, tt.wantQ, tt.wantAt)
			}
		})
	}
}

func TestMatchMembers(t *testing.T) {
	m := newMentionModel("@a", 2)

	t.Run("word prefix matches surname", func(t *testing.T) {
		got := m.matchMembers("lov", 8)
		if len(got) != 1 || got[0].UserID != "u-ada" {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("empty query lists all members", func(t *testing.T) {
		if got := m.matchMembers("", 8); len(got) != 3 {
			t.Errorf("got %d, want 3", len(got))
		}
	})
	t.Run("prefix matches multiple", func(t *testing.T) {
		// "a" prefixes "Ada" and "Alan".
		got := m.matchMembers("a", 8)
		if len(got) != 2 {
			t.Errorf("got %d, want 2: %+v", len(got), got)
		}
	})
	t.Run("one-on-one chat yields nothing", func(t *testing.T) {
		c := m.chats["c1"]
		c.ChatType = graph.ChatOneOnOne
		m.chats["c1"] = c
		if got := m.matchMembers("a", 8); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
}

func TestApplyMentionSelection(t *testing.T) {
	m := newMentionModel("hi @lov", 7)
	m.refreshMentionPicker()
	if !m.mentionPicker {
		t.Fatal("picker should be open after typing @lov")
	}
	if len(m.mentionMatches) != 1 || m.mentionMatches[0].UserID != "u-ada" {
		t.Fatalf("unexpected matches: %+v", m.mentionMatches)
	}
	if !m.applyMentionSelection() {
		t.Fatal("applyMentionSelection returned false")
	}
	if got, want := m.compose.Value(), "hi @Ada Lovelace "; got != want {
		t.Errorf("compose = %q, want %q", got, want)
	}
	if len(m.mentions) != 1 || m.mentions[0].UserID != "u-ada" {
		t.Errorf("mentions = %+v", m.mentions)
	}
	if m.mentionPicker {
		t.Error("picker should close after completion")
	}
}

func TestRecordMentionDedup(t *testing.T) {
	m := newMentionModel("", 0)
	mem := graph.ConversationMember{DisplayName: "Ada Lovelace", UserID: "u-ada"}
	m.recordMention(mem)
	m.recordMention(mem)
	if len(m.mentions) != 1 {
		t.Errorf("mentions = %d, want 1 (deduped)", len(m.mentions))
	}
}
