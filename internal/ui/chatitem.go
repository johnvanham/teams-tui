package ui

import (
	"strings"

	"github.com/jvh/teams-tui/internal/graph"
)

// chatItem adapts a graph.Chat to the bubbles list.Item / list.DefaultItem
// interfaces.
type chatItem struct {
	chat    graph.Chat
	selfID  string
	preview string
	unread  bool
}

func newChatItem(c graph.Chat, selfID string, unread bool) chatItem {
	preview := ""
	if c.LastMessagePreview != nil {
		preview = c.LastMessagePreview.Body.PlainText()
		preview = strings.ReplaceAll(preview, "\n", " ")
		if from := c.LastMessagePreview.From; from != nil && from.User != nil && from.User.DisplayName != "" {
			preview = from.User.DisplayName + ": " + preview
		}
	}
	if preview == "" {
		preview = "No messages yet"
	}
	return chatItem{chat: c, selfID: selfID, preview: preview, unread: unread}
}

// chatTypeGlyph returns the marker a chat row leads with, identifying the kind
// of chat. The symbol legend under the expanded help (legend.go) reads these
// from here so the two can't drift apart.
func chatTypeGlyph(t graph.ChatType) string {
	switch t {
	case graph.ChatGroup:
		return "[#]"
	case graph.ChatMeeting:
		return "[@]"
	default:
		return "[>]"
	}
}

// Title implements list.DefaultItem. It leads with a type glyph followed by the
// chat's display name. Unread chats are highlighted by the chat delegate via a
// row background (see chatdelegate.go), not by any title marker, so titles stay
// aligned whether or not a chat is unread.
func (i chatItem) Title() string {
	return chatTypeGlyph(i.chat.ChatType) + " " + i.chat.DisplayName(i.selfID)
}

// Description implements list.DefaultItem.
func (i chatItem) Description() string { return i.preview }

// FilterValue implements list.Item.
func (i chatItem) FilterValue() string {
	return i.chat.DisplayName(i.selfID) + " " + i.preview
}
