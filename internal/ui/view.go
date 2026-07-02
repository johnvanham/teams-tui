package ui

import (
	"fmt"
	"image/color"
	"regexp"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jvh/teams-tui/internal/graph"
	"github.com/jvh/teams-tui/internal/ui/styles"
)

const (
	sidebarWidth           = 32
	titleHeight            = 1
	statusHeight           = 1
	sidebarHeaderRows      = 1 // "Chats" header drawn inside the sidebar chrome
	participantsHeaderRows = 1 // presence header above the conversation
	composeMinLines        = 1 // minimum visible rows in the compose box
	scrollbarWidth         = 2 // scroll-indicator gutter: one spacer + one bar column
)

// tallestHelpGroup returns the number of rows in the longest full-help group.
// The bubbles help component renders each group as a vertical column laid side
// by side, so the rendered block's height is that of the tallest column — which
// layout() must reserve so lower bindings (e.g. ctrl+f) aren't clipped.
func tallestHelpGroup(groups [][]key.Binding) int {
	max := 0
	for _, g := range groups {
		if len(g) > max {
			max = len(g)
		}
	}
	return max
}

// layout recomputes component sizes from the current terminal dimensions. It
// is called on resize and whenever the help expansion changes the chrome size.
func (m *Model) layout() {
	if !m.ready {
		return
	}
	// Remember whether the messages viewport was pinned to the bottom before we
	// resize it. Growing the compose box shrinks the viewport, which would
	// otherwise leave the newest message scrolled off-screen. We re-anchor to
	// the bottom at the end only when the user hadn't scrolled up themselves.
	stickToBottom := m.viewport.AtBottom()

	helpHeight := 1
	if m.help.ShowAll {
		// The full help lays each key group out as a column side by side, so its
		// height is the tallest group (plus a blank separator row above it).
		helpHeight = tallestHelpGroup(m.keys.FullHelp()) + 1
	}

	bannerHeight := 0
	if m.activeBanner() != "" {
		bannerHeight = 1
	}

	// Vertical budget: title + (banner) + body + status + help.
	bodyHeight := m.height - titleHeight - statusHeight - helpHeight - bannerHeight
	if bodyHeight < 3 {
		bodyHeight = 3
	}

	// Sidebar (list) occupies the left column; account for its border (2),
	// padding (2), and the header row inside it.
	listInnerW := sidebarWidth - 4
	listInnerH := bodyHeight - 2 - sidebarHeaderRows
	if listInnerW < 1 {
		listInnerW = 1
	}
	if listInnerH < 1 {
		listInnerH = 1
	}
	m.list.SetSize(listInnerW, listInnerH)
	m.contacts.SetSize(listInnerW, listInnerH)

	// Right column: messages viewport on top, compose box (grows) below.
	rightW := m.width - sidebarWidth
	rightInnerW := rightW - 4 // border + padding
	if rightInnerW < 1 {
		rightInnerW = 1
	}

	// The compose box grows with its content (the textarea manages this itself
	// via DynamicHeight, including wrapped lines and internal scrolling) up to
	// ~50% of the screen height.
	composeMax := m.height/2 - 2 // leave room for the box border
	if composeMax < composeMinLines {
		composeMax = composeMinLines
	}
	m.compose.MaxHeight = composeMax
	// Setting width re-runs the textarea's height recalculation against the new
	// wrap width, so it reports an up-to-date Height() below.
	m.compose.SetWidth(rightInnerW)
	composeInnerH := m.compose.Height()
	if composeInnerH < composeMinLines {
		composeInnerH = composeMinLines
	}
	if composeInnerH > composeMax {
		composeInnerH = composeMax
	}
	composeBoxH := composeInnerH + 2 // border
	// Reserve one row for the participants/presence header above the messages,
	// plus the inline emoji popup's rows when it is open (it sits between the
	// messages and the compose box, stealing height from the viewport), plus
	// the spell-check strip below the compose box when it has misspellings.
	vpInnerH := bodyHeight - composeBoxH - 2 - participantsHeaderRows - m.emojiPickerHeight() - m.reactPickerHeight() - m.emojiBrowserHeight() - m.mentionPickerHeight() - m.spellPickerHeight() - m.spellStripHeight() - m.replyBannerHeight()
	if vpInnerH < 1 {
		vpInnerH = 1
	}

	// Reserve the rightmost inner column for the scroll indicator gutter, so
	// the wrapped message text never collides with the bar.
	vpContentW := rightInnerW - scrollbarWidth
	if vpContentW < 1 {
		vpContentW = 1
	}
	m.viewport.SetWidth(vpContentW)
	m.viewport.SetHeight(vpInnerH)
	m.help.SetWidth(m.width)

	// Re-render the conversation to fit the new width.
	m.renderConversation()

	// Keep the latest message visible after the viewport's height changed
	// (e.g. the compose box grew while typing), unless the user had scrolled up.
	if stickToBottom {
		m.viewport.GotoBottom()
	}
}

