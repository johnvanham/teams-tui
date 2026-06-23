package ui

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/jvh/teams-tui/internal/clipboard"
	"github.com/jvh/teams-tui/internal/graph"
)

// sortMembers orders conversation members deterministically: by display name,
// then user ID. This keeps participant lists and chat names stable across the
// unordered member lists Graph returns on each poll.
func sortMembers(members []graph.ConversationMember) {
	sort.SliceStable(members, func(i, j int) bool {
		if members[i].DisplayName != members[j].DisplayName {
			return members[i].DisplayName < members[j].DisplayName
		}
		return members[i].UserID < members[j].UserID
	})
}

// Update is the central event handler.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleResize(msg)

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)

	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)

	case tea.FocusMsg:
		m.focused = true
		return m, nil

	case tea.BlurMsg:
		m.focused = false
		return m, nil
	}

	// Non-key messages.
	switch msg := msg.(type) {
	case deviceCodeMsg:
		m.deviceCode = msg.code
		m.phase = phaseAuthWaiting
		return m, pollTokenCmd(m.ctx, m.auth, m.store, msg.code)

	case authDoneMsg:
		m.tokens = msg.tokens
		m.client = graph.NewClient(m.cfg.GraphBaseURL, msg.tokens)
		m.phase = phaseLoading
		return m, tea.Batch(
			loadMeCmd(m.ctx, m.client),
			loadChatsCmd(m.ctx, m.client),
			loadMeetingsCmd(m.ctx, m.client, m.meetingLookahead()),
			pollTickCmd(m.pollInterval()),
			fastTickCmd(fastInterval),
			meetingTickCmd(time.Minute),
			presenceTickCmd(presenceInterval),
		)

	case meMsg:
		m.me = msg.me
		// Now that we know our own ID, start maintaining a presence session
		// (heartbeat) and load our current status for the footer.
		return m, tea.Batch(
			loadMyPresenceCmd(m.ctx, m.client),
			keepSessionCmd(m.ctx, m.client, m.me.ID, m.sessionID, nil, sessionExpiry),
			sessionTickCmd(sessionInterval),
		)

	case myPresenceMsg:
		if msg.presence != nil {
			m.myPresence = msg.presence
		}
		return m, nil

	case sessionRefreshedMsg:
		// Heartbeat applied; refresh our own presence for the footer.
		return m, loadMyPresenceCmd(m.ctx, m.client)

	case presenceSetMsg:
		// Status change applied; refresh footer + keep session alive.
		return m, loadMyPresenceCmd(m.ctx, m.client)

	case sessionTickMsg:
		var cmd tea.Cmd
		if m.client != nil && m.me != nil {
			cmd = keepSessionCmd(m.ctx, m.client, m.me.ID, m.sessionID, m.preferredOption(), sessionExpiry)
		}
		return m, tea.Batch(cmd, sessionTickCmd(sessionInterval))

	case chatsMsg:
		return m.handleChats(msg)

	case messagesMsg:
		return m.handleMessages(msg)

	case olderMessagesMsg:
		return m.handleOlderMessages(msg)

	case messagesErrMsg:
		return m.handleMessagesErr(msg)

	case sentMsg:
		m.compose.Reset()
		m.clearPendingImage()
		m.layout() // shrink the compose box back to its minimum
		// Immediately refresh the affected chat for snappy feedback.
		return m, loadMessagesCmd(m.ctx, m.client, msg.chatID)

	case imagePastedMsg:
		if msg.err != nil {
			if errors.Is(msg.err, clipboard.ErrNoImage) {
				m.errText = "No image on the clipboard to paste."
			} else {
				m.errText = "Couldn't read clipboard: " + msg.err.Error()
			}
			return m, nil
		}
		m.pendingImage = msg.data
		m.pendingImageCT = msg.contentType
		// Move focus to the compose box so the user can add a caption and send.
		m.focus = focusCompose
		m.errText = ""
		return m, m.compose.Focus()

	case peopleMsg:
		return m.handlePeople(msg)

	case chatCreatedMsg:
		return m.handleChatCreated(msg)

	case imageOpenedMsg:
		m.openingImage = false
		if msg.err != nil {
			m.errText = "Couldn't open image: " + msg.err.Error()
		}
		return m, nil

	case editedMsg:
		if msg.err != nil {
			m.errText = "Couldn't edit message: " + msg.err.Error()
			return m, nil
		}
		// Refresh the chat so the edited text (and Teams' "Edited" marker) show.
		return m, loadMessagesCmd(m.ctx, m.client, msg.chatID)

	case reactedMsg:
		if msg.err != nil {
			m.errText = "Couldn't update reaction: " + msg.err.Error()
			return m, nil
		}
		// Refresh so the updated reaction summary appears under the message.
		return m, loadMessagesCmd(m.ctx, m.client, msg.chatID)

	case meetingsMsg:
		return m.handleMeetings(msg)

	case presencesMsg:
		return m.handlePresences(msg)

	case pollTickMsg:
		return m.handlePollTick()

	case fastTickMsg:
		return m.handleFastTick()

	case meetingTickMsg:
		return m, tea.Batch(
			loadMeetingsCmd(m.ctx, m.client, m.meetingLookahead()),
			meetingTickCmd(time.Minute),
		)

	case presenceTickMsg:
		var presCmd tea.Cmd
		if m.client != nil {
			presCmd = loadPresencesCmd(m.ctx, m.client, m.presenceWatchIDs())
		}
		return m, tea.Batch(presCmd, presenceTickCmd(presenceInterval))

	case errMsg:
		return m.handleError(msg)
	}

	// Spinner ticks and any other component messages.
	return m.updateComponents(msg)
}

func (m Model) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.ready = true
	m.layout()
	return m, nil
}

// chatIndexAtY maps a screen Y coordinate to an absolute chat index in the
// list, or -1 if the click is outside the list's item rows. It mirrors the
// layout geometry in view.go: title bar, optional banner, sidebar top border,
// the "Chats" header row, then the list items (each delegate item spans
// Height+Spacing rows, with the item's first row being its title).
func (m Model) chatIndexAtY(y int) int {
	bannerRows := 0
	if m.activeBanner() != "" {
		bannerRows = 1
	}
	// First screen row occupied by the list's first item.
	listTop := titleHeight + bannerRows + 1 /*border*/ + sidebarHeaderRows
	rowInList := y - listTop
	if rowInList < 0 {
		return -1
	}
	rowsPerItem := m.delegateRows()
	if rowsPerItem <= 0 {
		return -1
	}
	rowOnPage := rowInList / rowsPerItem
	perPage := m.list.Paginator.PerPage
	if perPage <= 0 {
		return -1
	}
	abs := m.list.Paginator.Page*perPage + rowOnPage
	items := m.list.VisibleItems()
	if abs < 0 || abs >= len(items) {
		return -1
	}
	return abs
}

// delegateRows is the number of screen rows each list item occupies
// (item height + inter-item spacing).
func (m Model) delegateRows() int {
	return 2 /*delegate height*/ + 1 /*spacing*/
}

