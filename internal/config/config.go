// Package config loads runtime configuration for teams-tui from a JSON file
// and/or environment variables. It supports both fully Entra-hosted tenants and
// hybrid Entra/AD federated setups (where authentication redirects to a
// company-hosted web login during the device-code browser step).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DefaultGraphBaseURL is the Microsoft Graph v1.0 endpoint.
	DefaultGraphBaseURL = "https://graph.microsoft.com/v1.0"
	// DefaultAuthHost is the public Entra login host.
	DefaultAuthHost = "https://login.microsoftonline.com"
)

// Config holds everything needed to authenticate and talk to Graph.
type Config struct {
	// ClientID is the Application (client) ID of the registered Entra app.
	ClientID string `json:"client_id"`
	// TenantID is the directory (tenant) ID, a GUID, or "organizations"/
	// "common". For hybrid/federated tenants use the specific tenant GUID so
	// the device-code browser flow redirects to your company login.
	TenantID string `json:"tenant_id"`
	// AuthHost overrides the Entra login host (e.g. a sovereign cloud or a
	// custom federation host). Defaults to login.microsoftonline.com.
	AuthHost string `json:"auth_host,omitempty"`
	// GraphBaseURL overrides the Microsoft Graph base URL.
	GraphBaseURL string `json:"graph_base_url,omitempty"`
	// Scopes requested during authentication. Sensible defaults are applied.
	Scopes []string `json:"scopes,omitempty"`
	// PollInterval in seconds for refreshing chats/messages.
	PollIntervalSeconds int `json:"poll_interval_seconds,omitempty"`
	// MeetingLookaheadMinutes controls how far ahead to warn about meetings.
	MeetingLookaheadMinutes int `json:"meeting_lookahead_minutes,omitempty"`
	// DisableDesktopNotify turns off OS-level desktop notifications.
	DisableDesktopNotify bool `json:"disable_desktop_notify,omitempty"`
	// CodeBlockStyle is the chroma syntax-highlighting theme used for code
	// blocks (e.g. "monokai", "dracula", "github-dark"). Defaults to
	// DefaultCodeBlockStyle. An unknown name falls back to that default.
	CodeBlockStyle string `json:"code_block_style,omitempty"`
}

// DefaultCodeBlockStyle is the chroma theme used to colour code blocks when the
// config doesn't specify one.
const DefaultCodeBlockStyle = "monokai"

// DefaultScopes are the delegated permissions the TUI needs. offline_access is
// required to obtain a refresh token for the keyring cache.
var DefaultScopes = []string{
	"offline_access",
	"openid",
	"profile",
	"User.Read",
	"Chat.ReadWrite",
	// People.Read powers the contacts/people picker (GET /me/people).
	"People.Read",
	"Calendars.Read",
	"Presence.Read.All",
	"Presence.ReadWrite",
}

// Load reads configuration, applying this precedence (highest first):
// environment variables, the config file, then built-in defaults.
func Load() (*Config, error) {
	cfg := &Config{}

	if path := ConfigFilePath(); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			if err := json.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("parsing config file %s: %w", path, err)
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading config file %s: %w", path, err)
		}
	}

	// Environment variable overrides.
	if v := os.Getenv("TEAMS_TUI_CLIENT_ID"); v != "" {
		cfg.ClientID = v
	}
	if v := os.Getenv("TEAMS_TUI_TENANT_ID"); v != "" {
		cfg.TenantID = v
	}
	if v := os.Getenv("TEAMS_TUI_AUTH_HOST"); v != "" {
		cfg.AuthHost = v
	}
	if v := os.Getenv("TEAMS_TUI_GRAPH_BASE_URL"); v != "" {
		cfg.GraphBaseURL = v
	}
	if v := os.Getenv("TEAMS_TUI_SCOPES"); v != "" {
		cfg.Scopes = strings.Fields(v)
	}
	if v := os.Getenv("TEAMS_TUI_CODE_BLOCK_STYLE"); v != "" {
		cfg.CodeBlockStyle = v
	}

	cfg.applyDefaults()

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.AuthHost == "" {
		c.AuthHost = DefaultAuthHost
	}
	c.AuthHost = strings.TrimRight(c.AuthHost, "/")
	if c.GraphBaseURL == "" {
		c.GraphBaseURL = DefaultGraphBaseURL
	}
	c.GraphBaseURL = strings.TrimRight(c.GraphBaseURL, "/")
	if c.TenantID == "" {
		c.TenantID = "organizations"
	}
	if len(c.Scopes) == 0 {
		c.Scopes = append([]string(nil), DefaultScopes...)
	}
	if c.PollIntervalSeconds <= 0 {
		c.PollIntervalSeconds = 10
	}
	if c.MeetingLookaheadMinutes <= 0 {
		c.MeetingLookaheadMinutes = 5
	}
	if c.CodeBlockStyle == "" {
		c.CodeBlockStyle = DefaultCodeBlockStyle
	}
}

func (c *Config) validate() error {
	if c.ClientID == "" {
		return fmt.Errorf("client_id is required: set it in %s or via TEAMS_TUI_CLIENT_ID. "+
			"See the README for how to register an Entra application", ConfigFilePath())
	}
	return nil
}

// DeviceCodeURL returns the Entra device authorization endpoint.
func (c *Config) DeviceCodeURL() string {
	return fmt.Sprintf("%s/%s/oauth2/v2.0/devicecode", c.AuthHost, c.TenantID)
}

// TokenURL returns the Entra token endpoint.
func (c *Config) TokenURL() string {
	return fmt.Sprintf("%s/%s/oauth2/v2.0/token", c.AuthHost, c.TenantID)
}

// ScopeString returns scopes joined by spaces for OAuth requests.
func (c *Config) ScopeString() string {
	return strings.Join(c.Scopes, " ")
}

// ConfigDir returns the directory used for teams-tui configuration.
func ConfigDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "teams-tui")
}

// ConfigFilePath returns the path to the JSON config file.
func ConfigFilePath() string {
	if v := os.Getenv("TEAMS_TUI_CONFIG"); v != "" {
		return v
	}
	return filepath.Join(ConfigDir(), "config.json")
}
