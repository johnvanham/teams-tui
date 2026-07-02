package graph

import (
	"strconv"
	"strings"
	"time"
)

// User is a minimal Graph user/profile.
type User struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	UserPrincipalName string `json:"userPrincipalName"`
	Mail              string `json:"mail"`
}

// Person is a contact surfaced by the /me/people endpoint, used to power the
// "start new chat" people picker. ScoredEmailAddresses is Graph's ranked list
// of the person's emails; the highest-scored one comes first.
type Person struct {
	ID                   string `json:"id"`
	DisplayName          string `json:"displayName"`
	UserPrincipalName    string `json:"userPrincipalName"`
	ScoredEmailAddresses []struct {
		Address string `json:"address"`
	} `json:"scoredEmailAddresses"`
}

// Email returns the person's first (highest-scored) email address, or empty
// when none are present, so callers can show a contact's address without
// indexing into the raw slice.
func (p Person) Email() string {
	if len(p.ScoredEmailAddresses) > 0 {
		return p.ScoredEmailAddresses[0].Address
	}
	return ""
}

// Presence holds a user's Teams availability.
type Presence struct {
	ID           string `json:"id"`
	Availability string `json:"availability"`
	Activity     string `json:"activity"`
}

// PresenceOption is a settable presence choice with the availability/activity
// pair Graph expects for setUserPreferredPresence / setPresence.
type PresenceOption struct {
	Name         string // user-facing label
	Availability string
	Activity     string
}

// SettablePresences are the statuses a user can choose in the picker. The
// availability/activity pairs follow the combinations documented for
// setUserPreferredPresence.
var SettablePresences = []PresenceOption{
	{Name: "Available", Availability: "Available", Activity: "Available"},
	{Name: "Busy", Availability: "Busy", Activity: "Busy"},
	{Name: "Do not disturb", Availability: "DoNotDisturb", Activity: "DoNotDisturb"},
	{Name: "Be right back", Availability: "BeRightBack", Activity: "BeRightBack"},
	{Name: "Away", Availability: "Away", Activity: "Away"},
	{Name: "Appear offline", Availability: "Offline", Activity: "OffWork"},
}

// Label returns a short human-readable status label derived from availability.
func (p Presence) Label() string {
	switch p.Availability {
	case "Available":
		return "Available"
	case "AvailableIdle":
		return "Available"
	case "Away", "BeRightBack":
		return "Away"
	case "Busy", "BusyIdle":
		return "Busy"
	case "InACall", "InAConferenceCall":
		return "In a call"
	case "InAMeeting":
		return "In a meeting"
	case "Presenting":
		return "Presenting"
	case "DoNotDisturb", "Focusing":
		return "Do not disturb"
	case "Offline":
		return "Offline"
	case "OutOfOffice":
		return "Out of office"
	case "", "PresenceUnknown":
		return ""
	default:
		return p.Availability
	}
}

// Glyph returns a single status indicator character for the presence.
func (p Presence) Glyph() string {
	switch p.Availability {
	case "Available", "AvailableIdle":
		return "●" // green dot (colored at render time)
	case "Busy", "BusyIdle", "InACall", "InAConferenceCall", "InAMeeting", "Presenting":
		return "●" // red
	case "DoNotDisturb", "Focusing":
		return "⊘"
	case "Away", "BeRightBack", "OutOfOffice":
		return "◐" // amber
	case "Offline", "", "PresenceUnknown":
		return "○"
	default:
		return "○"
	}
}

// ChatType enumerates the Graph chatType values.
type ChatType string

const (
	ChatOneOnOne ChatType = "oneOnOne"
	ChatGroup    ChatType = "group"
	ChatMeeting  ChatType = "meeting"
)

// Chat is a Teams chat (1:1, group, or meeting).
type Chat struct {
	ID                  string               `json:"id"`
	ChatType            ChatType             `json:"chatType"`
	Topic               string               `json:"topic"`
	LastUpdatedDateTime time.Time            `json:"lastUpdatedDateTime"`
	Members             []ConversationMember `json:"members"`
	LastMessagePreview  *MessagePreview      `json:"lastMessagePreview"`
	Viewpoint           *ChatViewpoint       `json:"viewpoint"`
}

