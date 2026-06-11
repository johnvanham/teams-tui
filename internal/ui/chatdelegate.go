package ui

import (
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/jvh/teams-tui/internal/ui/styles"
)

// chatDelegate renders chats using the standard list layout but highlights
// chats with unread messages by recolouring the leading marker glyph. Like
// contactDelegate, it lets the embedded DefaultDelegate paint the row, then
// post-processes the rendered line so we only restyle the marker and leave the
// default delegate's padding, truncation and selection styling intact.
type chatDelegate struct {
	list.DefaultDelegate
}

// newChatDelegate builds the chat delegate with the same geometry the chat list
// has always used (two-line items, one-line spacing).
func newChatDelegate() chatDelegate {
	d := list.NewDefaultDelegate()
	d.SetHeight(2)
	d.SetSpacing(1)
	return chatDelegate{DefaultDelegate: d}
}

// Render draws the item via the embedded default delegate, then highlights the
// unread marker. Non-chatItem or already-read items fall straight through.
func (d chatDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	ci, ok := item.(chatItem)
	if !ok || !ci.unread {
		d.DefaultDelegate.Render(w, m, index, item)
		return
	}

	// Render normally into a buffer so we keep the default delegate's padding,
	// truncation, selection bar and description styling.
	var buf strings.Builder
	d.DefaultDelegate.Render(&buf, m, index, item)
	out := buf.String()

	// Recolour the leading unread marker. The marker is the first visible
	// character of the title (Title() puts it first). Our recoloured marker
	// ends with a reset, which would strip the title's styling from the rest of
	// the line, so we re-assert whatever SGR sequence the default delegate had
	// opened just before the marker (reusing contactDelegate's ansiPrefixRe).
	colored := styles.UnreadDot.Render(unreadGlyph)
	if i := strings.Index(out, unreadGlyph); i >= 0 {
		titleStyle := ansiPrefixRe.FindString(out[:i])
		out = out[:i] + colored + titleStyle + out[i+len(unreadGlyph):]
	}
	_, _ = io.WriteString(w, out)
}

// Update satisfies list.ItemDelegate; defer to the embedded default.
func (d chatDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return d.DefaultDelegate.Update(msg, m)
}

var _ list.ItemDelegate = chatDelegate{}