// View renders the whole application as a tea.View (v2 returns a struct).
func (m Model) View() tea.View {
	var content string
	switch m.phase {
	case phaseAuthStarting:
		content = m.viewAuthStarting()
	case phaseAuthWaiting:
		content = m.viewAuthWaiting()
	case phaseLoading:
		content = m.viewLoading()
	case phaseError:
		content = m.viewError()
	case phaseReady:
		content = m.viewReady()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	// Report focus changes so polling can back off when the terminal is
	// unfocused (handled via tea.FocusMsg / tea.BlurMsg).
	v.ReportFocus = true
	// Enable mouse only in the main UI. During sign-in we leave the mouse to
	// the terminal so the user can select/copy the device code.
	if m.phase == phaseReady {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

func (m Model) centered(s string) string {
	if m.width == 0 || m.height == 0 {
		return s
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, s)
}

func (m Model) viewAuthStarting() string {
	body := fmt.Sprintf("%s Contacting Microsoft Entra…", m.spinner.View())
	return m.centered(body)
}

func (m Model) viewAuthWaiting() string {
	if m.deviceCode == nil {
		return m.centered(m.spinner.View() + " Preparing sign-in…")
	}
	dc := m.deviceCode
	title := styles.TitleBar.Render(" Microsoft Teams — Sign in ")
	code := styles.DeviceCode.Render(dc.UserCode)
	url := styles.DeviceURL.Render(dc.VerificationURI)

	// Each element is centered within the column so the padded code box lines
	// up cleanly instead of being shifted by leading spaces.
	block := lipgloss.JoinVertical(lipgloss.Center,
		title,
		"",
		styles.Hint.Render("1. Open this URL in a browser:"),
		url,
		"",
		styles.Hint.Render("2. Enter this code:"),
		code,
		"",
		styles.Hint.Render(m.spinner.View()+" Waiting for you to finish signing in…"),
		"",
		styles.Hint.Render("This works with both Entra-hosted and hybrid/federated"),
		styles.Hint.Render("setups — your company login page will appear if required."),
		"",
		styles.Hint.Render("Press ctrl+c to cancel."),
	)
	return m.centered(block)
}

func (m Model) viewLoading() string {
	return m.centered(m.spinner.View() + " Loading your chats…")
}

func (m Model) viewError() string {
	box := styles.ErrorBanner.Render("Error") + "\n\n" +
		m.errText + "\n\n" +
		styles.Hint.Render("Press ctrl+c to quit.")
	return m.centered(box)
}

func (m Model) viewReady() string {
	if !m.ready {
		return m.spinner.View() + " starting…"
	}

	// Status picker overlay takes over the screen while open.
	if m.showStatus {
		return m.viewStatusPicker()
	}

	// Title bar.
	who := "Teams"
	if m.me != nil && m.me.DisplayName != "" {
		who = "Teams — " + m.me.DisplayName
	}
	title := styles.TitleBar.Width(m.width).Render(who)

	// Sidebar with its own header above the list. Pin the width so the column
	// geometry is predictable (also matters for mouse click targeting). The
	// header label and list content switch with the sidebar mode.
	sideStyle := styles.SidebarBlurred
	if m.focus == focusChats {
		sideStyle = styles.SidebarFocused
	}
	headerLabel := "Chats"
	listView := m.list.View()
	if m.sidebarMode == sidebarContacts {
		headerLabel = "Contacts"
		listView = m.contacts.View()
	}
	header := styles.SidebarHeader.Width(sidebarWidth - 4).Render(headerLabel)
	sidebarInner := lipgloss.JoinVertical(lipgloss.Left, header, listView)
	sidebar := sideStyle.Width(sidebarWidth).Render(sidebarInner)

	// Participants header (names + presence) above the conversation.
	participants := m.participantsHeader()

	// The right column boxes are pinned to the remaining width so they align
	// flush with the participants bar and fill the screen.
	rightW := m.width - sidebarWidth
	if rightW < 3 {
		rightW = 3
	}
	boxW := rightW // lipgloss .Width is the total rendered width incl. border

	// Messages pane. The viewport content sits left of a one-column scroll
	// indicator gutter, so it's clear when we're not at the latest message.
	msgStyle := styles.MessagePaneBlurred
	if m.focus == focusMessages {
		msgStyle = styles.MessagePaneFocused
	}
	msgInner := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.viewport.View(),
		m.scrollbar(m.viewport.Height()),
	)
	messages := msgStyle.Width(boxW).Render(msgInner)

	// Compose box.
	composeStyle := styles.ComposeBlurred
	if m.focus == focusCompose {
		composeStyle = styles.ComposeFocused
	}
	compose := composeStyle.Width(boxW).Render(m.compose.View())

	// The inline emoji popup sits directly above the compose box so it never
	// covers the text being typed.
	rightParts := []string{participants, messages}
	if m.emojiPicker {
		rightParts = append(rightParts, m.viewEmojiPicker())
	}
	if m.reactPicker {
		rightParts = append(rightParts, m.viewReactPicker())
	}
	if m.emojiBrowser {
		rightParts = append(rightParts, m.viewEmojiBrowser())
	}
	if m.mentionPicker {
		rightParts = append(rightParts, m.viewMentionPicker())
	}
	if m.spellPicker {
		rightParts = append(rightParts, m.viewSpellPicker())
	}
	// Reply-preview banner sits directly above the compose box while replying,
	// showing the sender and quoted snippet (or highlighted selection).
	if m.replyBannerHeight() > 0 {
		rightParts = append(rightParts, m.viewReplyBanner(boxW))
	}
	rightParts = append(rightParts, compose)
	// Spell-check strip sits directly beneath the compose box, listing
	// misspelled words + top suggestions (only when there are any).
	if m.spellStripHeight() > 0 {
		rightParts = append(rightParts, m.viewSpellStrip(boxW))
	}
	right := lipgloss.JoinVertical(lipgloss.Left, rightParts...)
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, right)

	parts := []string{title}
	if b := m.activeBanner(); b != "" {
		parts = append(parts, styles.Banner.Width(m.width).Render(b))
	}
	parts = append(parts, body)

	// Status + help.
	status := m.statusLine()
	parts = append(parts, status)
	parts = append(parts, m.help.View(m.keys))

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// participantsHeader renders a single line listing the other participants in
// the current chat, each prefixed by a colored presence glyph and (for 1:1
// chats) followed by their status label. The line is width-clamped to the
// right column and truncated with an ellipsis if needed.
func (m Model) participantsHeader() string {
	rightW := m.width - sidebarWidth
	if rightW < 1 {
		rightW = 1
	}
	bar := styles.ParticipantsBar
	chat, ok := m.chats[m.currentChat]
	if !ok || m.currentChat == "" {
		return bar.Width(rightW).Render("")
	}

	// All inner spans carry the bar's background so the row is one uniform
	// colour. Generous spacing keeps it readable: a wide gap separates the
	// title from the participants, and participants are separated by spaces.
	bg := styles.ParticipantsBarBg
	on := func(fg color.Color, s string) string {
		return lipgloss.NewStyle().Background(bg).Foreground(fg).Render(s)
	}

	self := m.selfID()
	oneOnOne := chat.ChatType == graph.ChatOneOnOne
	// A real topic gives the chat a distinct title; otherwise DisplayName just
	// echoes the member names, so we'd be duplicating the participant list.
	hasTitle := chat.Topic != ""

	// Participants with presence glyphs, each separated by clear spacing.
	var parts []string
	for _, mem := range chat.Members {
		if mem.UserID != "" && mem.UserID == self {
			continue
		}
		name := mem.DisplayName
		if name == "" {
			continue
		}
		seg := m.presenceGlyph(mem.UserID, bg) + on(styles.LightGrey, " "+name)
		// Show the textual status next to the name for 1:1 chats (and any chat
		// with a title, where there's room and no name duplication).
		if oneOnOne || hasTitle {
			if p, ok := m.presences[mem.UserID]; ok {
				if label := p.Label(); label != "" {
					seg += on(styles.Grey, " — "+label)
				}
			}
		}
		parts = append(parts, seg)
	}
	participants := strings.Join(parts, on(styles.Grey, "    "))

	// Build the line. With a real topic: bold title, wide gap, participants.
	// Without one: just the participants (the title would only repeat names).
	var body string
	if hasTitle {
		title := lipgloss.NewStyle().Background(bg).Foreground(styles.White).Bold(true).Render(chat.Topic)
		body = title
		if participants != "" {
			body += on(styles.Grey, "     ") + participants
		}
	} else {
		body = participants
	}

	// Clamp to the available inner width (minus the bar's horizontal padding,
	// which is 2 cells each side). ansi.Truncate is escape-aware so it never
	// cuts mid-sequence (which was causing stray characters/background gaps).
	avail := rightW - 4
	if lipgloss.Width(body) > avail {
		body = ansi.Truncate(body, avail, "…")
	}
	return bar.Width(rightW).Render(body)
}

// presenceGlyph returns the colored presence indicator for a user, rendered on
// the given background so it blends into the surrounding bar.
func (m Model) presenceGlyph(userID string, bg color.Color) string {
	p, ok := m.presences[userID]
	glyph := "○"
	fg := styles.LightGrey
	if ok {
		glyph = p.Glyph()
		fg = presenceColor(p.Availability)
	}
	return lipgloss.NewStyle().Background(bg).Foreground(fg).Render(glyph)
}

// humanBytes formats a byte count as a short human-readable string (e.g.
// "12 KB", "1.4 MB") for the attached-image indicator.
func humanBytes(n int) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := int64(n) / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// emojiPickerHeight returns the number of terminal rows the inline emoji popup
// occupies when open (one row per match + a hint row + the box border), or 0
// when it is closed. layout() subtracts this from the messages viewport so the
// overall vertical budget stays balanced.
func (m Model) emojiPickerHeight() int {
	if !m.emojiPicker || len(m.emojiMatches) == 0 {
		return 0
	}
	return len(m.emojiMatches) + 1 + 2 // rows + hint + top/bottom border
}

// replyBannerHeight returns the rows the reply-preview banner occupies above the
// compose box (1 while replying to a message, else 0). layout() subtracts it
// from the messages viewport so the vertical budget stays balanced.
func (m Model) replyBannerHeight() int {
	if m.replyTo == nil {
		return 0
	}
	return 1
}

// viewReplyBanner renders a one-line preview of the message being replied to,
// shown directly above the compose box while a reply is in progress, e.g.:
//
//	▌ Reply to Ada: sounds good, let's do that   esc to cancel
//
// The quoted text is the reply preview — which is the mouse-highlighted snippet
// when the user selected one before pressing q, otherwise the whole message — so
// it reflects exactly what will be quoted. It's flattened to a single line and
// clamped to the available width with an ellipsis.
func (m Model) viewReplyBanner(width int) string {
	if m.replyTo == nil {
		return ""
	}
	bar := styles.ReplyBar.Render("▌ ")
	who := m.replyTo.SenderName
	if who == "" {
		who = "message"
	}
	label := styles.ReplyLabel.Render("Reply to " + who + ": ")
	hint := "   " + styles.Hint.Render("esc to cancel")

	// Collapse the (possibly multi-line) preview to one line so the banner
	// stays a single row regardless of how much was quoted.
	preview := strings.Join(strings.Fields(m.replyTo.Preview), " ")

	avail := width - lipgloss.Width(bar) - lipgloss.Width(label) - lipgloss.Width(hint)
	if avail < 1 {
		avail = 1
	}
	preview = ansi.Truncate(preview, avail, "…")
	return bar + label + styles.ReplyText.Render(preview) + hint
}

// spellStripHeight returns the rows the spell-check strip occupies (1 when
// there are misspellings to show, else 0). layout() subtracts it from the
// messages viewport so the vertical budget stays balanced.
func (m Model) spellStripHeight() int {
	if len(m.spellMisspell) == 0 {
		return 0
	}
	return 1
}

// viewSpellStrip renders a one-line summary of misspelled words and their top
// suggestion under the compose box, e.g.:
//
//	spelling: teh→the · recieve→receive · xyzzyq   ctrl+f to fix
//
// Words are clamped to the available width; a trailing "(+N)" indicates how
// many further misspellings didn't fit. When at least one word has a
// suggestion, a "ctrl+f to fix" hint is appended (space permitting) so the
// correction picker is discoverable.
func (m Model) viewSpellStrip(width int) string {
	if len(m.spellMisspell) == 0 {
		return ""
	}
	label := styles.SpellLabel.Render("spelling: ")
	sep := styles.SpellLabel.Render(" · ")

	// A hint pointing at the correction picker, shown only when something is
	// actually fixable (a misspelling with a suggestion).
	hint := ""
	for _, ms := range m.spellMisspell {
		if len(ms.Suggestions) > 0 {
			hint = "   " + styles.Hint.Render("ctrl+f to fix")
			break
		}
	}

	avail := width - lipgloss.Width(label)
	// Reserve room for a worst-case overflow suffix "(+N)" plus the hint so the
	// strip never exceeds the width once appended. N is the largest we could
	// report (all-but-the-first omitted).
	suffixReserve := lipgloss.Width(fmt.Sprintf(" (+%d)", len(m.spellMisspell))) + lipgloss.Width(hint)

	var parts []string
	used := 0
	omitted := 0
	for i, ms := range m.spellMisspell {
		entry := styles.SpellWord.Render(ms.Word)
		if len(ms.Suggestions) > 0 {
			entry += styles.SpellLabel.Render("→") + styles.SpellSuggestion.Render(ms.Suggestions[0])
		}
		w := lipgloss.Width(entry)
		if i > 0 {
			w += lipgloss.Width(sep)
		}
		// Always show at least the first entry; stop once the entry (plus the
		// reserved overflow suffix + hint, since stopping here means we'll
		// append them) would exceed the budget.
		if i > 0 && used+w+suffixReserve > avail {
			omitted = len(m.spellMisspell) - i
			break
		}
		if i > 0 {
			parts = append(parts, sep)
		}
		parts = append(parts, entry)
		used += w
	}
	if omitted > 0 {
		parts = append(parts, styles.SpellLabel.Render(fmt.Sprintf(" (+%d)", omitted)))
	}
	// Drop the hint if even the reserved space doesn't fit the current width.
	if used+lipgloss.Width(hint) > avail {
		hint = ""
	}
	return label + strings.Join(parts, "") + hint
}

// reactPickerHeight returns the rows the reaction picker occupies when open
// (one row per emoji + a query/hint row + the box border), or 0 when closed.
// layout() subtracts this from the messages viewport like the emoji picker.
func (m Model) reactPickerHeight() int {
	if !m.reactPicker {
		return 0
	}
	n := len(m.reactMatches)
	if n == 0 {
		n = 1 // still show the "no matches" row
	}
	return n + 1 + 2 // rows + query/hint + top/bottom border
}

// viewReactPicker renders the reaction emoji chooser as a small bordered list
// with a search line. Each row shows the glyph and its :shortcode:; the
// highlighted row is reverse-styled.
func (m Model) viewReactPicker() string {
	rows := make([]string, 0, len(m.reactMatches)+1)
	query := "React: " + m.reactQuery
	rows = append(rows, styles.Hint.Render(query+"▌"))
	if len(m.reactMatches) == 0 {
		rows = append(rows, styles.EmojiPickerItem.Render(" no matching emoji "))
	}
	for i, e := range m.reactMatches {
		label := e.Emoji + "  :" + e.Name + ":"
		if i == m.reactSel {
			rows = append(rows, styles.EmojiPickerSelected.Render(" "+label+" "))
		} else {
			rows = append(rows, styles.EmojiPickerItem.Render(" "+label+" "))
		}
	}
	hint := styles.Hint.Render("type to search · ↑↓ select · enter react · esc cancel")
	body := lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinVertical(lipgloss.Left, rows...), hint)
	return styles.EmojiPicker.Render(body)
}

// emojiBrowserHeight returns the rows the full emoji browser occupies when open
// (a query row + up to emojiBrowserMax emoji rows + a nav hint + the box
// border), or 0 when closed. layout() subtracts this from the messages viewport
// like the other pickers so the vertical budget stays balanced.
func (m Model) emojiBrowserHeight() int {
	if !m.emojiBrowser {
		return 0
	}
	rows, _ := m.browserWindow()
	n := len(rows)
	if n == 0 {
		n = 1 // still show the "no matches" row
	}
	extra := 0
	if len(m.browserMatches) > len(rows) {
		extra = 1 // the "+N more" indicator row
	}
	return n + extra + 2 + 2 // rows + indicator + query line + nav hint + top/bottom border
}

// viewEmojiBrowser renders the full emoji browser: a search line, a scrolling
// window of glyph + :shortcode: rows (the highlighted row reverse-styled, with a
// "+N more" tail when the list overflows the window), and a navigation hint.
func (m Model) viewEmojiBrowser() string {
	win, sel := m.browserWindow()
	total := len(m.browserMatches)

	rows := make([]string, 0, len(win)+2)
	query := "Emoji: " + m.browserQuery
	rows = append(rows, styles.Hint.Render(query+"▌"))
	if total == 0 {
		rows = append(rows, styles.EmojiPickerItem.Render(" no matching emoji "))
	}
	for i, e := range win {
		label := e.Emoji + "  :" + e.Name + ":"
		if i == sel {
			rows = append(rows, styles.EmojiPickerSelected.Render(" "+label+" "))
		} else {
			rows = append(rows, styles.EmojiPickerItem.Render(" "+label+" "))
		}
	}
	if total > len(win) {
		more := fmt.Sprintf(" %d of %d — keep typing to narrow ", m.browserSel+1, total)
		rows = append(rows, styles.Hint.Render(more))
	}
	hint := styles.Hint.Render("type to filter · ↑↓ select · enter insert · esc cancel")
	body := lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinVertical(lipgloss.Left, rows...), hint)
	return styles.EmojiPicker.Render(body)
}