// messagesContentTop is the screen Y of the first content row inside the
// messages viewport. It mirrors the right-column geometry in viewReady():
// title bar, optional banner, the participants header, then the pane's top
// border.
func (m Model) messagesContentTop() int {
	bannerRows := 0
	if m.activeBanner() != "" {
		bannerRows = 1
	}
	return titleHeight + bannerRows + participantsHeaderRows + 1 /*pane top border*/
}

// imageAtY maps a screen Y coordinate to the index of an image placeholder in
// convImages, if the click landed on one. It converts the screen row to a
// viewport content line (accounting for scroll offset) and looks it up in the
// imageLines map built during rendering.
func (m Model) imageAtY(y int) (int, bool) {
	if m.currentChat == "" || len(m.imageLines) == 0 {
		return 0, false
	}
	rowInPane := y - m.messagesContentTop()
	if rowInPane < 0 || rowInPane >= m.viewport.Height() {
		return 0, false
	}
	contentLine := m.viewport.YOffset() + rowInPane
	idx, ok := m.imageLines[contentLine]
	return idx, ok
}

// msgAtY maps a screen Y coordinate to the index of the message that owns that
// row in the conversation, using msgLineStart recorded during rendering. A click
// anywhere within a message's block (from its header down to just before the
// next message) selects that message.
func (m Model) msgAtY(y int) (int, bool) {
	if m.currentChat == "" || len(m.msgLineStart) == 0 {
		return 0, false
	}
	rowInPane := y - m.messagesContentTop()
	if rowInPane < 0 || rowInPane >= m.viewport.Height() {
		return 0, false
	}
	contentLine := m.viewport.YOffset() + rowInPane
	// Find the last message whose header starts at or before this line.
	idx := -1
	for i, start := range m.msgLineStart {
		if start <= contentLine {
			idx = i
		} else {
			break
		}
	}
	if idx < 0 {
		return 0, false
	}
	return idx, true
}

// withinSidebar reports whether an X coordinate falls inside the sidebar
// column (excluding its left/right borders).
func (m Model) withinSidebar(x int) bool {
	return x >= 1 && x < sidebarWidth-1
}

// composeTop is the screen Y of the compose box's top border. Below the
// participants header sits the messages viewport (its content height plus its
// top/bottom borders), then any open emoji/reaction picker, then the compose
// box. A click at or below this row lands on the compose box.
func (m Model) composeTop() int {
	return m.messagesContentTop() + m.viewport.Height() + 1 /*viewport bottom border*/ +
		m.emojiPickerHeight() + m.reactPickerHeight() + m.emojiBrowserHeight()
}

// withinCompose reports whether a screen Y coordinate falls on the compose box
// (at or below its top border).
func (m Model) withinCompose(y int) bool {
	return y >= m.composeTop()
}

// handleMouseClick activates the clicked chat and jumps focus to compose.
func (m Model) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if m.phase != phaseReady || msg.Button != tea.MouseLeft {
		return m, nil
	}
	if !m.withinSidebar(msg.X) {
		if msg.X >= sidebarWidth {
			// A click on the compose box focuses it so the user can type.
			if m.withinCompose(msg.Y) {
				m.focus = focusCompose
				m.renderConversation() // drop the messages-pane selection highlight
				return m, m.compose.Focus()
			}
			// A click directly on an image placeholder opens that image.
			if idx, ok := m.imageAtY(msg.Y); ok {
				return m.viewImageAt(idx)
			}
			// Otherwise it's a click in the messages pane: focus it and select
			// the clicked message so react/quote act on it.
			m.focus = focusMessages
			m.compose.Blur()
			if idx, ok := m.msgAtY(msg.Y); ok {
				m.selectedMsg = idx
			}
			m.renderConversation()
		}
		return m, nil
	}
	// In contacts mode the sidebar holds the people list, whose row geometry
	// the chat click-mapping doesn't track; keep contact selection keyboard-
	// driven (focus the sidebar so arrows/enter work).
	if m.sidebarMode == sidebarContacts {
		m.focus = focusChats
		m.compose.Blur()
		return m, nil
	}
	idx := m.chatIndexAtY(msg.Y)
	if idx < 0 {
		return m, nil
	}
	m.list.Select(idx)
	c := m.selectedChatID()
	if c == "" {
		return m, nil
	}
	openCmd := m.openChat(c)
	m.focus = focusCompose
	return m, tea.Batch(openCmd, m.compose.Focus())
}

// handleMouseWheel scrolls whichever pane the pointer is over: the chat list
// when over the sidebar, otherwise the active conversation viewport.
func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.phase != phaseReady {
		return m, nil
	}
	if m.withinSidebar(msg.X) {
		var cmd tea.Cmd
		if m.sidebarMode == sidebarContacts {
			m.contacts, cmd = m.contacts.Update(msg)
		} else {
			m.list, cmd = m.list.Update(msg)
		}
		return m, cmd
	}
	// Scroll the conversation. A larger step per wheel notch feels more
	// responsive, especially on trackpads.
	switch msg.Button {
	case tea.MouseWheelUp:
		m.viewport.ScrollUp(wheelScrollLines)
		return m, m.maybeLoadOlder()
	case tea.MouseWheelDown:
		m.viewport.ScrollDown(wheelScrollLines)
	}
	return m, nil
}

// maybeLoadOlder fetches an older page of messages when the conversation is
// scrolled near the top and more history is available. Returns nil otherwise.
func (m *Model) maybeLoadOlder() tea.Cmd {
	if m.currentChat == "" || m.client == nil {
		return nil
	}
	// Trigger a little before the very top so new content is ready in time.
	if m.viewport.YOffset() > 2 {
		return nil
	}
	next := m.nextLink[m.currentChat]
	if next == "" || m.loadingMore[m.currentChat] {
		return nil
	}
	m.loadingMore[m.currentChat] = true
	return loadOlderMessagesCmd(m.ctx, m.client, m.currentChat, next)
}

