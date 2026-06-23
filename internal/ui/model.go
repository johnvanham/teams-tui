// Package ui implements the Bubble Tea v2 terminal interface for teams-tui.
package ui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/alecthomas/chroma/v2"
	chromastyles "github.com/alecthomas/chroma/v2/styles"

	"github.com/jvh/teams-tui/internal/auth"
	"github.com/jvh/teams-tui/internal/config"
	"github.com/jvh/teams-tui/internal/graph"
	"github.com/jvh/teams-tui/internal/notify"
)

// phase represents the top-level screen state.
type phase int

const (
	phaseAuthStarting phase = iota // contacting Entra for a device code
	phaseAuthWaiting               // showing code, polling for token
	phaseLoading                   // authenticated, loading profile/chats
	phaseReady                     // main chat UI
	phaseError                     // fatal error screen
)

// focusArea identifies which pane currently has keyboard focus.
type focusArea int

const (
	focusChats focusArea = iota
	focusMessages
	focusCompose
)

// sidebarMode selects what the left column shows: the chat list or the
// contacts (people) list used to start a new conversation.
type sidebarMode int

const (
	sidebarChats sidebarMode = iota
	sidebarContacts
)

// Model is the root Bubble Tea model.
type Model struct {
	ctx      context.Context
	cfg      *config.Config
	auth     *auth.Authenticator
	store    *auth.Store
	notifier *notify.Notifier

	// codeStyle is the resolved chroma theme used to syntax-highlight code
	// blocks (from cfg.CodeBlockStyle, falling back to the chroma default).
	codeStyle *chroma.Style

	// Populated after authentication.
	tokens *auth.TokenSource
	client *graph.Client
	me     *graph.User

	// UI components.
	list         list.Model
	contacts     list.Model // people list shown in the sidebar's contacts mode
	viewport     viewport.Model
	compose      textarea.Model
	spinner      spinner.Model
	help         help.Model
	statusPicker list.Model
	keys         keyMap

	// State.
	phase           phase
	focus           focusArea
	sidebarMode     sidebarMode // chats vs contacts in the left column
	contactsLoaded  bool        // whether the people list has been fetched once
	deviceCode      *auth.DeviceCode
	width           int
	height          int
	ready           bool
	currentChat     string
	messages        map[string][]graph.Message
	chatOrder       []string                  // chat IDs in list order
	chats           map[string]graph.Chat     // chat ID -> chat (for member lookup)
	presences       map[string]graph.Presence // user ID -> presence
	chatErrors      map[string]string         // chat ID -> message-load error notice
	nextLink        map[string]string         // chat ID -> @odata.nextLink (older msgs)
	loadingMore     map[string]bool           // chat ID -> older-page fetch in flight
	lastSync        map[string]time.Time      // chat ID -> newest lastModified seen
	readUntil       map[string]time.Time      // chat ID -> local read horizon (mark-as-read)
	notifiedUntil   map[string]time.Time      // chat ID -> newest message we've already notified about
	notifyBaselined bool                      // first chat-list poll seen (suppresses startup backlog pings)
	chatsSig        string                    // signature of the rendered chat list
	people          []graph.Person            // contacts currently shown in the sidebar
	convImages      []graph.ImageRef          // images in the open chat, display order
	imageLines      map[int]int               // viewport content line -> convImages index
	convMsgs        []graph.Message           // open chat's messages in display (oldest-first) order
	msgLineStart    []int                     // convMsgs[i] -> viewport content line of its header
	selectedMsg     int                       // index into convMsgs of the highlighted message (-1 = none)
	openingImage    bool                      // an image download/open is in flight
	editingMsgID    string                    // message ID being edited ("" if composing new)
	pendingImage    []byte                    // image pasted from the clipboard, awaiting send
	pendingImageCT  string                    // MIME type of pendingImage (e.g. "image/png")
	focused         bool                      // terminal window has focus
	myPresence      *graph.Presence           // signed-in user's own presence
	sessionID       string                    // app presence session ID (client ID)
	showStatus      bool                      // status-picker overlay visible
	chosenStatus    *graph.PresenceOption     // status explicitly set by the user

	// Inline emoji autocomplete (triggered by ":" + >=2 chars in the composer).
	emojiPicker  bool                   // popup currently shown
	emojiMatches []graph.EmojiShortcode // current matches for the typed prefix
	emojiSel     int                    // highlighted match index
	emojiQuery   string                 // the typed token (without leading ':')

	// Reaction picker (opened with "r" on a selected message): a searchable
	// emoji chooser whose selection is POSTed as a reaction.
	reactPicker  bool                   // reaction overlay shown
	reactQuery   string                 // typed search text (shortcode prefix)
	reactMatches []graph.EmojiShortcode // current matches for reactQuery
	reactSel     int                    // highlighted match index
	reactMsgID   string                 // message being reacted to

	// Emoji browser (opened with "ctrl+:" while composing): lists every emoji
	// and filters as you type; the chosen glyph is inserted at the cursor.
	emojiBrowser   bool                   // browser overlay shown
	browserQuery   string                 // typed search text (shortcode prefix)
	browserMatches []graph.EmojiShortcode // current matches for browserQuery
	browserSel     int                    // highlighted match index

	// Mention autocomplete (triggered by "@" in the composer of a group chat):
	// tab/enter completes a participant's name, inserting "@DisplayName" and
	// recording the member so a Graph @-mention is attached on send.
	mentionPicker  bool                       // popup currently shown
	mentionMatches []graph.ConversationMember // members matching the typed query
	mentionSel     int                        // highlighted match index
	mentionQuery   string                     // text typed after the "@"
	mentions       []graph.Mention            // mentions chosen for the pending message

	// Transient notices.
	errText     string
	banner      string
	bannerUntil time.Time

	// Meeting de-duplication: event IDs already alerted.
	alerted map[string]bool
}

