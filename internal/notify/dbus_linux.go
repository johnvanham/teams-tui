//go:build linux

package notify

import (
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	notifyDest      = "org.freedesktop.Notifications"
	notifyPath      = "/org/freedesktop/Notifications"
	notifyIface     = "org.freedesktop.Notifications"
	defaultActionID = "default"
)

// dbusBackend sends clickable notifications over the freedesktop Notifications
// D-Bus interface and dispatches click callbacks. It listens on a single signal
// channel for two signals:
//
//   - ActivationToken(id, token): GNOME (and other Wayland daemons) emit this
//     just before ActionInvoked, carrying the XDG activation token needed to let
//     the target window come forward. We stash the latest token per id.
//   - ActionInvoked(id, action_key): the click itself. We look up the stored
//     callback for id and invoke it with the captured token (if any).
type dbusBackend struct {
	conn *dbus.Conn
	obj  dbus.BusObject

	mu       sync.Mutex
	handlers map[uint32]func(string) // notification id -> click callback
	tokens   map[uint32]string       // notification id -> activation token

	signals chan *dbus.Signal
	done    chan struct{}
}

// newActionBackend opens a session-bus connection and subscribes to the
// Notifications signals. It returns nil (so callers fall back to plain
// notifications) when no session bus is available or the daemon lacks the
// "actions" capability.
func newActionBackend() actionBackend {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil
	}
	if !serverSupportsActions(conn) {
		return nil
	}
	b := &dbusBackend{
		conn:     conn,
		obj:      conn.Object(notifyDest, notifyPath),
		handlers: make(map[uint32]func(string)),
		tokens:   make(map[uint32]string),
		signals:  make(chan *dbus.Signal, 16),
		done:     make(chan struct{}),
	}
	// Match only Notifications signals from the daemon to limit traffic.
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface(notifyIface),
		dbus.WithMatchObjectPath(notifyPath),
	); err != nil {
		return nil
	}
	conn.Signal(b.signals)
	go b.loop()
	return b
}

// serverSupportsActions reports whether the notification daemon advertises the
// "actions" capability; without it a clickable notification would show but do
// nothing, so we prefer the plain fallback.
func serverSupportsActions(conn *dbus.Conn) bool {
	obj := conn.Object(notifyDest, notifyPath)
	var caps []string
	if err := obj.Call(notifyIface+".GetCapabilities", 0).Store(&caps); err != nil {
		return false
	}
	for _, c := range caps {
		if c == "actions" {
			return true
		}
	}
	return false
}

func (b *dbusBackend) notifyWithAction(title, message string, onClick func(string)) error {
	// actions is a flat [key, label, ...] array; a "default" action fires on a
	// plain click (no visible button) per the freedesktop spec.
	actions := []string{defaultActionID, "Open"}
	hints := map[string]dbus.Variant{}
	var id uint32
	call := b.obj.Call(notifyIface+".Notify", 0,
		appName,      // app_name
		uint32(0),    // replaces_id
		"",           // app_icon
		title,        // summary
		message,      // body
		actions,      // actions
		hints,        // hints
		int32(-1),    // expire_timeout: daemon default
	)
	if call.Err != nil {
		return call.Err
	}
	if err := call.Store(&id); err != nil {
		return err
	}
	b.mu.Lock()
	b.handlers[id] = onClick
	b.mu.Unlock()
	return nil
}

// loop dispatches incoming Notifications signals until close(). It runs on its
// own goroutine; callbacks are invoked here, so they may block other click
// handling but never the UI.
func (b *dbusBackend) loop() {
	for {
		select {
		case <-b.done:
			return
		case sig, ok := <-b.signals:
			if !ok {
				return
			}
			b.handleSignal(sig)
		}
	}
}

func (b *dbusBackend) handleSignal(sig *dbus.Signal) {
	switch sig.Name {
	case notifyIface + ".ActivationToken":
		// Body: (UINT32 id, STRING activation_token)
		if len(sig.Body) < 2 {
			return
		}
		id, ok := sig.Body[0].(uint32)
		if !ok {
			return
		}
		token, _ := sig.Body[1].(string)
		b.mu.Lock()
		if _, tracked := b.handlers[id]; tracked {
			b.tokens[id] = token
		}
		b.mu.Unlock()

	case notifyIface + ".ActionInvoked":
		// Body: (UINT32 id, STRING action_key)
		if len(sig.Body) < 2 {
			return
		}
		id, ok := sig.Body[0].(uint32)
		if !ok {
			return
		}
		key, _ := sig.Body[1].(string)
		if key != defaultActionID {
			return
		}
		b.mu.Lock()
		handler := b.handlers[id]
		token := b.tokens[id]
		delete(b.handlers, id)
		delete(b.tokens, id)
		b.mu.Unlock()
		if handler != nil {
			handler(token)
		}

	case notifyIface + ".NotificationClosed":
		// Drop any handler for a dismissed/expired notification.
		if len(sig.Body) < 1 {
			return
		}
		if id, ok := sig.Body[0].(uint32); ok {
			b.mu.Lock()
			delete(b.handlers, id)
			delete(b.tokens, id)
			b.mu.Unlock()
		}
	}
}

func (b *dbusBackend) close() {
	select {
	case <-b.done:
		return // already closed
	default:
		close(b.done)
	}
	b.conn.RemoveSignal(b.signals)
	// Do not Close() the shared session bus: beeep and others may use it.
}