// handleStatusPickerKey handles input while the status-picker popup is open.
func (m Model) handleStatusPickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+s":
		m.showStatus = false
		return m, nil
	case "enter":
		m.showStatus = false
		if it, ok := m.statusPicker.SelectedItem().(statusItem); ok {
			opt := it.opt
			m.chosenStatus = &opt
			// Optimistically reflect the choice in the footer immediately.
			m.myPresence = &graph.Presence{Availability: opt.Availability, Activity: opt.Activity}
			if m.client != nil && m.me != nil {
				return m, setStatusCmd(m.ctx, m.client, m.me.ID, m.sessionID, opt)
			}
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.statusPicker, cmd = m.statusPicker.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Global quit always available. Best-effort: end our presence session so we
	// don't leave a stale status lingering (it would otherwise expire on its
	// own after sessionExpiry).
	if key.Matches(msg, m.keys.Quit) {
		if m.client != nil && m.me != nil {
			return m, tea.Sequence(
				clearSessionCmd(m.ctx, m.client, m.me.ID, m.sessionID),
				tea.Quit,
			)
		}
		return m, tea.Quit
	}

	if m.phase != phaseReady {
		// During auth/loading only quit is meaningful.
		return m, nil
	}

	// Status-picker popup captures all input while open.
	if m.showStatus {
		return m.handleStatusPickerKey(msg)
	}

	// Reaction picker captures all input while open.
	if m.reactPicker {
		return m.handleReactPickerKey(msg)
	}

	// Emoji browser captures all input while open.
	if m.emojiBrowser {
		return m.handleEmojiBrowserKey(msg)
	}

	// Open the full emoji browser (ctrl+:) while composing. It lists every
	// emoji and filters as you type; the chosen glyph is inserted at the cursor.
	if key.Matches(msg, m.keys.Emoji) && m.focus == focusCompose {
		m.openEmojiBrowser()
		m.layout()
		return m, nil
	}

	// Open the status picker.
	if key.Matches(msg, m.keys.Status) {
		m.showStatus = true
		return m, nil
	}

	// Toggle the sidebar between chats and contacts. Switching to contacts
	// moves focus there and lazily loads the people list on first use.
	if key.Matches(msg, m.keys.Contacts) {
		return m.toggleContacts()
	}

	// Contacts mode: the sidebar shows the people list. Route input to it,
	// with Enter starting a 1:1 chat with the selected person.
	if m.sidebarMode == sidebarContacts && m.focus == focusChats {
		return m.handleContactsKey(msg)
	}

	// If actively typing a filter, let the list consume keys (so Enter/Esc and
	// characters edit the filter rather than triggering global actions).
	if m.focus == focusChats && m.list.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	// Toggle help. "ctrl+g" and "f1" always work; "?" only toggles help when the
	// compose box is NOT focused, so it can still be typed into messages.
	switch s := msg.String(); {
	case s == "ctrl+g", s == "f1":
		m.help.ShowAll = !m.help.ShowAll
		m.layout()
		return m, nil
	case s == "?" && m.focus != focusCompose:
		m.help.ShowAll = !m.help.ShowAll
		m.layout()
		return m, nil
	}

	// View an image: opens the most recent image in the conversation in the OS
	// default viewer/browser. Available from any pane so it doesn't fight the
	// compose box's text input (ctrl+y isn't a printable key, so it's safe to
	// bind even while composing).
	if key.Matches(msg, m.keys.Image) {
		return m.viewImage()
	}

	// Edit: load the user's most recent message into the compose box for an
	// in-place edit. Available from any pane.
	if key.Matches(msg, m.keys.Edit) {
		return m.startEdit()
	}

	// Paste image: read an image from the OS clipboard and stage it for the
	// next send. The compose text (if any) becomes the image's caption. Only
	// meaningful with a chat open; we can't paste a binary image into the text
	// composer, so this is handled here rather than falling through to it.
	if key.Matches(msg, m.keys.Paste) {
		if m.currentChat == "" {
			return m, nil
		}
		m.errText = "Reading clipboard…"
		return m, pasteImageCmd()
	}

	// The inline emoji picker (only open while composing) claims navigation and
	// selection keys before pane-switching/refresh so Tab accepts a suggestion
	// instead of changing panes. Printable keys still fall through to the
	// composer below, re-filtering the popup without interrupting typing.
	if m.emojiPicker && m.focus == focusCompose {
		switch {
		case msg.String() == "esc":
			m.closeEmojiPicker()
			return m, nil
		case msg.String() == "up" || msg.String() == "ctrl+p":
			m.emojiPickerMove(-1)
			return m, nil
		case msg.String() == "down" || msg.String() == "ctrl+n":
			m.emojiPickerMove(1)
			return m, nil
		case key.Matches(msg, m.keys.Send), msg.String() == "tab":
			if m.applyEmojiSelection() {
				m.layout()
				return m, nil
			}
		}
	}

	switch {
	case key.Matches(msg, m.keys.NextPane):
		m.cycleFocus(1)
		return m, m.focusCmd()

	case key.Matches(msg, m.keys.PrevPane):
		m.cycleFocus(-1)
		return m, m.focusCmd()

	case key.Matches(msg, m.keys.Refresh):
		return m, m.refreshCmd()
	}

	// Messages pane: selection navigation and the react/quote actions are
	// handled here, before the jump-to-compose shortcut, so j/k/r/q act on the
	// selected message instead of being typed into the composer.
	if m.focus == focusMessages {
		return m.handleMessagesPaneKey(msg)
	}

	// Quality-of-life: if the user starts typing a printable character while
	// the chat list or messages pane is focused, jump straight to the compose
	// box and feed the keystroke there. This excludes "/" (which starts list
	// filtering) and the vim-style j/k navigation keys when browsing chats.
	if m.focus != focusCompose && m.currentChat != "" && isTypingKey(msg) {
		if !(m.focus == focusChats && isReservedListKey(msg)) {
			m.focus = focusCompose
			focusCmd := m.compose.Focus()
			var cmd tea.Cmd
			m.compose, cmd = m.compose.Update(msg)
			m.autoReplaceEmoticon()
			m.refreshEmojiPicker()
			m.layout() // grow the box to fit the new content
			return m, tea.Batch(focusCmd, cmd)
		}
	}

	// Compose-specific: Enter sends, alt+enter inserts a newline. (Emoji-picker
	// navigation/selection keys are intercepted earlier, before pane switching.)
	if m.focus == focusCompose {
		// Esc clears the compose box: it empties any typed text, exits an
		// in-progress edit (restoring the new-message state), discards a staged
		// clipboard image, and dismisses the emoji popup.
		if msg.String() == "esc" {
			m.editingMsgID = ""
			m.clearPendingImage()
			m.compose.Reset()
			m.closeEmojiPicker()
			m.layout()
			return m, nil
		}
		if key.Matches(msg, m.keys.Send) {
			// While the cursor sits inside an unclosed ``` code fence, Enter
			// continues the block (inserts a newline) instead of sending, so a
			// multi-line code block can be typed naturally. Close the fence with
			// another ``` line and Enter then sends as usual.
			if m.inOpenCodeBlock() {
				m.compose.InsertRune('\n')
				m.closeEmojiPicker()
				m.layout()
				return m, nil
			}
			return m.trySend()
		}
		if key.Matches(msg, m.keys.Newline) {
			// Inject a literal newline at the cursor.
			m.compose.InsertRune('\n')
			m.closeEmojiPicker()
			m.layout()
			return m, nil
		}
		var cmd tea.Cmd
		m.compose, cmd = m.compose.Update(msg)
		// Auto-replace a completed text emoticon (":-)" -> "🙂") inline, then
		// re-evaluate the :shortcode: autocomplete suggestions against the new
		// text.
		m.autoReplaceEmoticon()
		m.refreshEmojiPicker()
		// Recompute layout so the compose box grows/shrinks with its content.
		m.layout()
		return m, cmd
	}

	// Chats pane: arrows preview a chat; Enter opens it and jumps to compose.
	if m.focus == focusChats {
		// Enter: open the selected chat and move focus to the compose box so
		// the user can immediately start typing.
		if key.Matches(msg, m.keys.Send) {
			if c := m.selectedChatID(); c != "" {
				openCmd := m.openChat(c)
				m.focus = focusCompose
				return m, tea.Batch(openCmd, m.compose.Focus())
			}
			return m, nil
		}

		prevIndex := m.list.Index()
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		cmds := []tea.Cmd{cmd}
		// Navigating with arrows previews the highlighted chat in place,
		// without stealing focus from the list.
		if m.list.Index() != prevIndex {
			if c := m.selectedChatID(); c != "" {
				cmds = append(cmds, m.openChat(c))
			}
		}
		return m, tea.Batch(cmds...)
	}

	return m, nil
}