// spellPickerHeight returns the rows the correction picker occupies when open
// (one row per candidate + a hint row + the box border), or 0 when closed.
// layout() subtracts this from the messages viewport like the other pickers.
func (m Model) spellPickerHeight() int {
	if !m.spellPicker || len(m.spellCandidates) == 0 {
		return 0
	}
	return len(m.spellCandidates) + 1 + 2 // rows + hint + top/bottom border
}

// viewSpellPicker renders the spelling correction chooser: one row per
// "word → suggestion" candidate (the highlighted row reverse-styled) plus a
// navigation hint.
func (m Model) viewSpellPicker() string {
	if len(m.spellCandidates) == 0 {
		return ""
	}
	rows := make([]string, 0, len(m.spellCandidates))
	for i, c := range m.spellCandidates {
		label := c.Word + " → " + c.Suggestion
		if i == m.spellPickerSel {
			rows = append(rows, styles.EmojiPickerSelected.Render(" "+label+" "))
		} else {
			rows = append(rows, styles.EmojiPickerItem.Render(" "+label+" "))
		}
	}
	list := lipgloss.JoinVertical(lipgloss.Left, rows...)
	hint := styles.Hint.Render("↑↓ select · enter fix · esc close")
	body := lipgloss.JoinVertical(lipgloss.Left, list, hint)
	return styles.EmojiPicker.Render(body)
}

