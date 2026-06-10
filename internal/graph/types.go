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
	ID           string      `json:"id"`
	CreatedAt    time.Time   `json:"createdDateTime"`
	LastModified time.Time   `json:"lastModifiedDateTime"`
	MessageType  string      `json:"messageType"`
	Importance   string      `json:"importance"`
	Body         MessageBody `json:"body"`
	From         *From       `json:"from"`
	DeletedAt    *time.Time  `json:"deletedDateTime"`
	Reactions    []Reaction  `json:"reactions"`
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