// handleMessagesPaneKey handles input while the messages pane is focused:
// up/down (and k/j) move the message selection (auto-scrolling to keep it in
// view); page/home/end scroll the viewport directly; r reacts to the selected
// message (toggling off if already reacted with the chosen emoji); q quotes it
// into a reply. Other keys fall through to viewport scrolling.
func (m Model) handleMessagesPaneKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		return m.moveSelection(-1)
	case "down", "j":
		return m.moveSelection(1)
	case "g", "home":
		if len(m.convMsgs) > 0 {
			m.selectedMsg = 0
			m.renderConversation()
			m.scrollToSelection()
		}
		return m, m.maybeLoadOlder()
	case "G", "end":
		if len(m.convMsgs) > 0 {
			m.selectedMsg = len(m.convMsgs) - 1
			m.renderConversation()
			m.scrollToSelection()
		}
		return m, nil
	case "r":
		return m.reactToSelected()
	case "q":
		return m.quoteSelected()
	}
	// Anything else (pgup/pgdn, etc.): plain viewport scroll, loading older
	// history when scrolled near the top.
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, tea.Batch(cmd, m.maybeLoadOlder())
}

// moveSelection shifts the selected message by delta (clamped to the ends),
// re-renders so the highlight moves, scrolls to keep it visible, and loads older
// history if the selection reaches the top.
func (m Model) moveSelection(delta int) (tea.Model, tea.Cmd) {
	if len(m.convMsgs) == 0 {
		return m, nil
	}
	m.selectedMsg += delta
	if m.selectedMsg < 0 {
		m.selectedMsg = 0
	}
	if m.selectedMsg >= len(m.convMsgs) {
		m.selectedMsg = len(m.convMsgs) - 1
	}
	m.renderConversation()
	m.scrollToSelection()
	var older tea.Cmd
	if m.selectedMsg == 0 {
		older = m.maybeLoadOlder()
	}
	return m, older
}

func (m Model) handleChats(msg chatsMsg) (tea.Model, tea.Cmd) {
	// Order the chats client-side so the list is stable across polls regardless
	// of how Graph orders ties/null timestamps: most-recent activity first,
	// with chat ID as a deterministic tiebreak.
	chats := append([]graph.Chat(nil), msg.chats...)
	sort.SliceStable(chats, func(i, j int) bool {
		ai, aj := chats[i].LastActivity(), chats[j].LastActivity()
		if !ai.Equal(aj) {
			return ai.After(aj)
		}
		return chats[i].ID < chats[j].ID
	})

	// Record the chats (and their order) into the model, then let
	// rebuildChatList construct the list items and recompute unread state from
	// the same single place used for between-poll refreshes.
	m.chatOrder = m.chatOrder[:0]
	for _, c := range chats {
		// Graph does not guarantee a stable member order across polls, which
		// made the participant header and chat names jump around. Sort members
		// deterministically (by display name, then user ID) so the order is
		// stable between refreshes.
		sortMembers(c.Members)
		m.chatOrder = append(m.chatOrder, c.ID)
		m.chats[c.ID] = c
	}

	// Fire desktop notifications for new messages across all chats, using the
	// previews Graph returned. The first poll only establishes baselines so the
	// existing backlog doesn't ping at startup.
	m.notifyNewChatMessages(chats, !m.notifyBaselined)
	m.notifyBaselined = true

	// A successful chat refresh clears any stale transient error in the status
	// bar so it doesn't linger after recovery.
	m.errText = ""

	// Only rebuild the list when the displayed chats actually changed; this
	// avoids the per-poll cost of SetItems (pagination + delegate work).
	cmd := m.rebuildChatList()

	if m.phase == phaseLoading {
		m.phase = phaseReady
		m.layout()
	}

	var openCmd tea.Cmd
	// Auto-open the first chat if none selected yet.
	if m.currentChat == "" && len(m.chatOrder) > 0 {
		openCmd = m.openChat(m.chatOrder[0])
	}
	// Refresh presence for whoever is in the (now-)current chat.
	presCmd := loadPresencesCmd(m.ctx, m.client, m.currentChatParticipantIDs())

	// If the chat-list poll's preview shows the open chat has a message newer
	// than our last sync, fetch it now instead of waiting for the next fast
	// tick — this closes the window where a desktop notification fires before
	// the message appears in the open conversation.
	syncCmd := m.syncOpenChatIfBehind()
	return m, tea.Batch(cmd, openCmd, presCmd, syncCmd)
}

// syncOpenChatIfBehind issues an incremental message fetch for the currently
// open chat when the latest chat-list preview indicates a newer message than
// we've synced. It relies on the same lastSync horizon the fast tick uses, so
// the fetch is cheap and dedups naturally against the periodic poll. Returns
// nil when there's nothing to do (no open chat, no baseline yet, or already up
// to date).
func (m Model) syncOpenChatIfBehind() tea.Cmd {
	if m.client == nil || m.currentChat == "" {
		return nil
	}
	since, ok := m.lastSync[m.currentChat]
	if !ok {
		// No baseline yet (chat just opened); a full load is already in flight.
		return nil
	}
	c, ok := m.chats[m.currentChat]
	if !ok {
		return nil
	}
	prev := c.LastMessagePreview
	if prev == nil || !prev.CreatedAt.After(since) {
		return nil
	}
	return loadNewMessagesCmd(m.ctx, m.client, m.currentChat, since)
}

// handlePeople populates the contacts list from a people lookup. A failure
// (e.g. People.Read not consented) surfaces as a transient status notice
// rather than tearing down the UI.
func (m Model) handlePeople(msg peopleMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.errText = "Couldn't load contacts: " + msg.err.Error()
		return m, nil
	}
	m.people = msg.people
	setCmd := m.rebuildContacts()
	// Fetch presence for the loaded contacts so their availability shows next
	// to each name once it arrives.
	presCmd := loadPresencesCmd(m.ctx, m.client, m.peopleUserIDs())
	return m, tea.Batch(setCmd, presCmd)
}

// rebuildContacts repopulates the contacts list from m.people, baking each
// contact's latest known presence into its item. Called when people load and
// whenever presence updates.
func (m *Model) rebuildContacts() tea.Cmd {
	items := make([]list.Item, 0, len(m.people))
	for _, p := range m.people {
		items = append(items, personItem{person: p, presence: m.presences[p.ID]})
	}
	return m.contacts.SetItems(items)
}

// peopleUserIDs returns the user IDs of the currently loaded contacts for a
// presence lookup.
func (m Model) peopleUserIDs() []string {
	ids := make([]string, 0, len(m.people))
	for _, p := range m.people {
		if p.ID != "" {
			ids = append(ids, p.ID)
		}
	}
	return ids
}

