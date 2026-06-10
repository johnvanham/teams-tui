// Package auth implements the OAuth 2.0 device authorization grant against the
// Microsoft identity platform (Entra). It is intentionally MSAL-free: the
// device-code flow works identically for fully Entra-hosted tenants and for
// hybrid Entra/AD federated tenants, because the user completes sign-in in a
// real browser where federation redirects (e.g. to a company-hosted ADFS or web
// login) happen transparently.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jvh/teams-tui/internal/config"
)

// Token holds an access token plus the data required to refresh it.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	Scope        string    `json:"scope"`
	Expiry       time.Time `json:"expiry"`
}

// Valid reports whether the access token is present and not within the 60s
// expiry safety window.
func (t *Token) Valid() bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	return time.Now().Add(60 * time.Second).Before(t.Expiry)
}

// CoversScopes reports whether the token's granted scopes include every
// required scope. Comparison is case-insensitive and ignores the OpenID Connect
// scopes (openid/profile/offline_access), which Entra grants implicitly and may
// omit from the returned scope string. A token that predates a newly-added
// permission will fail this check, signalling that a fresh sign-in is required.
func (t *Token) CoversScopes(required []string) bool {
	if t == nil {
		return false
	}
	granted := make(map[string]bool)
	for _, s := range strings.Fields(strings.ToLower(t.Scope)) {
		// Graph returns scopes without the resource prefix; normalize.
		if i := strings.LastIndex(s, "/"); i >= 0 {
			s = s[i+1:]
		}
		granted[s] = true
	}
	for _, r := range required {
		rl := strings.ToLower(r)
		switch rl {
		case "openid", "profile", "offline_access", "email":
			continue // implicit / not always echoed back
		}
		if i := strings.LastIndex(rl, "/"); i >= 0 {
			rl = rl[i+1:]
		}
		if !granted[rl] {
			return false
		}
	}
	return true
}

// DeviceCode is the response from the device authorization endpoint.
type DeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	Message         string `json:"message"`
}

// Authenticator manages token acquisition and refresh.
type Authenticator struct {
	cfg    *config.Config
	client *http.Client
}

// New returns an Authenticator for the given configuration.
func New(cfg *config.Config) *Authenticator {
	return &Authenticator{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

type tokenResponse struct {
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// RequestDeviceCode begins the device authorization flow and returns the code
// and verification URI to present to the user.
func (a *Authenticator) RequestDeviceCode(ctx context.Context) (*DeviceCode, error) {
	form := url.Values{}
	form.Set("client_id", a.cfg.ClientID)
	form.Set("scope", a.cfg.ScopeString())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.cfg.DeviceCodeURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device code request: %w", err)
	}
	defer resp.Body.Close()

	var dc DeviceCode
	if err := json.NewDecoder(resp.Body).Decode(&dc); err != nil {
		return nil, fmt.Errorf("decoding device code response: %w", err)
	}
	if resp.StatusCode != http.StatusOK || dc.DeviceCode == "" {
		return nil, fmt.Errorf("device code request failed (status %d)", resp.StatusCode)
	}
	if dc.Interval <= 0 {
		dc.Interval = 5
	}
	return &dc, nil
}

// errAuthorizationPending is returned while the user has not yet completed
// sign-in. Callers should keep polling.
var errAuthorizationPending = fmt.Errorf("authorization_pending")

// PollToken polls the token endpoint until the user completes authentication,
// the code expires, or the context is cancelled.
func (a *Authenticator) PollToken(ctx context.Context, dc *DeviceCode) (*Token, error) {
	interval := time.Duration(dc.Interval) * time.Second
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("device code expired before sign-in completed")
		}

		tok, err := a.pollOnce(ctx, dc.DeviceCode)
		if err == errAuthorizationPending {
			continue
		}
		if err != nil {
			return nil, err
		}
		return tok, nil
	}
}

func (a *Authenticator) pollOnce(ctx context.Context, deviceCode string) (*Token, error) {
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("client_id", a.cfg.ClientID)
	form.Set("device_code", deviceCode)

	tr, status, err := a.tokenRequest(ctx, form)
	if err != nil {
		return nil, err
	}
	if tr.Error != "" {
		switch tr.Error {
		case "authorization_pending", "slow_down":
			return nil, errAuthorizationPending
		case "authorization_declined":
			return nil, fmt.Errorf("authorization declined by user")
		case "expired_token":
			return nil, fmt.Errorf("device code expired")
		default:
			return nil, fmt.Errorf("token error %q: %s", tr.Error, tr.ErrorDesc)
		}
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("token request failed (status %d)", status)
	}
	return tokenFromResponse(tr), nil
}

// Refresh exchanges a refresh token for a fresh access token.
func (a *Authenticator) Refresh(ctx context.Context, refreshToken string) (*Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", a.cfg.ClientID)
	form.Set("refresh_token", refreshToken)
	form.Set("scope", a.cfg.ScopeString())

	tr, status, err := a.tokenRequest(ctx, form)
	if err != nil {
		return nil, err
	}
	if tr.Error != "" {
		return nil, fmt.Errorf("refresh error %q: %s", tr.Error, tr.ErrorDesc)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("refresh request failed (status %d)", status)
	}
	tok := tokenFromResponse(tr)
	// Entra may not always return a new refresh token; keep the old one.
	if tok.RefreshToken == "" {
		tok.RefreshToken = refreshToken
	}
	return tok, nil
}

func (a *Authenticator) tokenRequest(ctx context.Context, form url.Values) (*tokenResponse, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.cfg.TokenURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("decoding token response: %w", err)
	}
	return &tr, resp.StatusCode, nil
}

func tokenFromResponse(tr *tokenResponse) *Token {
	return &Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		TokenType:    tr.TokenType,
		Scope:        tr.Scope,
		Expiry:       time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}
}
