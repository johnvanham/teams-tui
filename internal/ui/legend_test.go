package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/jvh/teams-tui/internal/graph"
)

func legendModel(width int) Model {
	m := Model{width: width}
	m.help.ShowAll = true
	return m
}

func TestLegendHiddenUntilHelpExpands(t *testing.T) {
	m := Model{width: 100}
	if rows := m.legendRows(); rows != nil {
		t.Errorf("legend rendered with the help collapsed: %v", rows)
	}
	if got := m.legendHeight(); got != 0 {
		t.Errorf("legendHeight() = %d with the help collapsed, want 0", got)
	}
}

// TestLegendCoversSidebarSymbols checks every marker the sidebar can show is
// explained, so a new one can't be added without the legend noticing.
func TestLegendCoversSidebarSymbols(t *testing.T) {
	m := legendModel(120)
	text := ansi.Strip(strings.Join(m.legendRows(), "\n"))

	for _, ct := range []graph.ChatType{graph.ChatOneOnOne, graph.ChatGroup, graph.ChatMeeting} {
		if !strings.Contains(text, chatTypeGlyph(ct)) {
			t.Errorf("legend is missing the %q chat marker %q", ct, chatTypeGlyph(ct))
		}
	}
	for _, availability := range []string{"Available", "Busy", "DoNotDisturb", "Away", "Offline"} {
		glyph := graph.Presence{Availability: availability}.Glyph()
		if !strings.Contains(text, glyph) {
			t.Errorf("legend is missing the %s presence glyph %q", availability, glyph)
		}
	}
	if !strings.Contains(text, "unread") {
		t.Error("legend doesn't explain the unread colour")
	}
}

// TestLegendFitsWidth checks the legend wraps rather than overflowing, at
// widths from comfortable down to cramped.
func TestLegendFitsWidth(t *testing.T) {
	for _, width := range []int{120, 100, 80, 60, 40, 30} {
		m := legendModel(width)
		rows := m.legendRows()
		if len(rows) == 0 {
			t.Fatalf("width %d: no legend rows", width)
		}
		for i, row := range rows {
			if w := ansi.StringWidth(row); w > width {
				t.Errorf("width %d: row %d is %d cells wide: %q",
					width, i, w, ansi.Strip(row))
			}
		}
		if got, want := m.legendHeight(), len(rows)+1; got != want {
			t.Errorf("width %d: legendHeight() = %d, want %d", width, got, want)
		}
	}
}

// TestChatTypeGlyphMatchesRow guards the legend's one assumption: that a chat
// row really does lead with chatTypeGlyph.
func TestChatTypeGlyphMatchesRow(t *testing.T) {
	for _, ct := range []graph.ChatType{graph.ChatOneOnOne, graph.ChatGroup, graph.ChatMeeting} {
		item := newChatItem(graph.Chat{ChatType: ct, Topic: "topic"}, "me", false)
		if want := chatTypeGlyph(ct) + " "; !strings.HasPrefix(item.Title(), want) {
			t.Errorf("%q row title = %q, want prefix %q", ct, item.Title(), want)
		}
	}
}
