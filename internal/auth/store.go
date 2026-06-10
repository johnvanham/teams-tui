package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "teams-tui"
	keyringUser    = "default"
)

// ErrNoToken indicates no cached token exists in the keyring.
var ErrNoToken = errors.New("no cached token")

// Store persists tokens in the OS keyring (Keychain on macOS, Secret Service on
// Linux, Credential Manager on Windows).
type Store struct{}

// NewStore returns a keyring-backed token store.
func NewStore() *Store { return &Store{} }

// Save serializes the token to the OS keyring.
func (s *Store) Save(t *Token) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	if err := keyring.Set(keyringService, keyringUser, string(data)); err != nil {
		return fmt.Errorf("saving token to keyring: %w", err)
	}
	return nil
}

// Load retrieves the cached token from the OS keyring.
func (s *Store) Load() (*Token, error) {
	data, err := keyring.Get(keyringService, keyringUser)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, ErrNoToken
		}
		return nil, fmt.Errorf("loading token from keyring: %w", err)
	}
	var t Token
	if err := json.Unmarshal([]byte(data), &t); err != nil {
		return nil, fmt.Errorf("decoding cached token: %w", err)
	}
	return &t, nil
}

// Clear removes the cached token (used on logout or unrecoverable auth errors).
func (s *Store) Clear() error {
	err := keyring.Delete(keyringService, keyringUser)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

// TokenSource combines an Authenticator and a Store to always provide a valid
// access token, transparently refreshing and persisting as needed. It is safe
// for sequential use from the TUI's command goroutines.
type TokenSource struct {
	auth  *Authenticator
	store *Store
	tok   *Token
}

// NewTokenSource builds a TokenSource around an initial token.
func NewTokenSource(a *Authenticator, store *Store, initial *Token) *TokenSource {
	return &TokenSource{auth: a, store: store, tok: initial}
}

// Token returns a currently-valid access token, refreshing if necessary.
func (ts *TokenSource) Token(ctx context.Context) (string, error) {
	if ts.tok.Valid() {
		return ts.tok.AccessToken, nil
	}
	if ts.tok == nil || ts.tok.RefreshToken == "" {
		return "", ErrNoToken
	}
	refreshed, err := ts.auth.Refresh(ctx, ts.tok.RefreshToken)
	if err != nil {
		return "", err
	}
	ts.tok = refreshed
	if err := ts.store.Save(refreshed); err != nil {
		// Persistence failure is non-fatal for the current session.
		return refreshed.AccessToken, nil
	}
	return refreshed.AccessToken, nil
}
