package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvh/teams-tui/internal/auth"
	"github.com/jvh/teams-tui/internal/clipboard"
	"github.com/jvh/teams-tui/internal/graph"
	"github.com/jvh/teams-tui/internal/open"
	"github.com/jvh/teams-tui/internal/spell"
)

// --- Messages flowing into Update ---

// deviceCodeMsg carries the device code to display during sign-in.
type deviceCodeMsg struct {
	code *auth.DeviceCode
}

// authDoneMsg signals the token source is ready (sign-in or cached token).
type authDoneMsg struct {
	tokens *auth.TokenSource
}

// errMsg is a generic error surfaced to the UI.
type errMsg struct{ err error }

// meMsg carries the signed-in user's profile.
type meMsg struct{ me *graph.User }

// chatsMsg carries the loaded chat list.
type chatsMsg struct{ chats []graph.Chat }

// messagesMsg carries messages for a specific chat.
type messagesMsg struct {
	chatID      string
	messages    []graph.Message
	nextLink    string // page link for older messages ("" if none/unknown)
	incremental bool   // true for a cheap "since" poll (merge, don't replace)
}

// olderMessagesMsg carries an older page of messages to prepend.
type olderMessagesMsg struct {
	chatID   string
	messages []graph.Message
	nextLink string
}

// messagesErrMsg reports a failure to load a specific chat's messages.
type messagesErrMsg struct {
	chatID string
	err    error
}

// sentMsg signals a message was sent successfully to a chat.
type sentMsg struct{ chatID string }

// imagePastedMsg carries an image read from the OS clipboard, ready to be
// attached to the next outgoing message. err is set when the clipboard held no
// image or the read failed.
type imagePastedMsg struct {
	data        []byte
	contentType string
	err         error
}

// editedMsg signals a message was edited successfully so the chat can refresh.
type editedMsg struct {
	chatID string
	err    error
}

// reactedMsg signals a reaction was added/removed so the chat can refresh.
type reactedMsg struct {
	chatID string
	err    error
}

// peopleMsg carries the loaded contacts (people) for the sidebar's contacts
// mode. err is set when the lookup failed (e.g. People.Read unconsented).
type peopleMsg struct {
	people []graph.Person
	err    error
}

// chatCreatedMsg signals a new 1:1 chat was created so it can be opened.
type chatCreatedMsg struct {
	chat *graph.Chat
	err  error
}

// imageOpenedMsg reports the result of downloading and opening an image in the
// OS default viewer/browser.
type imageOpenedMsg struct {
	name string
	err  error
}

// meetingsMsg carries upcoming events for notification checks.
type meetingsMsg struct{ events []graph.Event }

// presencesMsg carries presence info for chat participants.
type presencesMsg struct{ presences map[string]graph.Presence }

// myPresenceMsg carries the signed-in user's own presence.
type myPresenceMsg struct{ presence *graph.Presence }

// presenceSetMsg signals a status change completed (so we can refresh).
type presenceSetMsg struct{}

// pollTickMsg drives periodic refresh of the chat list (slower cadence).
type pollTickMsg time.Time

// fastTickMsg drives the rapid incremental refresh of the open chat.
type fastTickMsg time.Time

// meetingTickMsg drives periodic checks for upcoming meetings.
type meetingTickMsg time.Time

// presenceTickMsg drives periodic presence refresh.
type presenceTickMsg time.Time

// sessionTickMsg drives the presence-session keep-alive heartbeat.
type sessionTickMsg time.Time

// sessionRefreshedMsg signals a successful presence-session heartbeat.
type sessionRefreshedMsg struct{}

// spellDebounceMsg fires after the compose box has been idle briefly; gen ties
// it to the keystroke that scheduled it so stale ticks (superseded by newer
// typing) are ignored.
type spellDebounceMsg struct{ gen int }

// spellCheckedMsg carries the misspelled words found in the compose text. gen
// lets the model drop results from a check that a newer edit has superseded.
type spellCheckedMsg struct {
	gen   int
	words []spell.Misspelling
}

const (
	// wheelScrollLines is how many lines one mouse-wheel notch scrolls the
	// conversation. Higher feels more responsive, especially on trackpads.
	wheelScrollLines = 5
	// fastInterval is the rapid incremental poll of the open chat when the app
	// is focused — near-real-time without true push.
	fastInterval = 1 * time.Second
	// fastIntervalIdle is the backed-off open-chat poll when the terminal is
	// unfocused.
	fastIntervalIdle = 15 * time.Second
	// presenceInterval is how often participant presence is refreshed.
	presenceInterval = 30 * time.Second
	// sessionExpiry is the lifetime requested for the app presence session.
	sessionExpiry = 5 * time.Minute
	// sessionInterval is the heartbeat period; shorter than sessionExpiry so
	// the session never lapses.
	sessionInterval = 4 * time.Minute
	// spellDebounce is how long the compose box must be idle before a spell
	// check runs, so rapid typing collapses into one subprocess invocation.
	spellDebounce = 400 * time.Millisecond
)

