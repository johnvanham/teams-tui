// Package focus raises the terminal window and selects the tmux pane running
// teams-tui in response to a clicked desktop notification. Everything here is
// best-effort: on a headless box, outside tmux, or when the configured focus
// command is missing, the calls degrade to no-ops so notification clicks never
// break. The one piece that always matters — switching to the right chat — is
// handled by the UI itself; this package only brings the window/pane forward.
package focus

import (
	"os"
	"os/exec"
	"strings"
)

// tokenPlaceholder is the literal substring replaced with the XDG activation
// token inside a focus command template.
const tokenPlaceholder = "{token}"

// ExpandFocusCommand substitutes the activation token into a focus command
// template. It is separated from RaiseTerminal so the substitution is unit
// testable without executing anything.
func ExpandFocusCommand(template, activationToken string) string {
	return strings.ReplaceAll(template, tokenPlaceholder, activationToken)
}

// RaiseTerminal runs the configured focus command (via `sh -c`) to bring the
// terminal window forward. The activation token is both substituted into the
// template (`{token}`) and exported as XDG_ACTIVATION_TOKEN so a command can
// read it either way. An empty template is a no-op. Errors are returned for the
// caller to log/ignore; a failed raise must not abort the click handling.
func RaiseTerminal(template, activationToken string) error {
	if strings.TrimSpace(template) == "" {
		return nil
	}
	line := ExpandFocusCommand(template, activationToken)
	cmd := exec.Command("sh", "-c", line)
	// Expose the token to commands that prefer the env var over substitution.
	cmd.Env = append(os.Environ(), "XDG_ACTIVATION_TOKEN="+activationToken)
	return cmd.Run()
}

// SelectTmuxPane selects the tmux window and pane identified by pane (a
// $TMUX_PANE id like "%3"). It no-ops when pane is empty or we're not running
// inside tmux, so it's safe to call unconditionally. select-window brings the
// pane's window to the foreground of its session; select-pane focuses the pane
// within that window.
func SelectTmuxPane(pane string) error {
	if pane == "" || os.Getenv("TMUX") == "" {
		return nil
	}
	if err := exec.Command("tmux", "select-window", "-t", pane).Run(); err != nil {
		return err
	}
	return exec.Command("tmux", "select-pane", "-t", pane).Run()
}