// mentionPickerHeight returns the rows the @-mention popup occupies when open
// (one row per match + a hint row + the box border), or 0 when closed. layout()
// subtracts this from the messages viewport like the other pickers.
func (m Model) mentionPickerHeight() int {
	if !m.mentionPicker || len(m.mentionMatches) == 0 {
		return 0
	}
	return len(m.mentionMatches) + 1 + 2 // rows + hint + top/bottom border
}

// viewMentionPicker renders the @-mention autocomplete as a small bordered list
// of participant names, the highlighted row reverse-styled, with a hint line.
func (m Model) viewMentionPicker() string {
	if len(m.mentionMatches) == 0 {
		return ""
	}
	rows := make([]string, 0, len(m.mentionMatches))
	for i, mem := range m.mentionMatches {
		label := "@" + mem.DisplayName
		if i == m.mentionSel {
			rows = append(rows, styles.EmojiPickerSelected.Render(" "+label+" "))
		} else {
			rows = append(rows, styles.EmojiPickerItem.Render(" "+label+" "))
		}
	}
	list := lipgloss.JoinVertical(lipgloss.Left, rows...)
	hint := styles.Hint.Render("↑↓ select · tab/enter mention · esc close")
	body := lipgloss.JoinVertical(lipgloss.Left, list, hint)
	return styles.EmojiPicker.Render(body)
}