// presenceWatchIDs returns the de-duplicated set of user IDs whose presence the
// periodic tick should refresh: the current chat's participants always, plus
// the loaded contacts while the contacts panel is open so their status stays
// current.
func (m Model) presenceWatchIDs() []string {
	seen := make(map[string]bool)
	var ids []string
	add := func(list []string) {
		for _, id := range list {
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	add(m.currentChatParticipantIDs())
	if m.sidebarMode == sidebarContacts {
		add(m.peopleUserIDs())
	}
	return ids
}

// handleChatCreated opens a freshly created 1:1 chat: it switches the sidebar
// back to chats, refreshes the chat list so the new chat appears, and opens it.
func (m Model) handleChatCreated(msg chatCreatedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.errText = "Couldn't start chat: " + msg.err.Error()
		return m, nil
	}
	if msg.chat == nil || msg.chat.ID == "" {
		return m, nil
	}
	// Cache the chat so currentChatParticipantIDs/header lookups work even
	// before the next chat-list refresh lands.
	sortMembers(msg.chat.Members)
	m.chats[msg.chat.ID] = *msg.chat
	m.sidebarMode = sidebarChats
	m.focus = focusCompose
	openCmd := m.openChat(msg.chat.ID)
	return m, tea.Batch(
		loadChatsCmd(m.ctx, m.client), // surface the new chat in the sidebar
		openCmd,
		m.compose.Focus(),
	)
}

func (m Model) handleMessages(msg messagesMsg) (tea.Model, tea.Cmd) {
	delete(m.chatErrors, msg.chatID) // a successful load clears any prior error

	// Record the page link for fetching older messages, but only on the first
	// (full) load of a chat. Incremental "since" polls carry no usable nextLink.
	if !msg.incremental {
		if _, seen := m.nextLink[msg.chatID]; !seen {
			m.nextLink[msg.chatID] = msg.nextLink
		}
	}

	existing := m.messages[msg.chatID]

	// An incremental poll with no new messages is the common case — nothing to
	// do, no re-render.
	if msg.incremental && len(msg.messages) == 0 {
		return m, nil
	}

	// Fast path: a full poll that returned the identical newest page.
	if !msg.incremental && sameMessages(existing, msg.messages) {
		m.advanceSync(msg.chatID, msg.messages)
		return m, nil
	}

	// Merge new/changed messages into any history we already have, preserving
	// older messages loaded via upward pagination.
	merged := mergeMessages(existing, msg.messages)
	m.advanceSync(msg.chatID, msg.messages)
	if sameMessages(existing, merged) {
		return m, nil
	}
	m.messages[msg.chatID] = merged

	if msg.chatID == m.currentChat {
		// The user is looking at this chat, so anything that just arrived is
		// considered read. Advance the local read horizon past the newest
		// message so the chat won't flash unread once they switch away.
		m.advanceRead(msg.chatID, msg.messages)
		atBottom := m.viewport.AtBottom()
		m.renderConversation()
		if atBottom {
			m.viewport.GotoBottom()
		}
	}
	return m, nil
}

// notifyNewChatMessages fires a desktop notification for chats whose newest
// message (per Graph's lastMessagePreview) is newer than the last one we
// notified about. Driven from the chat-list poll, it covers every chat — not
// just the open one — using the preview Graph already returns, so no extra
// message fetch is needed.
//
// A per-chat horizon (notifiedUntil) ensures each message pings at most once.
// The first time we ever see the chat list (baseline), we record horizons
// without notifying so the user isn't flooded with their entire backlog at
// startup. Messages the user sent themselves and the chat they're actively
// viewing (while the terminal is focused) are skipped.
func (m *Model) notifyNewChatMessages(chats []graph.Chat, baseline bool) {
	self := m.selfID()

	for _, c := range chats {
		prev := c.LastMessagePreview
		if prev == nil || prev.CreatedAt.IsZero() {
			continue
		}

		last := prev.CreatedAt
		alreadyNotified := m.notifiedUntil[c.ID]

		// Advance the horizon up front so a message never re-notifies, then
		// decide whether this one warrants a ping.
		if last.After(m.notifiedUntil[c.ID]) {
			m.notifiedUntil[c.ID] = last
		}

		if baseline {
			continue // startup: establish horizons silently
		}
		if !last.After(alreadyNotified) {
			continue // nothing newer than what we've already pinged
		}
		// Skip our own messages.
		if from := prev.From; from != nil && from.User != nil && self != "" && from.User.ID == self {
			continue
		}
		// Skip the chat the user is actively watching.
		if c.ID == m.currentChat && m.focused {
			continue
		}

		text := prev.Body.PlainText()
		if text == "" {
			text = "New message"
		}
		title := "Teams"
		if name := c.DisplayName(self); name != "" {
			title = name
		}
		m.notifier.Notify(title, notifyPreview(text))
	}
}

// notifyPreview shortens a message body for a notification: it collapses it to a
// single line and truncates long text so the OS popup stays compact.
func notifyPreview(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	const max = 140
	if len([]rune(s)) > max {
		r := []rune(s)
		s = strings.TrimSpace(string(r[:max])) + "…"
	}
	return s
}

// advanceRead moves a chat's local read horizon past the newest message in
// msgs. Used while a chat is open so incoming messages the user is actively
// viewing don't later resurface as unread.
func (m *Model) advanceRead(chatID string, msgs []graph.Message) {
	newest := m.readUntil[chatID]
	for _, msg := range msgs {
		if msg.CreatedAt.After(newest) {
			newest = msg.CreatedAt
		}
	}
	if newest.After(m.readUntil[chatID]) {
		m.readUntil[chatID] = newest
	}
}

// advanceSync records the newest lastModifiedDateTime seen for a chat so the
// next incremental poll only fetches messages after it.
func (m *Model) advanceSync(chatID string, msgs []graph.Message) {
	newest := m.lastSync[chatID]
	for _, msg := range msgs {
		if msg.LastModified.After(newest) {
			newest = msg.LastModified
		}
	}
	if newest.After(m.lastSync[chatID]) {
		m.lastSync[chatID] = newest
	}
}

// handleOlderMessages prepends an older page of messages to the buffer when the
// user scrolls to the top, preserving the scroll position.
func (m Model) handleOlderMessages(msg olderMessagesMsg) (tea.Model, tea.Cmd) {
	m.loadingMore[msg.chatID] = false
	m.nextLink[msg.chatID] = msg.nextLink
	if len(msg.messages) == 0 {
		return m, nil
	}
	before := m.messages[msg.chatID]
	m.messages[msg.chatID] = mergeMessages(before, msg.messages)

	if msg.chatID == m.currentChat {
		// Preserve the viewing position: remember how far from the bottom we
		// were, re-render, then restore so the view doesn't jump.
		linesFromTop := m.viewport.TotalLineCount() - m.viewport.YOffset()
		m.renderConversation()
		newOffset := m.viewport.TotalLineCount() - linesFromTop
		if newOffset < 0 {
			newOffset = 0
		}
		m.viewport.SetYOffset(newOffset)
	}
	return m, nil
}

// sameMessages reports whether two message slices represent the same content
// for rendering purposes (same length, IDs, and last-modified timestamps so
// edits/reactions still trigger a refresh).
func sameMessages(a, b []graph.Message) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || !a[i].LastModified.Equal(b[i].LastModified) {
			return false
		}
	}
	return true
}

