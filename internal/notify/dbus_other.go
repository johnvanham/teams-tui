//go:build !linux

package notify

// newActionBackend has no clickable-notification implementation off Linux, so
// it returns nil and callers fall back to plain notifications.
func newActionBackend() actionBackend { return nil }
