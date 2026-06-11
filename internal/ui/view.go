package ui

import (
	"fmt"
	"image/color"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
)

// layout recomputes component sizes from the current terminal dimensions. It
// is called on resize and whenever the help expansion changes the chrome size.
func (m *Model) layout() {
	if !m.ready {
		return
	}
	helpHeight := 1
	if m.help.ShowAll {
		helpHeight = 3
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
	// messages and the compose box, stealing height from the viewport).
	vpInnerH := bodyHeight - composeBoxH - 2 - participantsHeaderRows - m.emojiPickerHeight()
	if vpInnerH < 1 {
		vpInnerH = 1
	}

	m.viewport.SetWidth(rightInnerW)
	m.viewport.SetHeight(vpInnerH)
	m.help.SetWidth(m.width)

	// Re-render the conversation to fit the new width.
	m.renderConversation()
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

	// Messages pane.
	msgStyle := styles.MessagePaneBlurred
	if m.focus == focusMessages {
		msgStyle = styles.MessagePaneFocused
	}
	messages := msgStyle.Width(boxW).Render(m.viewport.View())

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
	rightParts = append(rightParts, compose)
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
	// Reset click/keybinding image state up front so every early return below
	// leaves no stale placeholders from a previously open chat.
	m.convImages = m.convImages[:0]
	m.imageLines = nil
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

	width := m.viewport.Width()
	var b strings.Builder
	// Collect every image in the conversation, in display order, so the image
	// keybinding can reference them by number ("open image [2]"). line tracks
	// the current 0-based content line so we can map a click on a placeholder
	// row back to its image (imageLines).
	m.imageLines = make(map[int]int)
	line := 0
	for _, msg := range ordered {
		text := msg.Body.PlainText()
		if msg.DeletedAt != nil {
			text = "(message deleted)"
		}
		images := msg.Images()
		// Skip truly empty messages (no text and no images).
		if text == "" && len(images) == 0 {
			continue
		}
		name := msg.SenderName()
		nameStyle := styles.SenderName
		if m.me != nil && msg.From != nil && msg.From.User != nil && msg.From.User.ID == m.me.ID {
			nameStyle = styles.SenderSelf
		}
		ts := msg.CreatedAt.Local().Format("15:04")
		header := nameStyle.Render(name) + " " + styles.Timestamp.Render(ts)
		b.WriteString(header)
		b.WriteString("\n")
		line++ // header row
		if text != "" {
			rendered := renderBody(text, width)
			b.WriteString(rendered)
			b.WriteString("\n")
			line += strings.Count(rendered, "\n") + 1
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
			line++ // placeholder row
		}
		if reactions := msg.ReactionSummary(); len(reactions) > 0 {
			b.WriteString(styles.Reaction.Render(strings.Join(reactions, "  ")))
			b.WriteString("\n")
			line++ // reaction row
		}
		b.WriteString("\n")
		line++ // blank separator row
	}
	m.viewport.SetContent(strings.TrimRight(b.String(), "\n"))
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
func renderBody(text string, width int) string {
	lines := strings.Split(text, "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		// An opening fence (```​ or ```lang) starts a code block that runs until
		// the matching closing fence (or end of text if Teams sent a malformed
		// block).
		if strings.HasPrefix(line, codeFence) {
			lang := strings.TrimSpace(strings.TrimPrefix(line, codeFence))
			var code []string
			i++
			for ; i < len(lines); i++ {
				if strings.HasPrefix(lines[i], codeFence) {
					break
				}
				code = append(code, lines[i])
			}
			out = append(out, renderCodeBlock(code, lang, width))
			continue
		}
		out = append(out, wrapLineInline(line, width))
	}
	return strings.Join(out, "\n")
}

// renderCodeBlock styles a code block's lines as one dim panel. Each line is
// padded to a uniform inner width so the background forms a solid rectangle;
// lines wider than the available space are truncated with an ellipsis (code is
// not reflowed). An optional language label sits on a header row.
func renderCodeBlock(code []string, lang string, width int) string {
	// Inner width available for code text inside the style's horizontal padding
	// (1 cell each side).
	inner := width - 2
	if inner < 1 {
		inner = 1
	}

	// Determine the panel width from the widest line, clamped to the viewport,
	// so short snippets don't stretch the full width.
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

	var rows []string
	if lang != "" {
		rows = append(rows, styles.CodeBlockLang.Width(panel+2).Render(lang))
	}
	for _, c := range code {
		if ansi.StringWidth(c) > panel {
			c = ansi.Truncate(c, panel, "…")
		}
		rows = append(rows, styles.CodeBlock.Width(panel+2).Render(c))
	}
	if len(rows) == 0 {
		// Empty block: render a single blank padded row so it's still visible.
		rows = append(rows, styles.CodeBlock.Width(panel+2).Render(""))
	}
	return strings.Join(rows, "\n")
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