// mergeMessages unions two message slices by ID, keeping the most recently
// modified copy of any duplicate. Order is not guaranteed; the renderer sorts
// by creation time.
func mergeMessages(existing, incoming []graph.Message) []graph.Message {
	if len(existing) == 0 {
		return incoming
	}
	if len(incoming) == 0 {
		return existing
	}
	byID := make(map[string]graph.Message, len(existing)+len(incoming))
	order := make([]string, 0, len(existing)+len(incoming))
	add := func(msgs []graph.Message) {
		for _, msg := range msgs {
			if prev, ok := byID[msg.ID]; ok {
				if msg.LastModified.After(prev.LastModified) {
					byID[msg.ID] = msg
				}
				continue
			}
			byID[msg.ID] = msg
			order = append(order, msg.ID)
		}
	}
	add(existing)
	add(incoming)
	out := make([]graph.Message, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out
}

// handleMessagesErr records a per-chat message-load failure. Some chats (e.g.
// certain meeting chats) return 403 under delegated permissions; rather than
// flashing a sticky error in the status bar on every poll, we note it against
// the chat and show an inline notice in the conversation.
func (m Model) handleMessagesErr(msg messagesErrMsg) (tea.Model, tea.Cmd) {
	notice := "This conversation can't be loaded."
	if ae, ok := msg.err.(*graph.APIError); ok && ae.Status == 403 {
		notice = "You don't have access to this conversation's messages (it may be a meeting chat)."
	}
	m.chatErrors[msg.chatID] = notice
	if msg.chatID == m.currentChat {
		m.renderConversation()
	}
	return m, nil
}

// handlePresences merges fetched presence info into the model. The conversation
// header re-renders naturally on the next View (it reads m.presences directly),
// but the contacts list bakes presence into its items, so rebuild it when an
// update touches a loaded contact.
func (m Model) handlePresences(msg presencesMsg) (tea.Model, tea.Cmd) {
	affectsContacts := false
	for id, p := range msg.presences {
		m.presences[id] = p
		if !affectsContacts {
			for _, person := range m.people {
				if person.ID == id {
					affectsContacts = true
					break
				}
			}
		}
	}
	if affectsContacts {
		return m, m.rebuildContacts()
	}
	return m, nil
}

func (m Model) handleMeetings(msg meetingsMsg) (tea.Model, tea.Cmd) {
	now := time.Now().UTC()
	for _, ev := range msg.events {
		if ev.IsCancelled || m.alerted[ev.ID] {
			continue
		}
		start, err := ev.Start.Time()
		if err != nil {
			continue
		}
		delta := start.Sub(now)
		// Alert when a meeting starts within the lookahead window.
		if delta >= -time.Minute && delta <= m.meetingLookahead() {
			m.alerted[ev.ID] = true
			subject := ev.Subject
			if subject == "" {
				subject = "Untitled meeting"
			}
			when := humanizeUntil(delta)
			m.banner = "Meeting: " + subject + " " + when
			m.bannerUntil = time.Now().Add(30 * time.Second)
			m.notifier.Alert("Teams meeting "+when, subject)
		}
	}
	return m, nil
}

// handlePollTick refreshes the chat list on the slower cadence. The open chat's
// messages are kept fresh separately by the fast incremental tick.
func (m Model) handlePollTick() (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{pollTickCmd(m.pollInterval())}
	if m.client != nil {
		cmds = append(cmds, loadChatsCmd(m.ctx, m.client))
	}
	return m, tea.Batch(cmds...)
}

// handleFastTick performs a cheap incremental refresh of the open chat (only
// messages changed since the last sync), then reschedules itself. The interval
// adapts to whether the terminal is focused.
func (m Model) handleFastTick() (tea.Model, tea.Cmd) {
	interval := fastInterval
	if !m.focused {
		interval = fastIntervalIdle
	}
	var cmd tea.Cmd
	if m.client != nil && m.currentChat != "" {
		if since, ok := m.lastSync[m.currentChat]; ok {
			cmd = loadNewMessagesCmd(m.ctx, m.client, m.currentChat, since)
		} else {
			// No baseline yet (chat just opened); a full load is in flight.
			cmd = nil
		}
	}
	return m, tea.Batch(cmd, fastTickCmd(interval))
}

func (m Model) handleError(msg errMsg) (tea.Model, tea.Cmd) {
	if m.phase == phaseAuthStarting || m.phase == phaseAuthWaiting || m.phase == phaseLoading {
		m.phase = phaseError
		m.errText = msg.err.Error()
		return m, nil
	}
	// Transient error in ready state: show in status, keep running.
	m.errText = msg.err.Error()
	return m, nil
}

// updateComponents forwards a message to all sub-components (used for spinner
// ticks and similar broadcast messages).
func (m Model) updateComponents(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	m.spinner, cmd = m.spinner.Update(msg)
	cmds = append(cmds, cmd)

	if m.phase == phaseReady {
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
		m.contacts, cmd = m.contacts.Update(msg)
		cmds = append(cmds, cmd)
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
		m.compose, cmd = m.compose.Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

// --- helpers ---

func (m *Model) cycleFocus(dir int) {
	order := []focusArea{focusChats, focusMessages, focusCompose}
	idx := 0
	for i, f := range order {
		if f == m.focus {
			idx = i
		}
	}
	idx = (idx + dir + len(order)) % len(order)
	m.focus = order[idx]
}

// focusCmd applies focus/blur side effects (textarea cursor) and returns any
// resulting command.
func (m *Model) focusCmd() tea.Cmd {
	// Re-render so the message-selection highlight shows only while the messages
	// pane is focused (and is removed when focus moves elsewhere). Keep the
	// selection visible when entering the pane.
	m.renderConversation()
	if m.focus == focusMessages {
		m.scrollToSelection()
	}
	if m.focus == focusCompose {
		return m.compose.Focus()
	}
	// Leaving the composer dismisses the inline emoji popup.
	m.closeEmojiPicker()
	m.compose.Blur()
	return nil
}

func (m Model) selectedChatID() string {
	if it, ok := m.list.SelectedItem().(chatItem); ok {
		return it.chat.ID
	}
	return ""
}

// isTypingKey reports whether a key press represents an ordinary printable
// character typed by the user (no Ctrl/Alt modifier). Such keys should start a
// message rather than act as navigation.
func isTypingKey(msg tea.KeyPressMsg) bool {
	if msg.Text == "" {
		return false // special keys: arrows, enter, tab, backspace, etc.
	}
	if msg.Mod&(tea.ModCtrl|tea.ModAlt) != 0 {
		return false
	}
	return true
}

// isReservedListKey reports whether a printable key has special meaning in the
// chat list and therefore should not trigger the jump-to-compose behavior.
func isReservedListKey(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "/", "j", "k", "g", "G", "q":
		return true
	}
	return false
}

// toggleContacts flips the sidebar between the chat list and the contacts
// (people) list. Entering contacts mode focuses the sidebar and triggers a
// one-time people load; leaving it returns focus to the chats.
func (m Model) toggleContacts() (tea.Model, tea.Cmd) {
	if m.sidebarMode == sidebarContacts {
		m.sidebarMode = sidebarChats
		m.focus = focusChats
		m.compose.Blur()
		return m, nil
	}
	m.sidebarMode = sidebarContacts
	m.focus = focusChats
	m.compose.Blur()
	var cmd tea.Cmd
	if !m.contactsLoaded && m.client != nil {
		m.contactsLoaded = true
		cmd = loadPeopleCmd(m.ctx, m.client, "")
	}
	return m, cmd
}

// handleContactsKey processes input while the sidebar is in contacts mode.
// Enter (when not editing a filter) starts a 1:1 chat with the highlighted
// person; all other keys drive the list, whose filter doubles as a people
// search box.
func (m Model) handleContactsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// While typing in the filter box, let the list consume everything so
	// Enter/Esc and characters edit the search rather than triggering actions.
	if m.contacts.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.contacts, cmd = m.contacts.Update(msg)
		return m, cmd
	}
	if key.Matches(msg, m.keys.Send) {
		return m.startChatWithSelected()
	}
	var cmd tea.Cmd
	m.contacts, cmd = m.contacts.Update(msg)
	return m, cmd
}

