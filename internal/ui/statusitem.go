package ui

import "github.com/jvh/teams-tui/internal/graph"

// statusItem adapts a graph.PresenceOption to the bubbles list interfaces for
// the status-picker popup.
type statusItem struct {
	opt graph.PresenceOption
}

func (i statusItem) Title() string       { return i.opt.Name }
func (i statusItem) Description() string { return "" }
func (i statusItem) FilterValue() string { return i.opt.Name }
