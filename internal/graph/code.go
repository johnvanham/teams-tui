package graph

import (
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Microsoft Teams encodes code in chat message HTML two ways: inline snippets as
// <code>…</code> (or a <span class="…code…">), and multi-line code blocks as a
// <codeblock>…</codeblock> element wrapping a <code> (the canonical form the
// Teams service stores). Content from other clients or pastes can also arrive as
// <pre>…</pre>; both are treated as block code, optionally carrying a
// data-language / class attribute naming the language. The generic tag strip in
// PlainText would flatten both — collapsing the indentation and blank lines that
// make code readable — so we extract them first, convert them to a Markdown-ish
// form the renderer understands, and protect that form from the whitespace
// normalizer.
//
// The output uses two conventions the UI layer keys off of:
//   - A block becomes a fenced region delimited by lines of codeFence, with an
//     optional language on the opening fence (```go). The lines in between are
//     preserved verbatim.
//   - Inline code is wrapped in single backticks (`like this`).
var (
	// blockRes match a code-block element, each capturing its attribute list
	// (group 1) and inner HTML (group 2). Teams canonicalizes a sent code block
	// into a <codeblock>…</codeblock> element (wrapping a <code>), while pasted
	// or other-client content can arrive as <pre>…</pre>; both are block code.
	// Go's RE2 has no backreferences, so each tag gets its own pattern. (?is):
	// case-insensitive, dot-matches-newline. bareClass marks the <codeblock>
	// form, whose class attribute holds the bare language name (e.g. "Php").
	blockRes = []struct {
		re        *regexp.Regexp
		bareClass bool
	}{
		{regexp.MustCompile(`(?is)<codeblock\b([^>]*)>(.*?)</codeblock\s*>`), true},
		{regexp.MustCompile(`(?is)<pre\b([^>]*)>(.*?)</pre\s*>`), false},
	}
	// inlineCodeRe matches a standalone <code>…</code> element (used for inline
	// snippets). Block code is handled by preRe first, so by the time this runs
	// any remaining <code> is inline.
	inlineCodeRe = regexp.MustCompile(`(?is)<code\b[^>]*>(.*?)</code\s*>`)
	// langRe pulls a language hint from a <pre> (or its inner <code>) attribute
	// list: data-language="go", class="language-go", or class="lang-go".
	langAttrRe  = regexp.MustCompile(`(?is)\bdata-language\s*=\s*("([^"]*)"|'([^']*)')`)
	langClassRe = regexp.MustCompile(`(?is)\bclass\s*=\s*("([^"]*)"|'([^']*)')`)
	langTokenRe = regexp.MustCompile(`(?is)\b(?:language|lang|highlight-source)-([\w+#.-]+)`)
	// innerBrRe / innerTagRe clean the captured code body: <br> becomes a
	// newline, every other tag (e.g. the inner <code>, syntax-highlight <span>s)
	// is dropped without touching the text it wraps.
	innerBrRe  = regexp.MustCompile(`(?is)<br\s*/?>`)
	innerTagRe = regexp.MustCompile(`(?s)<[^>]*>`)
)

const (
	// codeFence is the marker line the UI uses to detect a code block. It is the
	// familiar Markdown triple backtick; renderConversation styles every line
	// between an opening and closing fence as code.
	codeFence = "```"
)

// extractCodeBlocks pulls block-level code (<codeblock>/<pre>) and inline <code>
// snippets out of HTML body content, returning the content with each replaced by a unique
// placeholder plus a map from placeholder to its already-converted (fenced or
// backtick-wrapped) text. The caller substitutes the placeholders back in after
// whitespace normalization so code formatting survives untouched.
func extractCodeBlocks(content string) (string, map[string]string) {
	blocks := make(map[string]string)
	n := 0

	// Block code first: a block element wraps a <code>, so handling it up front
	// keeps the inline pass below from matching the inner element.
	for _, b := range blockRes {
		re, bareClass := b.re, b.bareClass
		content = re.ReplaceAllStringFunc(content, func(m string) string {
			sub := re.FindStringSubmatch(m)
			if sub == nil {
				return m
			}
			attrs, inner := sub[1], sub[2]
			lang := detectLanguage(attrs, inner, bareClass)
			code := decodeCodeText(inner)
			fenced := codeFence + lang + "\n" + code + "\n" + codeFence
			key := codePlaceholder(n)
			n++
			blocks[key] = fenced
			// Surround with newlines so the block always starts on its own line
			// even if Teams inlined the element next to other text.
			return "\n" + key + "\n"
		})
	}

	// Inline code: wrap the decoded text in backticks. We keep it on its line so
	// normalizeWhitespace still trims surrounding prose normally.
	content = inlineCodeRe.ReplaceAllStringFunc(content, func(m string) string {
		sub := inlineCodeRe.FindStringSubmatch(m)
		if sub == nil {
			return m
		}
		code := strings.TrimRight(decodeCodeText(sub[1]), "\n")
		key := codePlaceholder(n)
		n++
		blocks[key] = "`" + code + "`"
		return key
	})

	return content, blocks
}

// restoreCodeBlocks substitutes each placeholder produced by extractCodeBlocks
// with its converted text, after whitespace normalization has run on everything
// else. Done last so the verbatim code is never reflowed.
func restoreCodeBlocks(content string, blocks map[string]string) string {
	for key, val := range blocks {
		content = strings.ReplaceAll(content, key, val)
	}
	return content
}

// codePlaceholder builds a sentinel unlikely to appear in real prose. The
// surrounding NUL bytes keep it a single, untrimmable token, and the numeric
// suffix makes each placeholder unique within a message.
func codePlaceholder(n int) string {
	return "\x00CODEBLOCK" + strconv.Itoa(n) + "\x00"
}

// decodeCodeText turns the inner HTML of a code element into plain text while
// preserving the line structure that matters for code: <br> becomes a newline,
// all other tags are stripped, and entities are decoded. Leading/trailing blank
// lines are trimmed but interior indentation and blank lines are kept verbatim.
func decodeCodeText(s string) string {
	s = innerBrRe.ReplaceAllString(s, "\n")
	// Block-level wrappers Teams sometimes uses for each code line.
	s = strings.NewReplacer("</div>", "\n", "</DIV>", "\n", "</p>", "\n", "</P>", "\n").Replace(s)
	s = innerTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	// Normalize CRLF and trim only the outer blank lines so the body keeps its
	// own indentation.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Trim(s, "\n")
}

// detectLanguage resolves a language hint for a block of code, returning the
// bare token (e.g. "go", "php") or "" if none is present. It looks first at the
// block element's own attributes (attrs), then scans the inner HTML for a
// prefixed class token. It understands:
//   - data-language="go"
//   - class="language-go" / "lang-go" / "highlight-source-go" (on the block or
//     an inner <code>)
//   - the Teams <codeblock> form, which puts the bare (often capitalized)
//     language directly in class, e.g. class="Php" — only honored when
//     bareClass is true, so syntax-highlight classes on inner <code>/<span>
//     elements (e.g. "hljs-keyword") are never mistaken for a language.
func detectLanguage(attrs, inner string, bareClass bool) string {
	if m := langAttrRe.FindStringSubmatch(attrs); m != nil {
		if v := strings.TrimSpace(attrValue(m)); v != "" {
			return strings.ToLower(v)
		}
	}
	if m := langClassRe.FindStringSubmatch(attrs); m != nil {
		if v := strings.TrimSpace(attrValue(m)); v != "" {
			if t := langTokenRe.FindStringSubmatch(v); t != nil {
				return strings.ToLower(t[1])
			}
			if bareClass && isLanguageToken(v) {
				return strings.ToLower(v)
			}
		}
	}
	// Inner HTML: only accept an explicit language- / lang- prefixed token, so
	// arbitrary highlight classes don't leak in as a "language".
	if t := langTokenRe.FindStringSubmatch(inner); t != nil {
		return strings.ToLower(t[1])
	}
	return ""
}

// isLanguageToken reports whether v looks like a bare language name (a single
// token of letters/digits plus the few punctuation chars languages use, like
// "c++", "c#", "objective-c"). It rejects multi-word class lists and anything
// with spaces, so generic CSS classes aren't mistaken for a language.
func isLanguageToken(v string) bool {
	if v == "" || strings.ContainsAny(v, " \t") {
		return false
	}
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '+', r == '#', r == '-', r == '.':
		default:
			return false
		}
	}
	return true
}