// fastTickCmd schedules the next incremental open-chat poll.
func fastTickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return fastTickMsg(t) })
}

// --- Commands ---

// requestDeviceCodeCmd begins the device authorization flow.
func requestDeviceCodeCmd(ctx context.Context, a *auth.Authenticator) tea.Cmd {
	return func() tea.Msg {
		dc, err := a.RequestDeviceCode(ctx)
		if err != nil {
			return errMsg{err}
		}
		return deviceCodeMsg{dc}
	}
}

// pollTokenCmd polls for the token after the user has been shown the code,
// then persists it and constructs a TokenSource.
func pollTokenCmd(ctx context.Context, a *auth.Authenticator, store *auth.Store, dc *auth.DeviceCode) tea.Cmd {
	return func() tea.Msg {
		tok, err := a.PollToken(ctx, dc)
		if err != nil {
			return errMsg{err}
		}
		if err := store.Save(tok); err != nil {
			return errMsg{err}
		}
		return authDoneMsg{auth.NewTokenSource(a, store, tok)}
	}
}

// loadMeCmd fetches the signed-in user's profile.
func loadMeCmd(ctx context.Context, c *graph.Client) tea.Cmd {
	return func() tea.Msg {
		me, err := c.Me(ctx)
		if err != nil {
			return errMsg{err}
		}
		return meMsg{me}
	}
}

// loadChatsCmd fetches the chat list.
func loadChatsCmd(ctx context.Context, c *graph.Client) tea.Cmd {
	return func() tea.Msg {
		chats, err := c.ListChats(ctx, 50)
		if err != nil {
			return errMsg{err}
		}
		return chatsMsg{chats}
	}
}

// loadMessagesCmd fetches messages for a chat. Failures are reported as a
// per-chat messagesErrMsg (not a global errMsg) so that a chat we can't read
// (e.g. some meeting chats that 403 under delegated permissions) shows an
// inline notice instead of a sticky error in the status bar.
func loadMessagesCmd(ctx context.Context, c *graph.Client, chatID string) tea.Cmd {
	return func() tea.Msg {
		msgs, next, err := c.ListMessagesPage(ctx, chatID, 40)
		if err != nil {
			return messagesErrMsg{chatID: chatID, err: err}
		}
		return messagesMsg{chatID: chatID, messages: msgs, nextLink: next}
	}
}

// loadNewMessagesCmd fetches only messages changed since `since` for the open
// chat (cheap incremental poll). Results are merged into the buffer.
func loadNewMessagesCmd(ctx context.Context, c *graph.Client, chatID string, since time.Time) tea.Cmd {
	return func() tea.Msg {
		msgs, err := c.ListMessagesSince(ctx, chatID, since)
		if err != nil {
			// Non-fatal: skip this round, the next full poll will recover.
			return messagesMsg{chatID: chatID, messages: nil, incremental: true}
		}
		return messagesMsg{chatID: chatID, messages: msgs, incremental: true}
	}
}

// loadOlderMessagesCmd follows a chat's nextLink to fetch an older page.
func loadOlderMessagesCmd(ctx context.Context, c *graph.Client, chatID, nextLink string) tea.Cmd {
	return func() tea.Msg {
		msgs, next, err := c.FollowMessagesPage(ctx, nextLink)
		if err != nil {
			// Non-fatal: just stop paginating on error.
			return olderMessagesMsg{chatID: chatID, messages: nil, nextLink: ""}
		}
		return olderMessagesMsg{chatID: chatID, messages: msgs, nextLink: next}
	}
}

// sendMessageCmd posts a message to a chat, attaching any @-mentions so Graph
// notifies the mentioned participants.
func sendMessageCmd(ctx context.Context, c *graph.Client, chatID, text string, mentions []graph.Mention) tea.Cmd {
	return func() tea.Msg {
		if _, err := c.SendMessageWithMentions(ctx, chatID, text, mentions); err != nil {
			return errMsg{err}
		}
		return sentMsg{chatID: chatID}
	}
}

