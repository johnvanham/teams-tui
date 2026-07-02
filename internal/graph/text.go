package graph

import (
	"html"
	"regexp"
	"strings"
)

var (
	tagRe        = regexp.MustCompile(`(?s)<[^>]*>`)
	multiSpaceRe = regexp.MustCompile(`[ \t]{2,}`)
	// blockquoteRe matches a <blockquote>…</blockquote> element. Native Teams
	// replies now arrive as messageReference attachments (see quote.go), but
	// older messages — and any client that inlines a quote — still use
	// <blockquote>, so this receive path is kept. Captured group 1 is the inner
	// HTML, which quoteToLines converts to "> "-prefixed lines.
	blockquoteRe = regexp.MustCompile(`(?is)<blockquote\b[^>]*>(.*?)</blockquote\s*>`)
)

// PlainText converts a message body to a readable plain-text string. Graph chat
// messages are frequently HTML; this strips tags, decodes entities, and
// normalizes whitespace while preserving the paragraph structure (a single
// blank line between paragraphs, matching the native Teams client).
func (b MessageBody) PlainText() string {
	content := b.Content
	var codeBlocks map[string]string
	if strings.EqualFold(b.ContentType, "html") {
		// Pull code blocks and inline code out first, leaving placeholders in
		// their place. They are restored verbatim after whitespace
		// normalization so their indentation and blank lines are preserved.
		content, codeBlocks = extractCodeBlocks(content)

		// Explicit line breaks.
		brReplacer := strings.NewReplacer(
			"<br>", "\n", "<br/>", "\n", "<br />", "\n",
			"<BR>", "\n", "<BR/>", "\n", "<BR />", "\n",
		)
		content = brReplacer.Replace(content)

		// Bullets for list items (open tag introduces the line).
		content = strings.ReplaceAll(content, "<li>", "\n- ")
		content = strings.ReplaceAll(content, "<LI>", "\n- ")

		// Resolve Teams <emoji> elements to their Unicode characters before the
		// generic tag strip below, which would otherwise discard them.
		content = replaceEmojiTags(content)

		// Convert quoted replies (<blockquote>) to "> "-prefixed lines so the
		// quote structure survives the tag strip and the UI can style it. Done
		// before the block-element newline replacements below so the quote's own
		// paragraph breaks are handled by quoteToLines.
		content = blockquoteRe.ReplaceAllStringFunc(content, func(m string) string {
			sub := blockquoteRe.FindStringSubmatch(m)
			if sub == nil {
				return m
			}
			return "\n" + quoteToLines(sub[1]) + "\n"
		})

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

	content = normalizeWhitespace(content)
	return restoreCodeBlocks(content, codeBlocks)
}

// quoteToLines converts the inner HTML of a <blockquote> into plain-text lines,
// each prefixed with "> " so the renderer can style the quote (and so it stays
// readable in any client). Block boundaries (<br>, </p>, </div>) become line
// breaks; remaining tags are stripped and entities decoded. Blank lines inside
// the quote are dropped to keep it compact.
func quoteToLines(inner string) string {
	s := strings.NewReplacer(
		"<br>", "\n", "<br/>", "\n", "<br />", "\n",
		"<BR>", "\n", "<BR/>", "\n", "<BR />", "\n",
		"</p>", "\n", "</P>", "\n", "</div>", "\n", "</DIV>", "\n",
	).Replace(inner)
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")

	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = multiSpaceRe.ReplaceAllString(line, " ")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, "> "+line)
	}
	return strings.Join(out, "\n")
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
