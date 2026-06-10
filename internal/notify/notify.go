// Package notify sends OS-level desktop notifications, used for meeting-start
// alerts and (optionally) new message pings. It degrades gracefully: failures
// are swallowed so a headless or notification-less environment never breaks the
// TUI.
package notify

import "github.com/gen2brain/beeep"

// Notifier fires desktop notifications when enabled.
type Notifier struct {
	enabled bool
}

// New returns a Notifier. When enabled is false, all calls are no-ops.
func New(enabled bool) *Notifier {
	return &Notifier{enabled: enabled}
}

// Notify shows a desktop notification with the given title and message.
func (n *Notifier) Notify(title, message string) {
	if n == nil || !n.enabled {
		return
	}
	_ = beeep.Notify(title, message, "")
}

// Alert shows a notification accompanied by the system alert sound, used for
// time-sensitive meeting reminders.
func (n *Notifier) Alert(title, message string) {
	if n == nil || !n.enabled {
		return
	}
	_ = beeep.Alert(title, message, "")
}
