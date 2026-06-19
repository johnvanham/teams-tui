package graph

import (
	"html"
	"strings"
)

// ComposeHTML converts a user's plain-text compose input into the Teams HTML
// body format so it renders correctly for every participant (and in the native
// Teams client). Without this, Graph stores the message as contentType "text",
// where newlines collapse and Markdown code fences show up literally instead of
// as a styled code block.
//
// It understands a small Markdown-ish subset, matching what we render on the
// receive side (see code.go):
//
//   - Fenced code blocks delimited by lines of ``` (optionally ```lang) become
//     <pre><code>…</code></pre>, preserving every interior line verbatim.
//   - Inline `code` spans become <code>…</code>.
//   - Every other line is HTML-escaped and wrapped in a <p>; blank lines become
//     empty paragraphs so vertical spacing survives the round-trip.
//
// All text is HTML-escaped, so a message that happens to contain literal HTML
// is shown as typed rather than interpreted.
func ComposeHTML(text string) string {
	// Normalize line endings so fence detection and <br> emission are stable.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	lines := strings.Split(text, "\n")
	var b strings.Builder

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Fenced code block: collect everything up to the closing fence (or the
		// end of input for an unterminated block) and emit it verbatim.
		if strings.HasPrefix(line, codeFence) {
			lang := strings.TrimSpace(strings.TrimPrefix(line, codeFence))
			var code []string
			i++
			for ; i < len(lines); i++ {
				if strings.HasPrefix(lines[i], codeFence) {
					break
				}
				code = append(code, lines[i])
			}
			b.WriteString(composeCodeBlock(code, lang))
			continue
		}

		// Quote: collect consecutive ">"-prefixed lines into one <blockquote>
		// so a multi-line quote renders as a single quoted region for every
		// participant (matching how Teams stores quoted replies).
		if isQuoteLine(line) {
			var quote []string
			for ; i < len(lines) && isQuoteLine(lines[i]); i++ {
				quote = append(quote, stripQuotePrefix(lines[i]))
			}
			i-- // the outer loop's i++ will advance past the last quote line
			b.WriteString(composeBlockquote(quote))
			continue
		}

		b.WriteString(composeParagraph(line))
	}

	return b.String()
}

// composeCodeBlock renders a fenced block as <pre><code>…</code></pre>. The body
// is HTML-escaped and joined with newlines (Teams preserves them inside <pre>).
// A language hint is carried on both data-language and a class so it survives
// regardless of which the consumer reads.
func composeCodeBlock(code []string, lang string) string {
	for i, c := range code {
		code[i] = html.EscapeString(c)
	}
	body := strings.Join(code, "\n")

	var attrs string
	if lang != "" {
		l := html.EscapeString(lang)
		attrs = ` data-language="` + l + `" class="language-` + l + `"`
	}
	return "<pre" + attrs + "><code>" + body + "</code></pre>"
}

// isQuoteLine reports whether a compose line is a Markdown-style quote line: a
// ">" optionally followed by a space, then the quoted text. A bare ">" (empty
// quoted line) also counts.
func isQuoteLine(line string) bool {
	t := strings.TrimLeft(line, " ")
	return strings.HasPrefix(t, ">")
}

// stripQuotePrefix removes the leading ">" (and a single following space) from a
// quote line, returning the quoted text.
func stripQuotePrefix(line string) string {
	t := strings.TrimLeft(line, " ")
	t = strings.TrimPrefix(t, ">")
	return strings.TrimPrefix(t, " ")
}

// composeBlockquote renders quoted lines as a single <blockquote>, with each
// line as its own <p> (HTML-escaped, inline `code` styled) so line breaks in
// the quote survive. Empty quoted lines become empty paragraphs.
func composeBlockquote(lines []string) string {
	var b strings.Builder
	b.WriteString("<blockquote>")
	for _, line := range lines {
		b.WriteString(composeParagraph(line))
	}
	b.WriteString("</blockquote>")
	return b.String()
}

// composeParagraph renders a single prose line as a <p>, escaping it and styling
// inline `code` spans. A blank line becomes an empty paragraph so blank lines
// the user typed are preserved as vertical spacing.
func composeParagraph(line string) string {
	if strings.TrimSpace(line) == "" {
		return "<p></p>"
	}
	return "<p>" + inlineCodeToHTML(line) + "</p>"
}

// inlineCodeToHTML HTML-escapes a prose line and wraps any `backtick` spans in
// <code>. Escaping is done per-segment so the code text is escaped too, but the
// <code> tags themselves are emitted literally. An unmatched trailing backtick
// is treated as plain text.
func inlineCodeToHTML(line string) string {
	var b strings.Builder
	for {
		start := strings.IndexByte(line, '`')
		if start < 0 {
			b.WriteString(html.EscapeString(line))
			break
		}
		end := strings.IndexByte(line[start+1:], '`')
		if end < 0 {
			// No closing backtick: the rest is plain text.
			b.WriteString(html.EscapeString(line))
			break
		}
		end += start + 1
		b.WriteString(html.EscapeString(line[:start]))
		b.WriteString("<code>")
		b.WriteString(html.EscapeString(line[start+1 : end]))
		b.WriteString("</code>")
		line = line[end+1:]
	}
	return b.String()
}
