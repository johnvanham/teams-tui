package ui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jvh/teams-tui/internal/graph"
	"github.com/jvh/teams-tui/internal/ui/styles"
)

// The symbol legend shown beneath the expanded help (ctrl+g). The keybinding
// help explains what to press; this explains what the sidebar's markers mean —
// the chat-type prefixes, the unread colour, and the presence dots — none of
// which are self-describing.
//
// Entries are built from the same helpers the sidebar renders with
// (chatTypeGlyph, graph.Presence.Glyph, presenceColor) rather than repeating
// their literals, so the legend can't quietly go stale.

// legendGap separates entries on a legend row.
const legendGap = "  "

// legendRows renders the symbol legend, one string per screen row. It returns
// nil while the help is collapsed, so the legend costs nothing until asked for.
func (m Model) legendRows() []string {
	if !m.help.ShowAll {
		return nil
	}

	glyph := func(s string, fg color.Color, desc string) string {
		return lipgloss.NewStyle().Foreground(fg).Render(s) + styles.Hint.Render(" "+desc)
	}
	chatType := func(t graph.ChatType, desc string) string {
		return glyph(chatTypeGlyph(t), styles.White, desc)
	}
	presence := func(availability, desc string) string {
		p := graph.Presence{Availability: availability}
		return glyph(p.Glyph(), presenceColor(availability), desc)
	}

	groups := []struct {
		label   string
		entries []string
	}{
		{"Chats", []string{
			chatType(graph.ChatOneOnOne, "direct"),
			chatType(graph.ChatGroup, "group"),
			chatType(graph.ChatMeeting, "meeting"),
			// No marker for unread: the row itself turns orange and bold, so
			// the sample is the explanation.
			styles.UnreadTitle.Padding(0).Render("name") + styles.Hint.Render(" unread"),
		}},
		{"People", []string{
			presence("Available", "available"),
			presence("Busy", "busy or in a call"),
			presence("DoNotDisturb", "do not disturb"),
			presence("Away", "away"),
			presence("Offline", "offline or unknown"),
		}},
	}

	labelWidth := 0
	for _, g := range groups {
		if len(g.label) > labelWidth {
			labelWidth = len(g.label)
		}
	}
	var rows []string
	for _, g := range groups {
		rows = append(rows, m.legendGroupRows(g.label, labelWidth, g.entries)...)
	}
	return rows
}

// legendGroupRows lays one labelled group out across as many rows as the
// terminal is wide enough for, indenting continuation rows under the first so
// the entries stay in a column beside their label.
func (m Model) legendGroupRows(label string, labelWidth int, entries []string) []string {
	lead := styles.LegendLabel.Render(fmt.Sprintf("%-*s", labelWidth, label)) + legendGap
	indent := strings.Repeat(" ", labelWidth+len(legendGap))
	avail := m.width - labelWidth - len(legendGap)
	if avail < 1 {
		avail = 1
	}

	var rows []string
	var line string
	var lineWidth int
	flush := func() {
		if line == "" {
			return
		}
		prefix := indent
		if len(rows) == 0 {
			prefix = lead
		}
		rows = append(rows, prefix+line)
		line, lineWidth = "", 0
	}
	for _, e := range entries {
		w := ansi.StringWidth(e)
		if line != "" && lineWidth+len(legendGap)+w > avail {
			flush()
		}
		if line != "" {
			line += legendGap
			lineWidth += len(legendGap)
		}
		line += e
		lineWidth += w
	}
	flush()
	return rows
}

// legendHeight is the number of screen rows the legend occupies, including the
// blank row separating it from the help above. layout() subtracts it from the
// body's budget so expanding the help never clips the panes.
func (m Model) legendHeight() int {
	n := len(m.legendRows())
	if n == 0 {
		return 0
	}
	return n + 1 // blank separator row
}
