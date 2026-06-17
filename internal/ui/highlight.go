package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// highlightCode applies syntax highlighting to a code block, returning one
// styled string per input line. Colours come from the given chroma style (its
// own palette, e.g. monokai), with each token rendered over that style's
// background so the panel reads as one solid theme. When the language is unknown
// or lexing fails, it returns nil so the caller falls back to plain text.
func highlightCode(lines []string, lang string, style *chroma.Style) []string {
	if len(lines) == 0 || style == nil {
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

	bg := styleBackground(style)

	// Rebuild the source as styled text, then split on newlines. Chroma
	// preserves the exact characters (including newlines inside token values),
	// so splitting the styled stream yields the same line count as the input.
	var b strings.Builder
	for _, tok := range it.Tokens() {
		value := tok.Value
		if value == "" {
			continue
		}
		ls := tokenStyle(style, tok.Type, bg)
		// Apply styling per line segment so a multi-line token (block
		// comments/strings) keeps the background on every line and the newline
		// itself stays unstyled (so the split below works cleanly).
		parts := strings.Split(value, "\n")
		for i, part := range parts {
			if part != "" {
				b.WriteString(ls.Render(part))
			}
			if i < len(parts)-1 {
				b.WriteByte('\n')
			}
		}
	}

	out := strings.Split(b.String(), "\n")
	// Defensive: if the token stream changed the line count, bail to the plain
	// fallback so callers' indexing stays valid.
	if len(out) != len(lines) {
		return nil
	}
	return out
}

// styleBackground returns the chroma style's background colour as a lipgloss
// colour, or nil when the style declares none (terminal default is used).
func styleBackground(style *chroma.Style) color.Color {
	bg := style.Get(chroma.Background).Background
	if !bg.IsSet() {
		return nil
	}
	return lipgloss.Color(bg.String())
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

// tokenStyle builds a lipgloss style for a token from the chroma style entry:
// foreground colour plus bold/italic/underline, over the panel background.
func tokenStyle(style *chroma.Style, t chroma.TokenType, bg color.Color) lipgloss.Style {
	ls := lipgloss.NewStyle()
	if bg != nil {
		ls = ls.Background(bg)
	}
	entry := style.Get(t)
	if entry.Colour.IsSet() {
		ls = ls.Foreground(lipgloss.Color(entry.Colour.String()))
	}
	if entry.Bold == chroma.Yes {
		ls = ls.Bold(true)
	}
	if entry.Italic == chroma.Yes {
		ls = ls.Italic(true)
	}
	if entry.Underline == chroma.Yes {
		ls = ls.Underline(true)
	}
	return ls
}

// codeBlockBackground returns the chroma style's background as a lipgloss colour,
// falling back to the app's CodeBlockBg when the style declares none. Used so the
// panel padding/labels share the theme's background.
func codeBlockBackground(style *chroma.Style) color.Color {
	if style != nil {
		if bg := styleBackground(style); bg != nil {
			return bg
		}
	}
	return nil
}
