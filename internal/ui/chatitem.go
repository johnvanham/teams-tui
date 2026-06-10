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
}

func newChatItem(c graph.Chat, selfID string) chatItem {
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
	return chatItem{chat: c, selfID: selfID, preview: preview}
}

// Title implements list.DefaultItem.
func (i chatItem) Title() string {
	prefix := ""
	switch i.chat.ChatType {
	case graph.ChatGroup:
		prefix = "[#] "
	case graph.ChatMeeting:
		prefix = "[@] "
	default:
		prefix = "[>] "
	}
	return prefix + i.chat.DisplayName(i.selfID)
}

// Description implements list.DefaultItem.
func (i chatItem) Description() string { return i.preview }

// FilterValue implements list.Item.
func (i chatItem) FilterValue() string {
	return i.chat.DisplayName(i.selfID) + " " + i.preview
}