// viewEmojiPicker renders the inline emoji autocomplete suggestions as a small
// bordered list. Each row shows the glyph and its :shortcode:; the highlighted
// row is reverse-styled. A hint line documents the navigation/accept keys.
func (m Model) viewEmojiPicker() string {
	if len(m.emojiMatches) == 0 {
		return ""
	}
	rows := make([]string, 0, len(m.emojiMatches))
	for i, e := range m.emojiMatches {
		label := e.Emoji + "  :" + e.Name + ":"
		if i == m.emojiSel {
			rows = append(rows, styles.EmojiPickerSelected.Render(" "+label+" "))
		} else {
			rows = append(rows, styles.EmojiPickerItem.Render(" "+label+" "))
		}
	}
	list := lipgloss.JoinVertical(lipgloss.Left, rows...)
	hint := styles.Hint.Render("↑↓ select · tab/enter insert · esc close")
	body := lipgloss.JoinVertical(lipgloss.Left, list, hint)
	return styles.EmojiPicker.Render(body)
}

// viewStatusPicker renders the status-selection popup centered on screen.
func (m Model) viewStatusPicker() string {
	box := styles.PopupBox.Render(m.statusPicker.View())
	hint := styles.Hint.Render("enter to set · esc to cancel")
	body := lipgloss.JoinVertical(lipgloss.Center, box, "", hint)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, body)
}

func (m Model) statusLine() string {
	left := ""
	switch m.focus {
	case focusChats:
		left = "CHATS"
		if m.sidebarMode == sidebarContacts {
			left = "CONTACTS"
		}
	case focusMessages:
		left = "MESSAGES"
	case focusCompose:
		left = "COMPOSE"
		if m.inOpenCodeBlock() {
			left = "CODE BLOCK (enter for new line · close with ``` then enter to send)"
		}
	}
	if m.editingMsgID != "" {
		left = "EDITING (enter to save · esc to cancel)"
	}
	if len(m.pendingImage) > 0 {
		left = fmt.Sprintf("IMAGE ATTACHED %s (enter to send · esc to discard)",
			humanBytes(len(m.pendingImage)))
	}
	if m.errText != "" {
		return styles.ErrorBanner.Width(m.width).Render(left + " · " + m.errText)
	}

	leftText := fmt.Sprintf("%s · %d chats", left, len(m.chatOrder))

	// Right side: own presence ("● Available"), plus a hint to change it.
	right := m.myStatusText() + "  " + styles.Hint.Render("ctrl+s")

	// Lay out left and right within the full width.
	gap := m.width - lipgloss.Width(leftText) - lipgloss.Width(right) - 2 // padding
	if gap < 1 {
		// Not enough room: just show the left text.
		return styles.StatusBar.Width(m.width).Render(leftText)
	}
	content := leftText + strings.Repeat(" ", gap) + right
	return styles.StatusBar.Width(m.width).Render(content)
}

// myStatusText renders the signed-in user's own presence for the footer.
func (m Model) myStatusText() string {
	if m.myPresence == nil {
		return styles.Hint.Render("status unknown")
	}
	label := m.myPresence.Label()
	if label == "" {
		label = "Offline"
	}
	var c color.Color
	switch m.myPresence.Availability {
	case "Available", "AvailableIdle":
		c = styles.Green
	case "Busy", "BusyIdle", "InACall", "InAConferenceCall", "InAMeeting", "Presenting",
		"DoNotDisturb", "Focusing":
		c = styles.Red
	case "Away", "BeRightBack", "OutOfOffice":
		c = styles.Yellow
	default:
		c = styles.LightGrey
	}
	glyph := lipgloss.NewStyle().Foreground(c).Render(m.myPresence.Glyph())
	return glyph + " " + label
}

