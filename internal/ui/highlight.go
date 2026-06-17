package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/jvh/teams-tui/internal/ui/styles"
)

// highlightCode applies syntax highlighting to a code block, returning one
// styled string per input line. Every token carries the code-block background
// so the panel reads as one solid colour; the foreground is chosen per token
// type from the palette in styles. When the language is unknown or lexing
// fails, it returns nil so the caller falls back to plain (unstyled) text.
func highlightCode(lines []string, lang string) []string {
	if len(lines) == 0 {
		return nil
	}
	lexer := pickLexer(lang, strings.Join(lines, "\n"))
	if lexer == nil {
		return nil
	}

	source := strings.Join(lines, "\n")
	it, err := lexer.Tokenise(nil, source)
	if err != nil {
		return nil
	}

	// Rebuild the source as styled text, then split on newlines. Chroma
	// preserves the exact characters (including newlines inside token values),
	// so splitting the styled stream yields the same line count as the input.
	var b strings.Builder
	for _, tok := range it.Tokens() {
		value := tok.Value
		if value == "" {
			continue
		}
		style := tokenStyle(tok.Type)
		// Apply styling per line segment so a multi-line token (rare: block
		// comments/strings) keeps the background on every line and the newline
		// itself stays unstyled (so split works cleanly).
		parts := strings.Split(value, "\n")
		for i, part := range parts {
			if part != "" {
				b.WriteString(style.Render(part))
			}
			if i < len(parts)-1 {
				b.WriteByte('\n')
			}
		}
	}

	out := strings.Split(b.String(), "\n")
	// Defensive: if the token stream lost/added a trailing newline, reconcile to
	// the original line count so callers' indexing stays valid.
	if len(out) != len(lines) {
		return nil
	}
	return out
}

// pickLexer resolves a chroma lexer from the fenced language hint, falling back
// to content analysis when the hint is missing or unrecognized. Returns nil
// when no lexer can be determined (caller renders plain text).
func pickLexer(lang, source string) chroma.Lexer {
	if lang != "" {
		if l := lexers.Get(lang); l != nil {
			return l
		}
	}
	if l := lexers.Analyse(source); l != nil {
		return l
	}
	return nil
}

// tokenStyle maps a chroma token type to a lipgloss style over the code-block
// background, using the existing palette. Unmapped tokens fall back to the
// default code foreground.
func tokenStyle(t chroma.TokenType) lipgloss.Style {
	base := lipgloss.NewStyle().Background(styles.CodeBlockBg)
	switch {
	case t.InCategory(chroma.Comment):
		return base.Foreground(styles.Grey).Italic(true)
	case t.InCategory(chroma.Keyword):
		return base.Foreground(styles.Purple).Bold(true)
	case t.InCategory(chroma.LiteralString):
		return base.Foreground(styles.Green)
	case t.InCategory(chroma.LiteralNumber):
		return base.Foreground(styles.Orange)
	case t.InCategory(chroma.Name):
		switch t {
		case chroma.NameFunction, chroma.NameClass, chroma.NameNamespace:
			return base.Foreground(styles.PurpleLt).Bold(true)
		case chroma.NameBuiltin, chroma.NameBuiltinPseudo:
			return base.Foreground(styles.Yellow)
		case chroma.NameTag:
			return base.Foreground(styles.Purple)
		case chroma.NameAttribute:
			return base.Foreground(styles.PurpleLt)
		}
		return base.Foreground(styles.CodeFg)
	case t.InCategory(chroma.Operator):
		return base.Foreground(styles.Yellow)
	default:
		return base.Foreground(styles.CodeFg)
	}
}
