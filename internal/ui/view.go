package ui

import (
	"fmt"
	"image/color"
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
	// Reserve one row for the participants/presence header above the messages.
	vpInnerH := bodyHeight - composeBoxH - 2 - participantsHeaderRows
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

	right := lipgloss.JoinVertical(lipgloss.Left, participants, messages, compose)
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
	color := styles.LightGrey
	if ok {
		glyph = p.Glyph()
		switch p.Availability {
		case "Available", "AvailableIdle":
			color = styles.Green
		case "Busy", "BusyIdle", "InACall", "InAConferenceCall", "InAMeeting", "Presenting",
			"DoNotDisturb", "Focusing":
			color = styles.Red
		case "Away", "BeRightBack", "OutOfOffice":
			color = styles.Yellow
		default:
			color = styles.LightGrey
		}
	}
	return lipgloss.NewStyle().Background(bg).Foreground(color).Render(glyph)
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
	case focusMessages:
		left = "MESSAGES"
	case focusCompose:
		left = "COMPOSE"
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
	// keybinding can reference them by number ("open image [2]").
	m.convImages = m.convImages[:0]
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
		if text != "" {
			b.WriteString(wrap(text, width))
			b.WriteString("\n")
		}
		// Render a numbered placeholder for each image; the number matches the
		// index used by the "view image" action (1-based for humans).
		for _, img := range images {
			m.convImages = append(m.convImages, img)
			n := len(m.convImages)
			label := img.Name
			if label == "" {
				label = "image"
			}
			placeholder := fmt.Sprintf("🖼  [%d] %s — ctrl+v to view", n, label)
			b.WriteString(styles.ImagePlaceholder.Render(placeholder))
			b.WriteString("\n")
		}
		if reactions := msg.ReactionSummary(); len(reactions) > 0 {
			b.WriteString(styles.Reaction.Render(strings.Join(reactions, "  ")))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	m.viewport.SetContent(strings.TrimRight(b.String(), "\n"))
}

// wrap performs simple word wrapping at the given width.
func wrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		out = append(out, wrapLine(line, width))
	}
	return strings.Join(out, "\n")
}

func wrapLine(line string, width int) string {
	words := strings.Fields(line)
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	cur := 0
	for i, w := range words {
		wl := len(w)
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
		_ = i
	}
	return b.String()
}