func (m Model) activeBanner() string {
	if m.banner != "" && time.Now().Before(m.bannerUntil) {
		return m.banner
	}
	return ""
}

// renderConversation formats the current chat's messages into the viewport.
func (m *Model) renderConversation() {
	// Reset click/keybinding image + selection state up front so every early
	// return below leaves no stale data from a previously open chat.
	m.convImages = m.convImages[:0]
	m.imageLines = nil
	m.convMsgs = nil
	m.msgLineStart = nil
	if m.currentChat == "" {
		m.viewport.SetContent(styles.Hint.Render("Select a chat to start messaging."))
		return
	}
	msgs := m.messages[m.currentChat]
	if len(msgs) == 0 {
		// Show a friendly notice if this chat failed to load (e.g. a meeting
		// chat we lack permission to read), otherwise the empty-chat prompt.
		if notice, ok := m.chatErrors[m.currentChat]; ok {
			m.viewport.SetContent(styles.Hint.Render(notice))
			return
		}
		m.viewport.SetContent(styles.Hint.Render("No messages yet. Say hello!"))
		return
	}

	// Graph returns newest-first; display oldest-first.
	ordered := make([]graph.Message, len(msgs))
	copy(ordered, msgs)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
	})

	// Keep only renderable messages (those with text or images), in display
	// order, so selection indexes line up with what's on screen. convMsgs is the
	// source of truth for the select/react/quote actions.
	visible := ordered[:0:0]
	for _, msg := range ordered {
		text := msg.PlainText()
		if msg.DeletedAt != nil {
			text = "(message deleted)"
		}
		if text == "" && len(msg.Images()) == 0 {
			continue
		}
		visible = append(visible, msg)
	}
	m.convMsgs = visible
	m.clampSelection()

	width := m.viewport.Width()
	var b strings.Builder
	// Collect every image in the conversation, in display order, so the image
	// keybinding can reference them by number ("open image [2]"). line tracks
	// the current 0-based content line so we can map a click on a placeholder
	// row back to its image (imageLines), and a message's header row back to its
	// index (msgLineStart).
	m.imageLines = make(map[int]int)
	m.msgLineStart = make([]int, len(m.convMsgs))
	// wrapCont[i] marks content line i as a soft word-wrap continuation of the
	// previous line, so selection text extraction can rejoin wrapped prose with
	// a space instead of a hard newline. Everything except reflowed prose (the
	// header, image/reaction rows, blank separators, code/quote blocks) is a
	// hard break (false).
	var wrapCont []bool
	line := 0
	for i, msg := range m.convMsgs {
		text := msg.PlainText()
		if msg.DeletedAt != nil {
			text = "(message deleted)"
		}
		images := msg.Images()
		name := msg.SenderName()
		nameStyle := styles.SenderName
		if m.me != nil && msg.From != nil && msg.From.User != nil && msg.From.User.ID == m.me.ID {
			nameStyle = styles.SenderSelf
		}
		ts := msg.CreatedAt.Local().Format("15:04")
		header := nameStyle.Render(name) + " " + styles.Timestamp.Render(ts)
		// Highlight the selected message's header while the messages pane is the
		// active pane, so it's clear which message react/quote will act on.
		if i == m.selectedMsg && m.focus == focusMessages {
			header = styles.MessageSelected.Render("› ") + header
		}
		m.msgLineStart[i] = line
		b.WriteString(header)
		b.WriteString("\n")
		wrapCont = append(wrapCont, false) // header is a hard break
		line++                             // header row
		if text != "" {
			bodyLines, bodyCont := renderBodyLines(text, width, m.codeStyle)
			b.WriteString(strings.Join(bodyLines, "\n"))
			b.WriteString("\n")
			wrapCont = append(wrapCont, bodyCont...)
			line += len(bodyLines)
		}
		// Render a numbered placeholder for each image; the number matches the
		// index used by the "view image" action (1-based for humans). Record
		// the content line so a click on the row can open that exact image.
		for _, img := range images {
			m.convImages = append(m.convImages, img)
			idx := len(m.convImages) - 1
			label := img.Name
			if label == "" {
				label = "image"
			}
			placeholder := fmt.Sprintf("🖼  [%d] %s — ctrl+y / click to view", idx+1, label)
			b.WriteString(styles.ImagePlaceholder.Render(placeholder))
			b.WriteString("\n")
			m.imageLines[line] = idx
			wrapCont = append(wrapCont, false)
			line++ // placeholder row
		}
		if reactions := msg.ReactionSummary(); len(reactions) > 0 {
			b.WriteString(styles.Reaction.Render(strings.Join(reactions, "  ")))
			b.WriteString("\n")
			wrapCont = append(wrapCont, false)
			line++ // reaction row
		}
		b.WriteString("\n")
		wrapCont = append(wrapCont, false)
		line++ // blank separator row
	}
	content := strings.TrimRight(b.String(), "\n")
	// Remember the rendered content split into lines so a mouse selection can be
	// highlighted and its text extracted (see selection.go). Kept in sync with
	// the viewport content set below. wrapCont is truncated to match, since
	// TrimRight may drop the trailing blank separator line(s).
	m.selContent = strings.Split(content, "\n")
	if len(wrapCont) > len(m.selContent) {
		wrapCont = wrapCont[:len(m.selContent)]
	}
	m.selWrapCont = wrapCont
	if m.selecting {
		content = m.applySelectionHighlight(content)
	}
	m.viewport.SetContent(content)
}

// clampSelection keeps selectedMsg within the bounds of the currently visible
// messages. When there is no valid selection yet (e.g. a chat just opened) it
// defaults to the newest (last) message so the messages pane has something
// highlighted as soon as it's focused.
func (m *Model) clampSelection() {
	n := len(m.convMsgs)
	if n == 0 {
		m.selectedMsg = -1
		return
	}
	if m.selectedMsg < 0 || m.selectedMsg >= n {
		m.selectedMsg = n - 1
	}
}