// ChatViewpoint carries the signed-in user's per-chat state, notably the read
// horizon used to compute unread chats. Graph returns it by default on
// /me/chats. LastMessageReadDateTime is the createdDateTime of the newest
// message the user has read in this chat.
type ChatViewpoint struct {
	IsHidden                bool      `json:"isHidden"`
	LastMessageReadDateTime time.Time `json:"lastMessageReadDateTime"`
}

// Unread reports whether the chat has messages the signed-in user hasn't read
// yet. A chat is unread when its last message is newer than the read horizon
// and was not sent by the user themselves (your own sends never count as
// unread). readHorizon overrides the server's viewpoint when non-zero, letting
// the caller reflect a just-opened chat as read before the server catches up.
func (c *Chat) Unread(selfID string, readHorizon time.Time) bool {
	if c.LastMessagePreview == nil {
		return false
	}
	last := c.LastMessagePreview.CreatedAt
	if last.IsZero() {
		return false
	}
	// Don't flag our own latest message as unread.
	if from := c.LastMessagePreview.From; from != nil && from.User != nil &&
		selfID != "" && from.User.ID == selfID {
		return false
	}
	// Use the later of the server's read marker and any local override.
	read := readHorizon
	if c.Viewpoint != nil && c.Viewpoint.LastMessageReadDateTime.After(read) {
		read = c.Viewpoint.LastMessageReadDateTime
	}
	return last.After(read)
}

// LastActivity returns the most recent activity time for ordering the chat
// list: the last message preview time when available, otherwise the chat's
// last-updated time.
func (c *Chat) LastActivity() time.Time {
	if c.LastMessagePreview != nil && !c.LastMessagePreview.CreatedAt.IsZero() {
		return c.LastMessagePreview.CreatedAt
	}
	return c.LastUpdatedDateTime
}

// ConversationMember is a participant in a chat.
type ConversationMember struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	UserID      string `json:"userId"`
	Email       string `json:"email"`
}

// MessagePreview is the lastMessagePreview relationship on a chat.
type MessagePreview struct {
	ID        string      `json:"id"`
	CreatedAt time.Time   `json:"createdDateTime"`
	Body      MessageBody `json:"body"`
	From      *From       `json:"from"`
}

// Message is a Teams chat message.
type Message struct {
	ID           string       `json:"id"`
	CreatedAt    time.Time    `json:"createdDateTime"`
	LastModified time.Time    `json:"lastModifiedDateTime"`
	MessageType  string       `json:"messageType"`
	Importance   string       `json:"importance"`
	Body         MessageBody  `json:"body"`
	From         *From        `json:"from"`
	DeletedAt    *time.Time   `json:"deletedDateTime"`
	Reactions    []Reaction   `json:"reactions"`
	Attachments  []Attachment `json:"attachments"`
}

// Attachment is a file or rich-content attachment on a chat message. Graph
// surfaces shared files and (some) images here; inline images embedded in the
// body HTML are exposed separately via Message.Images. ContentURL points at the
// downloadable content when present.
type Attachment struct {
	ID          string `json:"id"`
	ContentType string `json:"contentType"`
	ContentURL  string `json:"contentUrl"`
	Name        string `json:"name"`
	// Content carries the attachment's inline payload. For a reply/quote
	// (contentType "messageReference") it is a JSON string describing the
	// referenced message (its id, a preview of the quoted text, and the
	// sender); see Message.Quotes.
	Content string `json:"content"`
}

// Reaction is a single reaction on a chat message.
type Reaction struct {
	ReactionType string    `json:"reactionType"`
	DisplayName  string    `json:"displayName"`
	CreatedAt    time.Time `json:"createdDateTime"`
	User         *From     `json:"user"`
}

// Emoji maps a reaction's type to a renderable glyph. reactionType is usually a
// Unicode emoji already, but legacy/backward-compatible keyword types are
// translated here.
func (r Reaction) Emoji() string {
	switch strings.ToLower(r.ReactionType) {
	case "like":
		return "👍"
	case "heart":
		return "❤️"
	case "laugh":
		return "😆"
	case "surprised":
		return "😮"
	case "sad":
		return "😢"
	case "angry":
		return "😡"
	case "custom":
		return "🔸"
	case "":
		return "·"
	default:
		// Already a Unicode emoji character (e.g. "👍", "🎉").
		return r.ReactionType
	}
}

