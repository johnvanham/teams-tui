package ui

import (
	"strings"
	"testing"
)

func TestHighlightCode(t *testing.T) {
	t.Run("known language returns aligned styled lines", func(t *testing.T) {
		code := []string{"package main", "", "func main() {}"}
		out := highlightCode(code, "go")
		if out == nil {
			t.Fatal("highlightCode returned nil for go")
		}
		if len(out) != len(code) {
			t.Fatalf("line count = %d, want %d", len(out), len(code))
		}
		// At least one line should carry ANSI styling escapes.
		joined := strings.Join(out, "\n")
		if !strings.Contains(joined, "\x1b[") {
			t.Error("expected ANSI escapes in highlighted output, got none")
		}
		// The visible text must be preserved (ignoring escapes).
		for i := range code {
			if got := stripANSI(out[i]); got != code[i] {
				t.Errorf("line %d visible text = %q, want %q", i, got, code[i])
			}
		}
	})

	t.Run("unknown language falls back to nil", func(t *testing.T) {
		// A nonsense language with content that won't confidently analyse.
		if out := highlightCode([]string{"xyzzy"}, "not-a-language"); out != nil {
			// Analyse may still guess; tolerate a guess as long as it's aligned.
			if len(out) != 1 {
				t.Fatalf("fallback returned %d lines, want 1", len(out))
			}
		}
	})

	t.Run("empty input returns nil", func(t *testing.T) {
		if out := highlightCode(nil, "go"); out != nil {
			t.Errorf("expected nil for empty input, got %v", out)
		}
	})
}

// stripANSI removes CSI escape sequences for visible-text comparison.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && (s[i] < '@' || s[i] > '~') {
				i++
			}
			if i < len(s) {
				i++ // final byte
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
