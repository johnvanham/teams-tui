package graph

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/jvh/teams-tui/internal/auth"
)

// makeJWT builds a minimal unsigned JWT whose payload carries the given claims
// JSON. Only the payload segment is meaningful to tenantID; the header and
// signature are placeholders.
func makeJWT(payloadJSON string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(payloadJSON))
	return header + "." + payload + "."
}

func clientWithToken(accessToken string) *Client {
	tok := &auth.Token{
		AccessToken: accessToken,
		Expiry:      time.Now().Add(time.Hour),
	}
	ts := auth.NewTokenSource(nil, nil, tok)
	return NewClient("https://graph.example", ts)
}

func TestTenantID(t *testing.T) {
	t.Run("extracts tid claim", func(t *testing.T) {
		c := clientWithToken(makeJWT(`{"tid":"tenant-guid","oid":"user-guid"}`))
		got, err := c.tenantID(context.Background())
		if err != nil {
			t.Fatalf("tenantID() error = %v", err)
		}
		if got != "tenant-guid" {
			t.Errorf("tenantID() = %q, want %q", got, "tenant-guid")
		}
	})

	t.Run("missing tid claim -> error", func(t *testing.T) {
		c := clientWithToken(makeJWT(`{"oid":"user-guid"}`))
		if _, err := c.tenantID(context.Background()); err == nil {
			t.Error("expected error for token without tid claim")
		}
	})

	t.Run("non-JWT token -> error", func(t *testing.T) {
		c := clientWithToken("not-a-jwt")
		if _, err := c.tenantID(context.Background()); err == nil {
			t.Error("expected error for non-JWT token")
		}
	})
}
