package focus

import (
	"os"
	"testing"
)

func TestExpandFocusCommand(t *testing.T) {
	tests := []struct {
		name     string
		template string
		token    string
		want     string
	}{
		{
			name:     "substitutes single token",
			template: "raise --token {token}",
			token:    "abc123",
			want:     "raise --token abc123",
		},
		{
			name:     "substitutes every occurrence",
			template: "{token} and {token}",
			token:    "X",
			want:     "X and X",
		},
		{
			name:     "no placeholder leaves template unchanged",
			template: "wmctrl -a teams-tui",
			token:    "ignored",
			want:     "wmctrl -a teams-tui",
		},
		{
			name:     "empty token yields empty substitution",
			template: "cmd {token}",
			token:    "",
			want:     "cmd ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExpandFocusCommand(tt.template, tt.token); got != tt.want {
				t.Errorf("ExpandFocusCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRaiseTerminalEmptyIsNoop(t *testing.T) {
	if err := RaiseTerminal("", "tok"); err != nil {
		t.Errorf("empty template should be a no-op, got %v", err)
	}
	if err := RaiseTerminal("   ", "tok"); err != nil {
		t.Errorf("blank template should be a no-op, got %v", err)
	}
}

func TestRaiseTerminalExportsToken(t *testing.T) {
	// A command that echoes the env var into a temp file proves the token is
	// exported to the child even when the template has no {token} placeholder.
	f, err := os.CreateTemp(t.TempDir(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := RaiseTerminal("printf %s \"$XDG_ACTIVATION_TOKEN\" > "+f.Name(), "wayland-tok"); err != nil {
		t.Fatalf("RaiseTerminal error = %v", err)
	}
	got, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "wayland-tok" {
		t.Errorf("child saw XDG_ACTIVATION_TOKEN=%q, want %q", got, "wayland-tok")
	}
}

func TestSelectTmuxPaneNoopWhenNotInTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	if err := SelectTmuxPane("%3"); err != nil {
		t.Errorf("SelectTmuxPane outside tmux should no-op, got %v", err)
	}
	// Empty pane is always a no-op regardless of TMUX.
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1,0")
	if err := SelectTmuxPane(""); err != nil {
		t.Errorf("SelectTmuxPane with empty pane should no-op, got %v", err)
	}
}
