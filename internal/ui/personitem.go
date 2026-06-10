package ui

import "github.com/jvh/teams-tui/internal/graph"

// personItem adapts a graph.Person to the bubbles list.Item / list.DefaultItem
// interfaces so contacts can be shown in the sidebar's "Contacts" mode and
// picked to start a new 1:1 chat. presence is a snapshot of the contact's Teams
// availability (zero value until presence is fetched); it is baked into the
// item at build time because the list delegate renders from the static
// Title()/Description() strings rather than the live model.
type personItem struct {
	person   graph.Person
	presence graph.Presence
}

// Title implements list.DefaultItem. A presence glyph prefixes the name so the
// contact's availability is visible at a glance (the glyph is colored by the
// delegate's normal text style; the textual label lives in Description()).
func (i personItem) Title() string {
	name := i.person.DisplayName
	if name == "" {
		name = i.person.UserPrincipalName
	}
	return i.presence.Glyph() + " " + name
}

// Description implements list.DefaultItem: show the presence label plus the
// best available address so the user can both see status and disambiguate
// people with the same display name.
func (i personItem) Description() string {
	addr := i.person.Email()
	if addr == "" {
		addr = i.person.UserPrincipalName
	}
	if label := i.presence.Label(); label != "" {
		if addr != "" {
			return label + " · " + addr
		}
		return label
	}
	return addr
}

// FilterValue implements list.Item.
func (i personItem) FilterValue() string {
	return i.person.DisplayName + " " + i.person.UserPrincipalName + " " + i.person.Email()
}
