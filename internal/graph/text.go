package graph

import (
	"html"
	"regexp"
	"strings"
)

var (
	tagRe        = regexp.MustCompile(`(?s)<[^>]*>`)
	multiSpaceRe = regexp.MustCompile(`[ \t]{2,}`)
)

// PlainText converts a message body to a readable plain-text string. Graph chat
// messages are frequently HTML; this strips tags, decodes entities, and
// normalizes whitespace while preserving the paragraph structure (a single
// blank line between paragraphs, matching the native Teams client).
func (b MessageBody) PlainText() string {
	content := b.Content
	if strings.EqualFold(b.ContentType, "html") {
		// Explicit line breaks.
		brReplacer := strings.NewReplacer(
			"<br>", "\n", "<br/>", "\n", "<br />", "\n",
			"<BR>", "\n", "<BR/>", "\n", "<BR />", "\n",
		)
		content = brReplacer.Replace(content)

		// Bullets for list items (open tag introduces the line).
		content = strings.ReplaceAll(content, "<li>", "\n- ")
		content = strings.ReplaceAll(content, "<LI>", "\n- ")

		// End each block element with a single newline. A single <p>...</p> is
		// therefore one line; an explicit empty paragraph (<p>&nbsp;</p>)
		// becomes a whitespace-only line that normalizeWhitespace turns into a
		// single blank line — matching how the native client separates
		// paragraphs.
		content = strings.NewReplacer(
			"</p>", "\n", "</P>", "\n",
			"</div>", "\n", "</DIV>", "\n",
			"</li>", "", "</LI>", "",
		).Replace(content)

		// Drop all remaining tags and decode entities (turns &nbsp; into a
		// non-breaking space, &amp; into &, etc.).
		content = tagRe.ReplaceAllString(content, "")
		content = html.UnescapeString(content)
	}

	return normalizeWhitespace(content)
}

// normalizeWhitespace tidies the converted text: trims each line, replaces
// runs of spaces/tabs/non-breaking spaces with a single space, drops
// whitespace-only lines down to empty, and collapses consecutive blank lines
// into exactly one.
func normalizeWhitespace(s string) string {
	// Treat non-breaking spaces (U+00A0) as regular spaces so "empty" Teams
	// paragraphs (<p>&nbsp;</p>) become genuinely empty lines.
	s = strings.ReplaceAll(s, "\u00a0", " ")

	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blankRun := 0
	for _, line := range lines {
		line = multiSpaceRe.ReplaceAllString(line, " ")
		line = strings.TrimSpace(line)
		if line == "" {
			blankRun++
			// Allow at most one blank line between paragraphs.
			if blankRun > 1 {
				continue
			}
			// Skip leading blank lines.
			if len(out) == 0 {
				continue
			}
			out = append(out, "")
			continue
		}
		blankRun = 0
		out = append(out, line)
	}

	// Trim trailing blank lines.
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}