// selectedMessage returns the currently selected message, if any.
func (m Model) selectedMessage() (graph.Message, bool) {
	if m.selectedMsg < 0 || m.selectedMsg >= len(m.convMsgs) {
		return graph.Message{}, false
	}
	return m.convMsgs[m.selectedMsg], true
}

// scrollToSelection adjusts the viewport so the selected message's header is
// visible, nudging the offset only when the selection sits outside the current
// window. Keeps keyboard navigation from running the selection off-screen.
func (m *Model) scrollToSelection() {
	if m.selectedMsg < 0 || m.selectedMsg >= len(m.msgLineStart) {
		return
	}
	top := m.msgLineStart[m.selectedMsg]
	height := m.viewport.Height()
	off := m.viewport.YOffset()
	if top < off {
		m.viewport.SetYOffset(top)
	} else if top >= off+height {
		m.viewport.SetYOffset(top - height + 1)
	}
}

// scrollbar renders a one-column vertical scrollbar of the given height for the
// messages viewport: a dim track with a brighter thumb whose size and position
// reflect how much of the conversation is visible and where the window sits.
// It returns an empty string (no bar) when everything already fits on screen,
// so the gutter only appears once the conversation overflows.
func (m *Model) scrollbar(height int) string {
	if height < 1 {
		return ""
	}
	total := m.viewport.TotalLineCount()
	// Nothing to scroll: the whole conversation fits, so draw no bar (the
	// caller leaves the reserved columns blank).
	if total <= height {
		return strings.Repeat("\n", height-1)
	}

	// Thumb size is proportional to the visible fraction, at least one cell.
	thumb := height * height / total
	if thumb < 1 {
		thumb = 1
	}
	if thumb > height {
		thumb = height
	}

	// Position the thumb by scroll progress. AtBottom pins it flush to the end
	// so "we're at the latest" is unambiguous even with rounding.
	space := height - thumb
	var pos int
	if m.viewport.AtBottom() {
		pos = space
	} else {
		pos = int(m.viewport.ScrollPercent() * float64(space))
		if pos < 0 {
			pos = 0
		}
		if pos > space {
			pos = space
		}
	}

	var b strings.Builder
	for i := 0; i < height; i++ {
		// Leading space keeps the bar off the message text.
		b.WriteByte(' ')
		if i >= pos && i < pos+thumb {
			b.WriteString(styles.ScrollbarThumb.Render("█"))
		} else {
			b.WriteString(styles.ScrollbarTrack.Render("│"))
		}
		if i < height-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// codeFence is the marker line graph.MessageBody.PlainText emits around code
// blocks (a Markdown triple backtick, with an optional language on the opening
// fence). renderBody keys off it to style code distinctly from prose.
const codeFence = "```"

// renderBody renders a converted message body, styling fenced code blocks and
// inline `backtick` snippets distinctly from prose. Prose lines are
// word-wrapped to width; code blocks are shown verbatim (no reflow) on a dim
// panel, clamped to width. The result is a single string ready for the
// viewport.
func renderBody(text string, width int, codeStyle *chroma.Style) string {
	rendered, _ := renderBodyLines(text, width, codeStyle)
	return strings.Join(rendered, "\n")
}

// renderBodyLines is renderBody's underlying implementation: it returns the
// rendered body split into content lines plus a parallel "continuation" flag
// per line. A true flag marks a line that is a soft word-wrap continuation of
// the previous content line (not a real newline in the source), so text
// extraction (selection copy/quote) can rejoin wrapped prose with a space
// instead of a hard newline. Code-block and quote lines are always treated as
// hard breaks since they aren't reflowable prose.
func renderBodyLines(text string, width int, codeStyle *chroma.Style) (lines []string, cont []bool) {
	src := strings.Split(text, "\n")
	// add appends a rendered block (possibly multi-line) whose first line is a
	// hard break and whose remaining lines are marked continuations only when
	// soft is true (prose wrapping); code/quote blocks pass soft=false.
	add := func(block string, soft bool) {
		blockLines := strings.Split(block, "\n")
		for j, bl := range blockLines {
			lines = append(lines, bl)
			cont = append(cont, soft && j > 0)
		}
	}
	for i := 0; i < len(src); i++ {
		line := src[i]
		// An opening fence (```​ or ```lang) starts a code block that runs until
		// the matching closing fence (or end of text if Teams sent a malformed
		// block).
		if strings.HasPrefix(line, codeFence) {
			lang := strings.TrimSpace(strings.TrimPrefix(line, codeFence))
			var code []string
			i++
			for ; i < len(src); i++ {
				if strings.HasPrefix(src[i], codeFence) {
					break
				}
				code = append(code, src[i])
			}
			add(renderCodeBlock(code, lang, width, codeStyle), false)
			continue
		}
		// A run of ">"-prefixed lines is a quoted reply: collect the whole run
		// and render it as one styled block with a left bar.
		if isQuoteLine(line) {
			var quote []string
			for ; i < len(src) && isQuoteLine(src[i]); i++ {
				quote = append(quote, stripQuotePrefix(src[i]))
			}
			i-- // the loop's i++ will advance past the last quote line
			add(renderQuote(quote, width), false)
			continue
		}
		// Prose: word-wrapped, so lines after the first are soft continuations.
		add(wrapLineInline(line, width), true)
	}
	return lines, cont
}

// isQuoteLine reports whether a rendered body line is a Markdown-style quote
// line (">" optionally followed by a space). Mirrors graph.isQuoteLine on the
// send side so quoting round-trips.
func isQuoteLine(line string) bool {
	return strings.HasPrefix(strings.TrimLeft(line, " "), ">")
}

// stripQuotePrefix removes the leading ">" (and one following space) from a
// quote line, returning the quoted text.
func stripQuotePrefix(line string) string {
	t := strings.TrimLeft(line, " ")
	t = strings.TrimPrefix(t, ">")
	return strings.TrimPrefix(t, " ")
}

// renderQuote styles a run of quoted lines as a block with a coloured left bar,
// word-wrapping the quoted text to the remaining width. Inline `code` spans in
// the quote are styled like normal prose.
func renderQuote(quote []string, width int) string {
	const bar = "▌ "
	barW := lipgloss.Width(bar)
	textW := width - barW
	if textW < 1 {
		textW = 1
	}
	var rows []string
	for _, q := range quote {
		wrapped := wrapLineInline(q, textW)
		for _, wl := range strings.Split(wrapped, "\n") {
			rows = append(rows, styles.QuoteBar.Render(bar)+styles.Quote.Render(wl))
		}
	}
	return strings.Join(rows, "\n")
}

// renderCodeBlock styles a code block's lines as one dim panel. Each line is
// padded to a uniform inner width so the background forms a solid rectangle;
// lines wider than the available space are truncated with an ellipsis (code is
// not reflowed). When the language is recognized, the code is syntax-highlighted
// per token; otherwise it renders in a single code colour. An optional language
// label sits on a header row.
func renderCodeBlock(code []string, lang string, width int, codeStyle *chroma.Style) string {
	// Inner width available for code text inside the panel's horizontal padding
	// (1 cell each side).
	inner := width - 2
	if inner < 1 {
		inner = 1
	}

	// Panel width is the widest (plain) line, clamped to the viewport, so short
	// snippets don't stretch the full width. Widths are measured on the plain
	// text — never the highlighted text — so syntax-colour escapes don't skew it.
	panel := 0
	for _, c := range code {
		if w := ansi.StringWidth(c); w > panel {
			panel = w
		}
	}
	if lang != "" && panel < ansi.StringWidth(lang) {
		panel = ansi.StringWidth(lang)
	}
	if panel > inner {
		panel = inner
	}
	if panel < 1 {
		panel = 1
	}

	// Syntax-highlight when possible; highlighted[i] aligns with code[i]. The
	// panel background comes from the chroma theme (so it reads as one coherent
	// theme), falling back to the app's own dim code background.
	highlighted := highlightCode(code, lang, codeStyle)
	bg := codeBlockBackground(codeStyle)
	if bg == nil {
		bg = styles.CodeBlockBg
	}
	bgStyle := lipgloss.NewStyle().Background(bg)

	var rows []string
	if lang != "" {
		label := lipgloss.NewStyle().Background(bg).Foreground(styles.PurpleLt).Bold(true).Padding(0, 1)
		rows = append(rows, label.Width(panel+2).Render(lang))
	}
	for i, c := range code {
		styled := ""
		if highlighted != nil {
			styled = highlighted[i]
		}
		rows = append(rows, renderCodeLine(c, styled, panel, bgStyle))
	}
	if len(rows) == 0 {
		// Empty block: render a single blank padded row so it's still visible.
		rows = append(rows, bgStyle.Width(panel+2).Render(""))
	}
	return strings.Join(rows, "\n")
}

// renderCodeLine renders one code line into the panel. plain is the raw text
// (used for width math and the un-highlighted fallback); styled is the same line
// with syntax-colour escapes ("" when highlighting is unavailable); bgStyle
// carries the theme background. The line is truncated to panel cells and padded
// with background-coloured spaces so every row is exactly panel+2 wide (1 cell
// padding each side).
func renderCodeLine(plain, styled string, panel int, bgStyle lipgloss.Style) string {
	if styled == "" {
		// Plain path: lipgloss handles truncation-by-render width + padding.
		if ansi.StringWidth(plain) > panel {
			plain = ansi.Truncate(plain, panel, "…")
		}
		return bgStyle.Foreground(styles.CodeFg).Width(panel+2).Padding(0, 1).Render(plain)
	}

	// Highlighted path: the styled text already carries fg+bg escapes, so we
	// truncate and pad manually (escape-aware) rather than re-styling, which
	// would clobber the per-token colours.
	body := styled
	if ansi.StringWidth(plain) > panel {
		body = ansi.Truncate(styled, panel, "…")
	}
	pad := panel - ansi.StringWidth(body)
	if pad < 0 {
		pad = 0
	}
	// One cell of background padding on each side, plus right-fill to panel.
	return bgStyle.Render(" ") + body + bgStyle.Render(strings.Repeat(" ", pad)) + bgStyle.Render(" ")
}

// inlineCodeRe matches a `backtick` span within prose so it can be styled.
var inlineCodeRe = regexp.MustCompile("`([^`]+)`")

// wrapLineInline word-wraps a prose line to width, then re-styles any
// `backtick` spans on it with the inline-code style. Wrapping happens first
// (on the plain text, so widths are correct) and styling is applied to the
// wrapped output, which keeps the backtick markers intact across line breaks.
func wrapLineInline(line string, width int) string {
	wrapped := wrapLine(line, width)
	if !strings.Contains(wrapped, "`") {
		return wrapped
	}
	return inlineCodeRe.ReplaceAllStringFunc(wrapped, func(m string) string {
		inner := strings.Trim(m, "`")
		return styles.InlineCode.Render(inner)
	})
}

func wrapLine(line string, width int) string {
	words := strings.Fields(line)
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	cur := 0
	for _, w := range words {
		// Use display width (not byte length) so wide glyphs such as emoji,
		// which occupy two terminal cells, wrap correctly.
		wl := ansi.StringWidth(w)
		if cur == 0 {
			b.WriteString(w)
			cur = wl
			continue
		}
		if cur+1+wl > width {
			b.WriteString("\n")
			b.WriteString(w)
			cur = wl
		} else {
			b.WriteString(" ")
			b.WriteString(w)
			cur += 1 + wl
		}
	}
	return b.String()
}