// startChatWithSelected creates (or reuses) a 1:1 chat with the contact
// currently highlighted in the contacts list.
func (m Model) startChatWithSelected() (tea.Model, tea.Cmd) {
	it, ok := m.contacts.SelectedItem().(personItem)
	if !ok || m.client == nil || m.me == nil {
		return m, nil
	}
	if it.person.ID == "" {
		m.errText = "That contact can't be messaged (missing user id)."
		return m, nil
	}
	return m, createChatCmd(m.ctx, m.client, m.me.ID, it.person.ID)
}

// viewImage downloads (if needed) and opens the most recent image in the open
// conversation using the OS default app/browser. The newest image is the one
// the user most likely just received, so it's the default target.
func (m Model) viewImage() (tea.Model, tea.Cmd) {
	if len(m.convImages) == 0 {
		m.errText = "No images in this conversation."
		return m, nil
	}
	return m.viewImageAt(len(m.convImages) - 1)
}

// viewImageAt downloads (if needed) and opens the image at the given index in
// convImages using the OS default app/browser. Shared by the keybinding (newest
// image) and clicking a specific placeholder.
func (m Model) viewImageAt(idx int) (tea.Model, tea.Cmd) {
	if m.openingImage || m.client == nil {
		return m, nil
	}
	if idx < 0 || idx >= len(m.convImages) {
		return m, nil
	}
	m.openingImage = true
	m.errText = "Opening image…"
	return m, openImageCmd(m.ctx, m.client, m.convImages[idx])
}

// openChat sets the active chat, renders any cached messages, and fetches fresh
// messages plus participant presence.
func (m *Model) openChat(chatID string) tea.Cmd {
	m.currentChat = chatID
	// Opening a chat marks it read: advance the local read horizon past its
	// latest message so the unread highlight clears immediately, before the
	// server viewpoint catches up on the next poll. Persist the read state to
	// the server too so it syncs across devices.
	var readCmd tea.Cmd
	if c, ok := m.chats[chatID]; ok {
		horizon := c.LastActivity()
		if horizon.After(m.readUntil[chatID]) {
			m.readUntil[chatID] = horizon
		}
		if self := m.selfID(); self != "" {
			readCmd = markChatReadCmd(m.ctx, m.client, chatID, self)
		}
	}
	listCmd := m.rebuildChatList()
	m.renderConversation()
	m.viewport.GotoBottom()
	return tea.Batch(
		listCmd,
		loadMessagesCmd(m.ctx, m.client, chatID),
		loadPresencesCmd(m.ctx, m.client, m.currentChatParticipantIDs()),
		readCmd,
	)
}

// rebuildChatList regenerates the chat list items from the cached chats in
// m.chatOrder, recomputing each chat's unread state. It returns the list's
// SetItems command (or nil when nothing visibly changed, keyed off chatsSig).
// Use it to refresh unread highlighting between polls, e.g. right after opening
// a chat marks it read.
func (m *Model) rebuildChatList() tea.Cmd {
	self := m.selfID()
	items := make([]list.Item, 0, len(m.chatOrder))
	var sig strings.Builder
	for _, id := range m.chatOrder {
		c, ok := m.chats[id]
		if !ok {
			continue
		}
		unread := id != m.currentChat && c.Unread(self, m.readUntil[id])
		item := newChatItem(c, self, unread)
		items = append(items, item)
		sig.WriteString(c.ID)
		sig.WriteByte('\x1f')
		sig.WriteString(item.Title())
		sig.WriteByte('\x1f')
		sig.WriteString(item.Description())
		sig.WriteByte('\x1f')
		if unread {
			sig.WriteByte('1')
		} else {
			sig.WriteByte('0')
		}
		sig.WriteByte('\x1e')
	}
	newSig := sig.String()
	if newSig == m.chatsSig {
		return nil
	}
	m.chatsSig = newSig
	return m.list.SetItems(items)
}

// inOpenCodeBlock reports whether the compose buffer currently has an unclosed
// ``` code fence at or before the cursor's line — i.e. the user is typing inside
// a code block that hasn't been closed yet. It counts fence lines (a line whose
// first non-space content is ```) up to and including the cursor's line; an odd
// count means a block is open. This lets Enter continue the block instead of
// sending the message.
func (m Model) inOpenCodeBlock() bool {
	lines := strings.Split(m.compose.Value(), "\n")
	cur := m.compose.Line()
	if cur >= len(lines) {
		cur = len(lines) - 1
	}
	fences := 0
	for i := 0; i <= cur && i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), codeFence) {
			fences++
		}
	}
	return fences%2 == 1
}

func (m Model) trySend() (tea.Model, tea.Cmd) {
	// Convert emoji shortcodes (:thumbsup:) and text emoticons (:-)) to Unicode
	// before sending. This covers every outbound path below (new message, edit,
	// and image caption), since they all derive from this one string.
	text := strings.TrimSpace(graph.ReplaceShortcodes(m.compose.Value()))
	m.closeEmojiPicker()
	if m.currentChat == "" {
		return m, nil
	}
	chatID := m.currentChat

	// A staged clipboard image takes precedence: post it as an inline image,
	// using any typed text as the caption. Editing an existing message and
	// attaching an image are mutually exclusive, so this is checked first.
	if len(m.pendingImage) > 0 {
		img := m.pendingImage
		ct := m.pendingImageCT
		m.clearPendingImage()
		m.compose.Reset()
		m.editingMsgID = ""
		m.layout()
		return m, sendImageCmd(m.ctx, m.client, chatID, img, ct, text)
	}

	if text == "" {
		return m, nil
	}
	// If we're editing an existing message, PATCH it instead of posting a new
	// one, then leave edit mode.
	if m.editingMsgID != "" {
		msgID := m.editingMsgID
		m.editingMsgID = ""
		m.compose.Reset()
		m.layout()
		return m, editMessageCmd(m.ctx, m.client, chatID, msgID, text)
	}
	m.compose.Reset()
	m.layout() // shrink the compose box back immediately
	return m, sendMessageCmd(m.ctx, m.client, chatID, text)
}