// New constructs the root model and its sub-components.
func New(ctx context.Context, cfg *config.Config, a *auth.Authenticator, store *auth.Store, n *notify.Notifier) Model {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))

	l := list.New(nil, newChatDelegate(), 0, 0)
	l.Title = "Chats"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	// Hide the list's own title so its content starts directly with the first
	// item. This keeps the click-to-row geometry predictable; the "Chats"
	// header is drawn separately in the sidebar chrome.
	l.SetShowTitle(false)

	// Contacts list shares the chat list's geometry so the sidebar can swap
	// between them without a layout change, but uses a delegate that colours
	// each contact's presence glyph. Filtering doubles as a "search people" box.
	cl := list.New(nil, newContactDelegate(), 0, 0)
	cl.Title = "Contacts"
	cl.SetShowHelp(false)
	cl.SetShowStatusBar(false)
	cl.SetFilteringEnabled(true)
	cl.SetShowTitle(false)

	ta := textarea.New()
	// Let the textarea grow with its content (handling wrapped lines and
	// scrolling internally), from 1 row up to a cap applied in layout().
	ta.DynamicHeight = true
	ta.MinHeight = composeMinLines
	ta.MaxHeight = composeMinLines // real cap set in layout() from screen size
	ta.SetHeight(composeMinLines)
	ta.Blur()

	vp := viewport.New()

	// Status-picker popup list.
	pickerDelegate := list.NewDefaultDelegate()
	pickerDelegate.ShowDescription = false
	pickerDelegate.SetSpacing(0)
	pickerItems := make([]list.Item, 0, len(graph.SettablePresences))
	for _, opt := range graph.SettablePresences {
		pickerItems = append(pickerItems, statusItem{opt: opt})
	}
	// Height: title row + blank + one row per item.
	picker := list.New(pickerItems, pickerDelegate, 24, len(pickerItems)+2)
	picker.Title = "Set status"
	picker.SetShowHelp(false)
	picker.SetShowStatusBar(false)
	picker.SetFilteringEnabled(false)

	// Resolve the syntax-highlighting theme once. chromastyles.Get returns its
	// bland built-in fallback ("swapoff") for an unknown name, so when the
	// configured style isn't found we fall back to our own default instead.
	codeStyle := chromastyles.Get(cfg.CodeBlockStyle)
	if codeStyle == nil || (codeStyle.Name == "swapoff" && cfg.CodeBlockStyle != "swapoff") {
		codeStyle = chromastyles.Get(config.DefaultCodeBlockStyle)
	}

	return Model{
		ctx:           ctx,
		cfg:           cfg,
		codeStyle:     codeStyle,
		auth:          a,
		store:         store,
		notifier:      n,
		sessionID:     cfg.ClientID,
		list:          l,
		contacts:      cl,
		viewport:      vp,
		statusPicker:  picker,
		compose:       ta,
		spinner:       sp,
		help:          help.New(),
		keys:          defaultKeyMap(),
		phase:         phaseAuthStarting,
		focus:         focusChats,
		selectedMsg:   -1,
		messages:      make(map[string][]graph.Message),
		chats:         make(map[string]graph.Chat),
		presences:     make(map[string]graph.Presence),
		chatErrors:    make(map[string]string),
		nextLink:      make(map[string]string),
		loadingMore:   make(map[string]bool),
		lastSync:      make(map[string]time.Time),
		readUntil:     make(map[string]time.Time),
		notifiedUntil: make(map[string]time.Time),
		focused:       true, // assume focused until told otherwise
		alerted:       make(map[string]bool),
	}
}

