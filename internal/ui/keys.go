package ui

import "charm.land/bubbles/v2/key"

// keyMap defines all global key bindings. Component-specific navigation (list,
// viewport, textarea) is handled by the bubbles components themselves.
type keyMap struct {
	Send     key.Binding
	NextPane key.Binding
	PrevPane key.Binding
	Refresh  key.Binding
	Newline  key.Binding
	Status   key.Binding
	Contacts key.Binding
	Edit     key.Binding
	Image    key.Binding
	Paste    key.Binding
	Help     key.Binding
	Quit     key.Binding
	Up       key.Binding
	Down     key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Send: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "send"),
		),
		Newline: key.NewBinding(
			key.WithKeys("alt+enter", "ctrl+j"),
			key.WithHelp("alt+enter", "newline"),
		),
		NextPane: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next pane"),
		),
		PrevPane: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev pane"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("ctrl+r", "refresh"),
		),
		Status: key.NewBinding(
			key.WithKeys("ctrl+s"),
			key.WithHelp("ctrl+s", "status"),
		),
		Contacts: key.NewBinding(
			key.WithKeys("ctrl+o"),
			key.WithHelp("ctrl+o", "contacts/new chat"),
		),
		Edit: key.NewBinding(
			key.WithKeys("ctrl+e"),
			key.WithHelp("ctrl+e", "edit last message"),
		),
		Image: key.NewBinding(
			key.WithKeys("ctrl+y"),
			key.WithHelp("ctrl+y", "view image"),
		),
		Paste: key.NewBinding(
			key.WithKeys("ctrl+v"),
			key.WithHelp("ctrl+v", "paste image"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Help: key.NewBinding(
			key.WithKeys("ctrl+g", "f1", "?"),
			key.WithHelp("ctrl+g", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
	}
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.NextPane, k.Send, k.Contacts, k.Status, k.Help, k.Quit}
}

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.NextPane, k.PrevPane},
		{k.Send, k.Newline, k.Edit, k.Image, k.Paste},
		{k.Contacts, k.Status, k.Refresh},
		{k.Help, k.Quit},
	}
}