// reactCmd adds or removes a reaction on a message. When remove is true it
// calls UnsetReaction (toggling an existing reaction off); otherwise it adds the
// reaction with SetReaction. Either way it reports completion via reactedMsg so
// the chat can refresh and show the updated reaction summary.
func reactCmd(ctx context.Context, c *graph.Client, chatID, messageID, emoji string, remove bool) tea.Cmd {
	return func() tea.Msg {
		var err error
		if remove {
			err = c.UnsetReaction(ctx, chatID, messageID, emoji)
		} else {
			err = c.SetReaction(ctx, chatID, messageID, emoji)
		}
		return reactedMsg{chatID: chatID, err: err}
	}
}

// pasteImageCmd reads an image from the OS clipboard so it can be attached to
// the next outgoing message. Reading the clipboard may shell out to a helper
// (wl-paste/xclip/osascript/powershell), so it runs off the UI goroutine.
func pasteImageCmd() tea.Cmd {
	return func() tea.Msg {
		data, ct, err := clipboard.ReadImage()
		if err != nil {
			return imagePastedMsg{err: err}
		}
		return imagePastedMsg{data: data, contentType: ct}
	}
}

// textCopiedMsg reports the outcome of copying selected text to the OS
// clipboard so the UI can show a confirmation or error notice.
type textCopiedMsg struct {
	n   int   // number of runes copied (for the confirmation notice)
	err error // non-nil when the clipboard write failed
}

// copyTextCmd writes s to the OS clipboard. Like pasteImageCmd it shells out to
// a platform helper (wl-copy/xclip/pbcopy/clip), so it runs off the UI goroutine.
func copyTextCmd(s string) tea.Cmd {
	return func() tea.Msg {
		if err := clipboard.WriteText(s); err != nil {
			return textCopiedMsg{err: err}
		}
		return textCopiedMsg{n: len([]rune(s))}
	}
}

// sendImageCmd posts an inline image (with an optional text caption) to a chat.
func sendImageCmd(ctx context.Context, c *graph.Client, chatID string, img []byte, contentType, caption string) tea.Cmd {
	return func() tea.Msg {
		if _, err := c.SendImageMessage(ctx, chatID, img, contentType, caption); err != nil {
			return errMsg{err}
		}
		return sentMsg{chatID: chatID}
	}
}

// loadPeopleCmd fetches the user's contacts, optionally filtered by a search
// string, for the sidebar's contacts mode.
func loadPeopleCmd(ctx context.Context, c *graph.Client, search string) tea.Cmd {
	return func() tea.Msg {
		people, err := c.ListPeople(ctx, search)
		if err != nil {
			return peopleMsg{err: err}
		}
		return peopleMsg{people: people}
	}
}

// createChatCmd starts (or reuses) a 1:1 chat with the given user and reports
// the resulting chat so it can be opened.
func createChatCmd(ctx context.Context, c *graph.Client, myUserID, otherUserID string) tea.Cmd {
	return func() tea.Msg {
		chat, err := c.CreateOneOnOneChat(ctx, myUserID, otherUserID)
		if err != nil {
			return chatCreatedMsg{err: err}
		}
		return chatCreatedMsg{chat: chat}
	}
}

// openImageCmd opens an image referenced in a message. Inline/hosted images on
// graph.microsoft.com require an authenticated fetch, so we download the bytes
// and hand a temp file to the OS opener; plain http(s) URLs (e.g. public
// attachments) are opened directly in the browser. Either way the terminal
// can't render the image, so we delegate to the platform's default app.
func openImageCmd(ctx context.Context, c *graph.Client, img graph.ImageRef) tea.Cmd {
	return func() tea.Msg {
		name := img.Name
		if name == "" {
			name = "image"
		}
		// Authenticated hosted content (Graph) must be fetched with our bearer
		// token; opening the URL directly would 401 in a browser.
		if strings.Contains(img.URL, "graph.microsoft.com") {
			data, _, err := c.FetchHostedContent(ctx, img.URL)
			if err != nil {
				return imageOpenedMsg{name: name, err: err}
			}
			path, err := open.SaveTempImage(data, name)
			if err != nil {
				return imageOpenedMsg{name: name, err: err}
			}
			if err := open.Open(path); err != nil {
				return imageOpenedMsg{name: name, err: err}
			}
			return imageOpenedMsg{name: name}
		}
		// Public URL: let the browser/default app fetch and display it.
		if err := open.Open(img.URL); err != nil {
			return imageOpenedMsg{name: name, err: err}
		}
		return imageOpenedMsg{name: name}
	}
}

// editMessageCmd edits an existing chat message in place (PATCH).
func editMessageCmd(ctx context.Context, c *graph.Client, chatID, messageID, text string) tea.Cmd {
	return func() tea.Msg {
		if _, err := c.EditMessage(ctx, chatID, messageID, text); err != nil {
			return editedMsg{chatID: chatID, err: err}
		}
		return editedMsg{chatID: chatID}
	}
}

