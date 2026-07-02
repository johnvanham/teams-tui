package notify

import "testing"

func TestNotifyWithActionCaptured(t *testing.T) {
	n, got := NewCapturing()

	var clicked string
	n.NotifyWithAction("Alice", "hi there", func(token string) {
		clicked = token
	})

	if len(*got) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(*got))
	}
	c := (*got)[0]
	if c.Title != "Alice" || c.Message != "hi there" {
		t.Errorf("capture = %+v, want title/message Alice/hi there", c)
	}
	if c.Action == nil {
		t.Fatal("capture.Action is nil; NotifyWithAction should record the click callback")
	}

	// Simulate the notification being clicked with an activation token.
	c.Action("xdg-token-42")
	if clicked != "xdg-token-42" {
		t.Errorf("onClick got token %q, want %q", clicked, "xdg-token-42")
	}
}

func TestDisabledNotifierIgnoresAction(t *testing.T) {
	n := New(false) // disabled: no OS calls, no action backend
	called := false
	n.NotifyWithAction("t", "m", func(string) { called = true })
	if called {
		t.Error("disabled notifier must not invoke the click callback")
	}
	// Close must be safe even with no backend.
	n.Close()
}

func TestNilNotifierSafe(t *testing.T) {
	var n *Notifier
	n.Notify("t", "m")
	n.NotifyWithAction("t", "m", func(string) {})
	n.Alert("t", "m")
	n.Close()
}
