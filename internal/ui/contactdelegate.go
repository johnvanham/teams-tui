package ui

import (
	"image/color"
	"io"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jvh/teams-tui/internal/ui/styles"
)

// ansiPrefixRe matches the run of ANSI SGR escape sequences immediately
// preceding a position. We use it to recover the title's active style so it can
// be re-asserted after our independently-coloured glyph (whose trailing reset
// would otherwise strip the title colour from the rest of the line).
var ansiPrefixRe = regexp.MustCompile(`(\x1b\[[0-9;]*m)+$`)

// presenceColor maps a Teams availability to its conventional indicator colour
// (green = available, red = busy/DND/in-call, amber = away, grey = offline or
// unknown). It is the single source of truth shared by the conversation header
// and the contacts list so the glyph colours always agree.
func presenceColor(availability string) color.Color {
	switch availability {
	case "Available", "AvailableIdle":
		return styles.Green
	case "Busy", "BusyIdle", "InACall", "InAConferenceCall", "InAMeeting", "Presenting",
		"DoNotDisturb", "Focusing":
		return styles.Red
	case "Away", "BeRightBack", "OutOfOffice":
		return styles.Yellow
	default:
		return styles.LightGrey
	}
}

// contactDelegate renders contacts using the standard list layout but with the
// leading presence glyph coloured by availability. The bubbles DefaultDelegate
// paints the whole title in one style, so to colour just the glyph we let the
// default delegate render the row (with a glyph-less title) and then prepend a
// separately-coloured glyph onto the title line.
type contactDelegate struct {
	list.DefaultDelegate
}

// newContactDelegate builds the contacts delegate with the same geometry as the
// chat list (two-line items, one-line spacing).
func newContactDelegate() contactDelegate {
	d := list.NewDefaultDelegate()
	d.SetHeight(2)
	d.SetSpacing(1)
	return contactDelegate{DefaultDelegate: d}
}

// Render draws the item via the embedded default delegate, then recolours the
// leading presence glyph. Non-personItem items fall straight through.
func (d contactDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	pi, ok := item.(personItem)
	if !ok {
		d.DefaultDelegate.Render(w, m, index, item)
		return
	}

	// Render normally into a buffer so we keep the default delegate's padding,
	// truncation, selection bar and description styling.
	var buf strings.Builder
	d.DefaultDelegate.Render(&buf, m, index, item)
	out := buf.String()

	// The glyph is the first visible character of the title (Title() puts it
	// first). Re-render just that glyph in its presence colour and splice it
	// back in. Our coloured glyph ends with a reset, which would strip the
	// title's styling from the rest of the line, so we re-assert whatever SGR
	// sequence the default delegate had opened just before the glyph.
	glyph := pi.presence.Glyph()
	colored := lipgloss.NewStyle().Foreground(presenceColor(pi.presence.Availability)).Render(glyph)
	if i := strings.Index(out, glyph); i >= 0 {
		titleStyle := ansiPrefixRe.FindString(out[:i]) // style active at the glyph
		out = out[:i] + colored + titleStyle + out[i+len(glyph):]
	}
	_, _ = io.WriteString(w, out)
}

// Update satisfies list.ItemDelegate; defer to the embedded default.
func (d contactDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return d.DefaultDelegate.Update(msg, m)
}

var _ list.ItemDelegate = contactDelegate{}
