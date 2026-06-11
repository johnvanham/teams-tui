package graph

import (
	"html"
	"regexp"
	"strings"
)

// Microsoft Teams encodes emoji inside chat message HTML as <emoji> elements,
// e.g. <emoji id="smile" alt="😄" title="Smile"></emoji>. The alt attribute
// carries the standard Unicode codepoint for the emoji (the same character
// Teams renders with its own artwork); terminals render that codepoint with the
// font's own emoji glyph. We therefore resolve each <emoji> tag to its Unicode
// character so emoji survive the HTML→plaintext conversion instead of being
// silently stripped with every other tag.
var (
	// emojiTagRe matches a full <emoji ...>...</emoji> element, capturing the
	// attribute list (group 1) and the inner text (group 2). (?is) makes it
	// case-insensitive and dot-matches-newline, matching the rest of this
	// package's regex-based HTML handling. Some payloads self-close the tag
	// (<emoji ... />); the optional inner/closing portion handles both forms.
	emojiTagRe = regexp.MustCompile(`(?is)<emoji\b([^>]*?)/?>(?:(.*?)</emoji\s*>)?`)
	// emojiAltRe / emojiTitleRe / emojiIDRe pull candidate values from the
	// attribute list, tolerant of single or double quotes and attribute order.
	emojiAltRe   = regexp.MustCompile(`(?is)\balt\s*=\s*("([^"]*)"|'([^']*)')`)
	emojiTitleRe = regexp.MustCompile(`(?is)\btitle\s*=\s*("([^"]*)"|'([^']*)')`)
	emojiIDRe    = regexp.MustCompile(`(?is)\bid\s*=\s*("([^"]*)"|'([^']*)')`)
)

// emojiKeywords maps Teams' legacy keyword emoji ids (used when a tag carries no
// usable Unicode in its alt/inner text) to Unicode characters. It mirrors the
// keyword set handled by Reaction.Emoji so reactions and inline emoji stay
// visually consistent.
var emojiKeywords = map[string]string{
	"like":      "👍",
	"yes":       "👍",
	"heart":     "❤️",
	"love":      "❤️",
	"laugh":     "😆",
	"smile":     "😄",
	"happy":     "😊",
	"wink":      "😉",
	"surprised": "😮",
	"sad":       "😢",
	"cry":       "😢",
	"angry":     "😡",
	"cool":      "😎",
	"think":     "🤔",
	"clap":      "👏",
	"party":     "🎉",
	"fire":      "🔥",
	"check":     "✅",
	"cross":     "❌",
	"star":      "⭐",
	"rocket":    "🚀",
	"ok":        "👌",
	"pray":      "🙏",
}

// replaceEmojiTags rewrites every <emoji> element in HTML body content with its
// Unicode character so the subsequent generic tag strip preserves the glyph.
// Resolution order: alt attribute (the authoritative Unicode), then inner text,
// then a keyword lookup by id. A tag that resolves to nothing is left for the
// generic strip to remove.
func replaceEmojiTags(content string) string {
	return emojiTagRe.ReplaceAllStringFunc(content, func(tag string) string {
		sub := emojiTagRe.FindStringSubmatch(tag)
		if sub == nil {
			return tag
		}
		attrs, inner := sub[1], sub[2]

		// alt holds the canonical Unicode emoji.
		if alt := html.UnescapeString(attrValue(emojiAltRe.FindStringSubmatch(attrs))); alt != "" {
			return alt
		}

		// Some payloads put the character (or an HTML entity for it) between the
		// tags instead of in alt.
		if inner = strings.TrimSpace(html.UnescapeString(inner)); inner != "" {
			return inner
		}

		// Fall back to a keyword id like id="smile".
		id := html.UnescapeString(attrValue(emojiIDRe.FindStringSubmatch(attrs)))
		if glyph, ok := emojiKeywords[strings.ToLower(strings.TrimSpace(id))]; ok {
			return glyph
		}

		// As a last resort surface the human-readable title (e.g. "Smile") so
		// the message conveys that an emoji was present rather than dropping it.
		if title := html.UnescapeString(attrValue(emojiTitleRe.FindStringSubmatch(attrs))); title != "" {
			return ":" + title + ":"
		}

		return ""
	})
}