// startEdit loads one of the signed-in user's messages into the compose box for
// an in-place edit and focuses the composer. When a message is selected in the
// messages pane and it belongs to the user, that message is edited; otherwise it
// falls back to the user's most recent message. Enter commits the edit, Esc
// cancels.
func (m Model) startEdit() (tea.Model, tea.Cmd) {
	if m.currentChat == "" || m.me == nil {
		return m, nil
	}
	// Prefer the currently selected message if it's the user's own (so clicking
	// a message and pressing edit edits that one). Deleted messages can't be
	// edited.
	msg, ok := m.editableSelection()
	if !ok {
		msg, ok = m.latestOwnMessage(m.currentChat)
	}
	if !ok {
		m.errText = "No message of yours to edit in this chat."
		return m, nil
	}
	m.editingMsgID = msg.ID
	m.sidebarMode = sidebarChats
	m.focus = focusCompose
	m.compose.SetValue(msg.Body.PlainText())
	m.layout()
	return m, m.compose.Focus()
}

// editableSelection returns the selected message when it is the signed-in user's
// own, non-deleted message (the candidate for an in-place edit).
func (m Model) editableSelection() (graph.Message, bool) {
	sel, ok := m.selectedMessage()
	if !ok || m.me == nil || sel.DeletedAt != nil {
		return graph.Message{}, false
	}
	if sel.From == nil || sel.From.User == nil || sel.From.User.ID != m.me.ID {
		return graph.Message{}, false
	}
	return sel, true
}

// reactToSelected opens the reaction emoji picker for the selected message so
// the user can choose an emoji to react with. The actual POST happens when they
// pick one (handleReactPickerKey).
func (m Model) reactToSelected() (tea.Model, tea.Cmd) {
	sel, ok := m.selectedMessage()
	if !ok || sel.ID == "" {
		return m, nil
	}
	m.openReactPicker(sel.ID)
	m.layout()
	return m, nil
}

// handleReactPickerKey handles input while the reaction picker is open: typing
// filters the emoji, up/down move the highlight, enter applies the reaction
// (toggling it off if the user already reacted with that emoji), and esc
// cancels.
func (m Model) handleReactPickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.closeReactPicker()
		m.layout()
		return m, nil
	case "up", "ctrl+p":
		m.reactPickerMove(-1)
		return m, nil
	case "down", "ctrl+n":
		m.reactPickerMove(1)
		return m, nil
	case "backspace":
		if r := []rune(m.reactQuery); len(r) > 0 {
			m.reactQuery = string(r[:len(r)-1])
			m.reactSel = 0
			m.refreshReactMatches()
			m.layout()
		}
		return m, nil
	case "enter":
		return m.applyReaction()
	}
	// Printable characters extend the search query.
	if msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
		m.reactQuery += msg.Text
		m.reactSel = 0
		m.refreshReactMatches()
		m.layout()
	}
	return m, nil
}

// handleEmojiBrowserKey handles input while the full emoji browser is open:
// typing filters the list, up/down move the highlight (the list scrolls to keep
// it visible), enter inserts the highlighted glyph at the compose cursor, and
// esc cancels.
func (m Model) handleEmojiBrowserKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.closeEmojiBrowser()
		m.layout()
		return m, nil
	case "up", "ctrl+p":
		m.browserMove(-1)
		return m, nil
	case "down", "ctrl+n":
		m.browserMove(1)
		return m, nil
	case "backspace":
		if r := []rune(m.browserQuery); len(r) > 0 {
			m.browserQuery = string(r[:len(r)-1])
			m.browserSel = 0
			m.refreshBrowserMatches()
			m.layout()
		}
		return m, nil
	case "enter", "tab":
		m.applyBrowserSelection()
		m.layout()
		return m, nil
	}
	// Printable characters extend the search query.
	if msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
		m.browserQuery += msg.Text
		m.browserSel = 0
		m.refreshBrowserMatches()
		m.layout()
	}
	return m, nil
}

// applyReaction posts (or removes) the highlighted reaction for the message the
// picker was opened on. If the signed-in user already reacted to that message
// with the chosen emoji, the reaction is toggled off; otherwise it's added.
func (m Model) applyReaction() (tea.Model, tea.Cmd) {
	glyph, ok := m.selectedReaction()
	msgID := m.reactMsgID
	chatID := m.currentChat
	m.closeReactPicker()
	m.layout()
	if !ok || msgID == "" || chatID == "" || m.client == nil {
		return m, nil
	}
	// Decide add vs. remove from the message's current reactions.
	remove := false
	if self := m.selfID(); self != "" {
		for _, msg := range m.convMsgs {
			if msg.ID == msgID {
				remove = msg.UserReacted(self, glyph)
				break
			}
		}
	}
	return m, reactCmd(m.ctx, m.client, chatID, msgID, glyph, remove)
}

// quoteSelected prefills the compose box with the selected message quoted as a
// blockquote and moves focus to compose so the user can type their reply. The
// "> "-prefixed lines round-trip through ComposeHTML into a <blockquote> on
// send (see graph/compose.go).
func (m Model) quoteSelected() (tea.Model, tea.Cmd) {
	sel, ok := m.selectedMessage()
	if !ok {
		return m, nil
	}
	text := sel.Body.PlainText()
	if sel.DeletedAt != nil {
		text = "(message deleted)"
	}
	var b strings.Builder
	b.WriteString("> ")
	b.WriteString(sel.SenderName())
	b.WriteString(" wrote:\n")
	for _, ln := range strings.Split(text, "\n") {
		b.WriteString("> ")
		b.WriteString(ln)
		b.WriteString("\n")
	}
	b.WriteString("\n") // blank line so the reply starts below the quote
	// Preserve any half-typed reply by appending the quote prefix in front of it
	// only when the composer is empty; otherwise prepend the quote.
	existing := m.compose.Value()
	m.compose.SetValue(b.String() + existing)
	m.compose.MoveToEnd()
	m.focus = focusCompose
	m.layout()
	return m, m.compose.Focus()
}

// clearPendingImage discards any clipboard image staged for the next send.
func (m *Model) clearPendingImage() {
	m.pendingImage = nil
	m.pendingImageCT = ""
}

// latestOwnMessage returns the signed-in user's most recent non-deleted message
// in a chat (by creation time), if any.
func (m Model) latestOwnMessage(chatID string) (graph.Message, bool) {
	msgs := m.messages[chatID]
	var best graph.Message
	found := false
	for _, msg := range msgs {
		if msg.DeletedAt != nil {
			continue
		}
		if msg.From == nil || msg.From.User == nil || msg.From.User.ID != m.me.ID {
			continue
		}
		if !found || msg.CreatedAt.After(best.CreatedAt) {
			best = msg
			found = true
		}
	}
	return best, found
}

func (m Model) refreshCmd() tea.Cmd {
	if m.client == nil {
		return nil
	}
	cmds := []tea.Cmd{loadChatsCmd(m.ctx, m.client)}
	if m.currentChat != "" {
		cmds = append(cmds, loadMessagesCmd(m.ctx, m.client, m.currentChat))
	}
	return tea.Batch(cmds...)
}

func humanizeUntil(d time.Duration) string {
	if d <= time.Minute {
		return "starting now"
	}
	mins := int(d.Minutes())
	if mins == 1 {
		return "in 1 minute"
	}
	return "in " + strconv.Itoa(mins) + " minutes"
}
