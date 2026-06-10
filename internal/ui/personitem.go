package ui

import "github.com/jvh/teams-tui/internal/graph"

// personItem adapts a graph.Person to the bubbles list.Item / list.DefaultItem
// interfaces so contacts can be shown in the sidebar's "Contacts" mode and
// picked to start a new 1:1 chat.
type personItem struct {
	person graph.Person
}

// Title implements list.DefaultItem.
func (i personItem) Title() string {
	if i.person.DisplayName != "" {
		return "[+] " + i.person.DisplayName
	}
	return "[+] " + i.person.UserPrincipalName
}

// Description implements list.DefaultItem: show the best available address so
// the user can disambiguate people with the same display name.
func (i personItem) Description() string {
	if email := i.person.Email(); email != "" {
		return email
	}
	return i.person.UserPrincipalName
}

// FilterValue implements list.Item.
func (i personItem) FilterValue() string {
	return i.person.DisplayName + " " + i.person.UserPrincipalName + " " + i.person.Email()
}