// loadMeetingsCmd fetches upcoming events within the lookahead window.
func loadMeetingsCmd(ctx context.Context, c *graph.Client, lookahead time.Duration) tea.Cmd {
	return func() tea.Msg {
		events, err := c.UpcomingEvents(ctx, lookahead)
		if err != nil {
			// Calendar access may be unconsented; don't spam the UI.
			return meetingsMsg{events: nil}
		}
		return meetingsMsg{events: events}
	}
}

// loadPresencesCmd fetches presence for the given user IDs.
func loadPresencesCmd(ctx context.Context, c *graph.Client, userIDs []string) tea.Cmd {
	return func() tea.Msg {
		if len(userIDs) == 0 {
			return presencesMsg{presences: nil}
		}
		presences, err := c.GetPresences(ctx, userIDs)
		if err != nil {
			// Presence may be unconsented; degrade gracefully.
			return presencesMsg{presences: nil}
		}
		return presencesMsg{presences: presences}
	}
}

// loadMyPresenceCmd fetches the signed-in user's own presence.
func loadMyPresenceCmd(ctx context.Context, c *graph.Client) tea.Cmd {
	return func() tea.Msg {
		p, err := c.MyPresence(ctx)
		if err != nil {
			return myPresenceMsg{presence: nil}
		}
		return myPresenceMsg{presence: p}
	}
}

// keepSessionCmd refreshes the app presence session so the user's chosen
// status persists while the TUI runs (heartbeat). When opt is set, it also
// applies that availability/activity to the session.
func keepSessionCmd(ctx context.Context, c *graph.Client, userID, sessionID string, opt *graph.PresenceOption, expiry time.Duration) tea.Cmd {
	return func() tea.Msg {
		availability, activity := "Available", "Available"
		if opt != nil {
			availability, activity = opt.Availability, opt.Activity
		}
		// Best-effort: ignore errors so a transient failure doesn't disrupt the
		// UI; the next heartbeat will retry.
		_ = c.SetPresence(ctx, userID, sessionID, availability, activity, expiry)
		return sessionRefreshedMsg{}
	}
}

// setStatusCmd sets the user's preferred presence (persists) and immediately
// applies it to the app session so it takes effect now.
func setStatusCmd(ctx context.Context, c *graph.Client, userID, sessionID string, opt graph.PresenceOption) tea.Cmd {
	return func() tea.Msg {
		if err := c.SetUserPreferredPresence(ctx, userID, opt.Availability, opt.Activity, 0); err != nil {
			return errMsg{err}
		}
		_ = c.SetPresence(ctx, userID, sessionID, opt.Availability, opt.Activity, sessionExpiry)
		return presenceSetMsg{}
	}
}

// clearSessionCmd ends the app presence session (used on exit).
func clearSessionCmd(ctx context.Context, c *graph.Client, userID, sessionID string) tea.Cmd {
	return func() tea.Msg {
		_ = c.ClearPresence(ctx, userID, sessionID)
		return nil
	}
}

// markChatReadCmd marks a chat as read on the server (best-effort, fire and
// forget). The unread highlight is already cleared locally when the chat opens,
// so a failure here only means the read state won't sync to other devices until
// the next read; we therefore swallow the error.
func markChatReadCmd(ctx context.Context, c *graph.Client, chatID, userID string) tea.Cmd {
	return func() tea.Msg {
		_ = c.MarkChatRead(ctx, chatID, userID)
		return nil
	}
}

// pollTickCmd schedules the next chat/message refresh.
func pollTickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return pollTickMsg(t) })
}

// meetingTickCmd schedules the next meeting check.
func meetingTickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return meetingTickMsg(t) })
}

// presenceTickCmd schedules the next presence refresh.
func presenceTickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return presenceTickMsg(t) })
}

// sessionTickCmd schedules the next presence-session heartbeat.
func sessionTickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return sessionTickMsg(t) })
}

// spellDebounceCmd waits a short idle period before a spell check runs, tagging
// the resulting message with gen so a burst of keystrokes collapses into a
// single check of the final text.
func spellDebounceCmd(d time.Duration, gen int) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return spellDebounceMsg{gen: gen} })
}

// spellCheckCmd runs the (subprocess) spell check off the Update loop and
// returns the misspellings tagged with gen. A nil/unavailable checker yields an
// empty result, so the caller need not special-case it.
func spellCheckCmd(c *spell.Checker, text string, gen int) tea.Cmd {
	return func() tea.Msg {
		return spellCheckedMsg{gen: gen, words: c.CheckText(text)}
	}
}
