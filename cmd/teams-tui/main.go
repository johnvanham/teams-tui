// Command teams-tui is an intuitive terminal UI for Microsoft Teams chats,
// built with Bubble Tea v2. It authenticates via the OAuth device-code flow,
// which works for both fully Entra-hosted tenants and hybrid Entra/AD
// federated setups (the browser sign-in step transparently follows your
// company's federation redirects).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"github.com/jvh/teams-tui/internal/auth"
	"github.com/jvh/teams-tui/internal/config"
	"github.com/jvh/teams-tui/internal/notify"
	"github.com/jvh/teams-tui/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "teams-tui:", err)
		os.Exit(1)
	}
}

func run() error {
	logout := flag.Bool("logout", false,
		"clear the cached token from the OS keyring and exit (forces a fresh sign-in next launch)")
	flag.Parse()

	if *logout {
		if err := auth.NewStore().Clear(); err != nil {
			return fmt.Errorf("clearing cached token: %w", err)
		}
		fmt.Println("Signed out: cached token cleared. Next launch will prompt for sign-in.")
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	authenticator := auth.New(cfg)
	store := auth.NewStore()
	notifier := notify.New(!cfg.DisableDesktopNotify)
	// Release the D-Bus connection backing clickable notifications on exit.
	defer notifier.Close()

	model := ui.New(ctx, cfg, authenticator, store, notifier)

	p := tea.NewProgram(model, tea.WithContext(ctx))
	// Wire the notification-click sender so a clicked notification can inject a
	// chat-switch message into the running program from its D-Bus goroutine.
	model.SetProgram(p)
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}
