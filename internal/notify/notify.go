// Package notify sends OS-level desktop notifications, used for meeting-start
// alerts and (optionally) new message pings. It degrades gracefully: failures
// are swallowed so a headless or notification-less environment never breaks the
// TUI.
package notify

import "github.com/gen2brain/beeep"

// appName is the application name shown by the desktop notification daemon
// (e.g. as the title/source on GNOME). beeep defaults this to "DefaultAppName";
// we override it so notifications are attributed to this app instead.
const appName = "Teams Chat"

func init() {
	// beeep.AppName is a package-level setting consumed on each Notify/Alert
	// call (the D-Bus app_name on Linux, the toast AppID on Windows, the
	// notification group on macOS). Set it once at load time.
	beeep.AppName = appName
}

// Notifier fires desktop notifications when enabled. The notify/alert backends
// are function fields so tests can capture calls without touching the OS.
type Notifier struct {
	enabled bool
	notify  func(title, message string) error
	alert   func(title, message string) error
}

// New returns a Notifier. When enabled is false, all calls are no-ops.
func New(enabled bool) *Notifier {
	return &Notifier{
		enabled: enabled,
		notify:  func(t, m string) error { return beeep.Notify(t, m, "") },
		alert:   func(t, m string) error { return beeep.Alert(t, m, "") },
	}
}

// Notify shows a desktop notification with the given title and message.
func (n *Notifier) Notify(title, message string) {
	if n == nil || !n.enabled || n.notify == nil {
		return
	}
	_ = n.notify(title, message)
}

// Alert shows a notification accompanied by the system alert sound, used for
// time-sensitive meeting reminders.
func (n *Notifier) Alert(title, message string) {
	if n == nil || !n.enabled || n.alert == nil {
		return
	}
	_ = n.alert(title, message)
}

// Capture is a recorded notification, used by NewCapturing in tests.
type Capture struct {
	Title   string
	Message string
}

// NewCapturing returns an enabled Notifier that records every Notify/Alert call
// into the returned slice pointer instead of sending an OS notification. It lets
// callers assert on notification behaviour in unit tests.
func NewCapturing() (*Notifier, *[]Capture) {
	var got []Capture
	rec := func(t, m string) error {
		got = append(got, Capture{Title: t, Message: m})
		return nil
	}
	return &Notifier{enabled: true, notify: rec, alert: rec}, &got
}