// Init kicks off authentication: try the cached token first, otherwise begin
// the device-code flow.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.startAuthCmd(),
	)
}

// startAuthCmd attempts to reuse a cached token (refreshing if needed). If no
// usable token exists it triggers the device-code flow.
func (m Model) startAuthCmd() tea.Cmd {
	return func() tea.Msg {
		cached, err := m.store.Load()
		// Only reuse a cached token if it already covers every scope we now
		// require. A token minted before a permission was added (e.g. before
		// Chat.ReadWrite was consented) would otherwise refresh successfully
		// yet 403 on the first Graph call. In that case we re-authenticate.
		if err == nil && cached != nil && cached.RefreshToken != "" && cached.CoversScopes(m.cfg.Scopes) {
			ts := auth.NewTokenSource(m.auth, m.store, cached)
			// Validate by ensuring we can obtain an access token now.
			if _, terr := ts.Token(m.ctx); terr == nil {
				return authDoneMsg{ts}
			}
		}
		// Cached token missing/unusable/stale-scope; start a fresh sign-in.
		_ = m.store.Clear()
		dc, derr := m.auth.RequestDeviceCode(m.ctx)
		if derr != nil {
			return errMsg{derr}
		}
		return deviceCodeMsg{dc}
	}
}

// pollInterval returns the configured chat refresh interval.
func (m Model) pollInterval() time.Duration {
	return time.Duration(m.cfg.PollIntervalSeconds) * time.Second
}

// meetingLookahead returns how far ahead to look for meetings.
func (m Model) meetingLookahead() time.Duration {
	return time.Duration(m.cfg.MeetingLookaheadMinutes) * time.Minute
}

// selfID returns the signed-in user's id (empty before profile loads).
func (m Model) selfID() string {
	if m.me != nil {
		return m.me.ID
	}
	return ""
}

// preferredOption returns the status the user explicitly chose, if any, for the
// presence-session heartbeat to re-assert. Returns nil to default to Available.
func (m Model) preferredOption() *graph.PresenceOption {
	return m.chosenStatus
}

// currentChatParticipantIDs returns the user IDs of everyone in the current
// chat except the signed-in user (whose own presence we don't display).
func (m Model) currentChatParticipantIDs() []string {
	chat, ok := m.chats[m.currentChat]
	if !ok {
		return nil
	}
	self := m.selfID()
	seen := make(map[string]bool)
	var ids []string
	for _, mem := range chat.Members {
		if mem.UserID == "" || mem.UserID == self || seen[mem.UserID] {
			continue
		}
		seen[mem.UserID] = true
		ids = append(ids, mem.UserID)
	}
	return ids
}
