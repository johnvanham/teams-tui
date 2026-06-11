package ui

import (
	"io"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/jvh/teams-tui/internal/ui/styles"
)

// chatDelegate renders chats using the standard list layout but highlights
// chats with unread messages with a warm row background. It keeps two embedded
// default delegates: the normal one, and an "unread" variant whose normal
// title/description styles carry the unread background. Unread, non-selected
// rows are drawn with the unread variant; everything else (read rows, and the
// selected row whose own highlight should win) uses the normal delegate. This
// avoids fragile ANSI post-processing — the background fills cleanly because the
// styles are given the list width to render against.
type chatDelegate struct {
	list.DefaultDelegate
	unread list.DefaultDelegate
}

// newChatDelegate builds the chat delegate with the same geometry the chat list
// has always used (two-line items, one-line spacing).
func newChatDelegate() chatDelegate {
	base := list.NewDefaultDelegate()
	base.SetHeight(2)
	base.SetSpacing(1)

	// The unread variant differs only in its normal (non-selected) title and
	// description styles, which gain the warm background. Selected/dimmed styles
	// are left untouched so the selected row keeps its usual treatment.
	unread := list.NewDefaultDelegate()
	unread.SetHeight(2)
	unread.SetSpacing(1)
	unread.Styles.NormalTitle = styles.UnreadTitle
	unread.Styles.NormalDesc = styles.UnreadDesc

	return chatDelegate{DefaultDelegate: base, unread: unread}
}

// Render draws read rows (and the selected row) with the normal delegate, and
// unread non-selected rows with the background-highlighted variant.
func (d chatDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	ci, ok := item.(chatItem)
	isSelected := index == m.Index()
	if !ok || !ci.unread || isSelected {
		d.DefaultDelegate.Render(w, m, index, item)
		return
	}

	// Give the unread styles the full list width so the background fills the
	// whole row rather than just hugging the text. The default delegate sizes
	// text to width - paddingLeft - paddingRight, and our styles keep the same
	// left padding (2) as the normal styles, so the layout matches read rows.
	u := d.unread
	width := m.Width()
	u.Styles.NormalTitle = u.Styles.NormalTitle.Width(width)
	u.Styles.NormalDesc = u.Styles.NormalDesc.Width(width)
	u.Render(w, m, index, item)
}

// Update satisfies list.ItemDelegate; defer to the embedded default.
func (d chatDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return d.DefaultDelegate.Update(msg, m)
}

var _ list.ItemDelegate = chatDelegate{}
