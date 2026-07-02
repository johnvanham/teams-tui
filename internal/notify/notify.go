// Package notify sends OS-level desktop notifications, used for meeting-start
// alerts and (optionally) new message pings. It degrades gracefully: failures
// are swallowed so a headless or notification-less environment never breaks the
// TUI.
//
// Plain notifications go through beeep (cross-platform). Clickable
// notifications — where clicking focuses the terminal and opens the relevant
// chat — need per-notification action callbacks and the compositor's XDG
// activation token, which beeep does not expose. On Linux those are sent over
// D-Bus directly (see dbus_linux.go); everywhere else NotifyWithAction falls
// back to a plain beeep notification (still notifies, just isn't clickable).
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

// actionBackend sends a clickable notification whose click invokes onClick with
// the compositor's XDG activation token (empty when the daemon doesn't provide
// one). Implementations are platform-specific; a nil backend means "no
// clickable support here" and callers fall back to a plain notification. It
// returns an error only to signal the caller should use the fallback.
type actionBackend interface {
	notifyWithAction(title, message string, onClick func(activationToken string)) error
	close()
}

// Notifier fires desktop notifications when enabled. The notify/alert backends
// are function fields so tests can capture calls without touching the OS.
type Notifier struct {
	enabled bool
	notify  func(title, message string) error
	alert   func(title, message string) error
	// action, when non-nil, delivers clickable notifications with a click
	// callback. It is nil when the platform/environment can't support them, in
	// which case NotifyWithAction degrades to a plain notify.
	action actionBackend
}

// New returns a Notifier. When enabled is false, all calls are no-ops. On
// Linux it also attempts to open a D-Bus connection for clickable
// notifications; if that fails (no session bus, etc.) clickable notifications
// silently degrade to plain ones.
func New(enabled bool) *Notifier {
	n := &Notifier{
		enabled: enabled,
		notify:  func(t, m string) error { return beeep.Notify(t, m, "") },
		alert:   func(t, m string) error { return beeep.Alert(t, m, "") },
	}
	if enabled {
		n.action = newActionBackend()
	}
	return n
}

// Notify shows a desktop notification with the given title and message.
func (n *Notifier) Notify(title, message string) {
	if n == nil || !n.enabled || n.notify == nil {
		return
	}
	_ = n.notify(title, message)
}

// NotifyWithAction shows a notification that runs onClick when the user clicks
// it. onClick receives the XDG activation token (empty if unavailable), which a
// Wayland compositor requires to let the target window come forward. When no
// action backend is available (non-Linux, no D-Bus, or it errors), it falls
// back to a plain, non-clickable notification so the user is still alerted.
// onClick may run on a background goroutine and must be safe to call there.
func (n *Notifier) NotifyWithAction(title, message string, onClick func(activationToken string)) {
	if n == nil || !n.enabled {
		return
	}
	if n.action != nil {
		if err := n.action.notifyWithAction(title, message, onClick); err == nil {
			return
		}
		// Fall through to the plain notification on any failure.
	}
	n.Notify(title, message)
}

// Alert shows a notification accompanied by the system alert sound, used for
// time-sensitive meeting reminders.
func (n *Notifier) Alert(title, message string) {
	if n == nil || !n.enabled || n.alert == nil {
		return
	}
	_ = n.alert(title, message)
}

// Close releases any resources held for clickable notifications (the D-Bus
// connection + signal loop on Linux). Safe to call on a nil Notifier or when no
// action backend was created.
func (n *Notifier) Close() {
	if n == nil || n.action == nil {
		return
	}
	n.action.close()
}

// Capture is a recorded notification, used by NewCapturing in tests.
type Capture struct {
	Title   string
	Message string
	// Action holds the click callback for NotifyWithAction captures (nil for
	// plain Notify/Alert), letting tests simulate a click.
	Action func(activationToken string)
}

// NewCapturing returns an enabled Notifier that records every Notify/Alert/
// NotifyWithAction call into the returned slice pointer instead of sending an OS
// notification. It lets callers assert on notification behaviour — and simulate
// notification clicks — in unit tests.
func NewCapturing() (*Notifier, *[]Capture) {
	var got []Capture
	rec := func(t, m string) error {
		got = append(got, Capture{Title: t, Message: m})
		return nil
	}
	return &Notifier{
		enabled: true,
		notify:  rec,
		alert:   rec,
		action:  &captureBackend{record: &got},
	}, &got
}

// captureBackend records action notifications for NewCapturing.
type captureBackend struct{ record *[]Capture }

func (c *captureBackend) notifyWithAction(title, message string, onClick func(string)) error {
	*c.record = append(*c.record, Capture{Title: title, Message: message, Action: onClick})
	return nil
}

func (c *captureBackend) close() {}