// MessageBody is the content of a message.
type MessageBody struct {
	ContentType string `json:"contentType"` // "text" or "html"
	Content     string `json:"content"`
}

// From identifies the sender of a message.
type From struct {
	User        *User `json:"user"`
	Application *struct {
		DisplayName string `json:"displayName"`
	} `json:"application"`
}

// SenderName returns a best-effort display name for the message sender.
func (m *Message) SenderName() string {
	if m.From != nil && m.From.User != nil && m.From.User.DisplayName != "" {
		return m.From.User.DisplayName
	}
	if m.From != nil && m.From.Application != nil && m.From.Application.DisplayName != "" {
		return m.From.Application.DisplayName
	}
	return "System"
}

// ReactionSummary groups the message's reactions by glyph and returns ordered
// "<emoji> <count>" pairs (e.g. "👍 3"), preserving first-seen order. Returns
// nil when there are no reactions.
func (m *Message) ReactionSummary() []string {
	if len(m.Reactions) == 0 {
		return nil
	}
	counts := make(map[string]int)
	var order []string
	for _, r := range m.Reactions {
		e := r.Emoji()
		if _, seen := counts[e]; !seen {
			order = append(order, e)
		}
		counts[e]++
	}
	out := make([]string, 0, len(order))
	for _, e := range order {
		out = append(out, e+" "+strconv.Itoa(counts[e]))
	}
	return out
}

// UserReacted reports whether the user with the given ID has already reacted to
// this message with the given emoji glyph. It compares against each reaction's
// rendered Emoji() so a stored keyword type (e.g. "like") and its Unicode glyph
// ("👍") are treated as the same reaction. Used to decide whether re-reacting
// should toggle the reaction off. An empty userID never matches.
func (m *Message) UserReacted(userID, emoji string) bool {
	if userID == "" {
		return false
	}
	for _, r := range m.Reactions {
		if r.User == nil || r.User.User == nil || r.User.User.ID != userID {
			continue
		}
		if r.Emoji() == emoji {
			return true
		}
	}
	return false
}

// DisplayName produces a human-friendly label for a chat. For 1:1 and group
// chats without a topic it falls back to the member names (excluding self).
func (c *Chat) DisplayName(selfID string) string {
	if c.Topic != "" {
		return c.Topic
	}
	var names []string
	for _, m := range c.Members {
		if m.UserID != "" && m.UserID == selfID {
			continue
		}
		if m.DisplayName != "" {
			names = append(names, m.DisplayName)
		}
	}
	if len(names) > 0 {
		return strings.Join(names, ", ")
	}
	switch c.ChatType {
	case ChatGroup:
		return "Group chat"
	case ChatMeeting:
		return "Meeting chat"
	default:
		return "Chat"
	}
}

// Event is a calendar event used for meeting notifications.
type Event struct {
	ID               string     `json:"id"`
	Subject          string     `json:"subject"`
	IsCancelled      bool       `json:"isCancelled"`
	IsOnlineMeeting  bool       `json:"isOnlineMeeting"`
	OnlineMeetingURL string     `json:"onlineMeetingUrl"`
	Start            DateTimeTZ `json:"start"`
	End              DateTimeTZ `json:"end"`
	OnlineMeeting    *struct {
		JoinURL string `json:"joinUrl"`
	} `json:"onlineMeeting"`
}

// DateTimeTZ mirrors Graph's dateTimeTimeZone structure.
type DateTimeTZ struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

// Time parses the event time. Graph returns these in the requested timezone;
// when requested with a Prefer: outlook.timezone="UTC" header they are UTC.
func (d DateTimeTZ) Time() (time.Time, error) {
	// Graph omits the trailing Z; treat as UTC since we request UTC.
	layouts := []string{
		"2006-01-02T15:04:05.0000000",
		"2006-01-02T15:04:05",
		time.RFC3339,
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, d.DateTime); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, &time.ParseError{Value: d.DateTime}
}
