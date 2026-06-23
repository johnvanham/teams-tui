package graph

import (
	"html"
	"sort"
	"strconv"
	"strings"
)

// Mention is an @-mention of a chat participant to attach to an outgoing
// message. The composer inserts the literal text "@"+DisplayName into the
// message and records a Mention; at send time ComposeHTMLWithMentions rewrites
// each "@DisplayName" occurrence into Teams' <at id="N">DisplayName</at> markup
// and SendMessage emits a matching entry in the message's "mentions" array.
type Mention struct {
	DisplayName string // the member's display name, e.g. "Ada Lovelace"
	UserID      string // the member's AAD user object id
}

// mentionPayload is one entry of the Graph chatMessage "mentions" array. The id
// is a small integer that must match the id on the corresponding <at> element in
// the body HTML.
type mentionPayload struct {
	ID          int            `json:"id"`
	MentionText string         `json:"mentionText"`
	Mentioned   mentionedParty `json:"mentioned"`
}

type mentionedParty struct {
	User mentionedUser `json:"user"`
}

type mentionedUser struct {
	ID               string `json:"id"`
	DisplayName      string `json:"displayName"`
	UserIdentityType string `json:"userIdentityType"`
}

// ComposeHTMLWithMentions is ComposeHTML extended to turn "@DisplayName" runs in
// the prose into <at id="N">DisplayName</at> spans for the given mentions. It
// returns the body HTML and the parallel mentions payload that SendMessage puts
// in the request alongside the body. With no mentions it is identical to
// ComposeHTML.
//
// Markup is only emitted for "@DisplayName" text that actually appears in the
// message, so removing the inserted text (e.g. by editing it out) cleanly drops
// the mention. Each textual occurrence becomes its own <at> with a unique id, so
// mentioning the same person twice notifies once per visible tag, matching the
// desktop client.
func ComposeHTMLWithMentions(text string, mentions []Mention) (string, []mentionPayload) {
	if len(mentions) == 0 {
		return ComposeHTML(text), nil
	}

	// Longer display names first so "@Ann Marie" is matched before "@Ann" when
	// both are participants and one name is a prefix of the other.
	ordered := append([]Mention(nil), mentions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return len(ordered[i].DisplayName) > len(ordered[j].DisplayName)
	})

	var payloads []mentionPayload
	nextID := 0
	// mentionParagraph replaces composeParagraph's escaping for prose lines,
	// emitting <at> spans for recognized "@Name" runs and escaping the rest.
	mention := func(line string) string {
		if strings.TrimSpace(line) == "" {
			return "<p></p>"
		}
		return "<p>" + mentionizeInline(line, ordered, &nextID, &payloads) + "</p>"
	}

	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")

	var b strings.Builder
	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Code fences: emitted verbatim, never scanned for mentions.
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

		// Blockquotes: mentions inside a quote are unusual; keep the existing
		// (escaped) rendering to avoid notifying people from quoted text.
		if isQuoteLine(line) {
			var quote []string
			for ; i < len(lines) && isQuoteLine(lines[i]); i++ {
				quote = append(quote, stripQuotePrefix(lines[i]))
			}
			i--
			b.WriteString(composeBlockquote(quote))
			continue
		}

		b.WriteString(mention(line))
	}

	return b.String(), payloads
}

// mentionizeInline rewrites a single prose line: each "@DisplayName" run for a
// known mention becomes an <at id="N">DisplayName</at> element (and records a
// payload), while every other character — including inline `code` spans — is
// passed through inlineCodeToHTML so it is HTML-escaped exactly as normal prose.
func mentionizeInline(line string, ordered []Mention, nextID *int, payloads *[]mentionPayload) string {
	var b strings.Builder
	for i := 0; i < len(line); {
		if line[i] == '@' {
			if m, n, ok := matchMentionAt(line, i, ordered); ok {
				id := *nextID
				*nextID++
				b.WriteString(`<at id="`)
				b.WriteString(strconv.Itoa(id))
				b.WriteString(`">`)
				b.WriteString(html.EscapeString(m.DisplayName))
				b.WriteString(`</at>`)
				*payloads = append(*payloads, mentionPayload{
					ID:          id,
					MentionText: m.DisplayName,
					Mentioned: mentionedParty{User: mentionedUser{
						ID:               m.UserID,
						DisplayName:      m.DisplayName,
						UserIdentityType: "aadUser",
					}},
				})
				i += n
				continue
			}
		}
		// Accumulate a run of non-mention text up to the next '@' and escape it
		// (with inline-code handling) in one go.
		j := strings.IndexByte(line[i+1:], '@')
		var chunk string
		if j < 0 {
			chunk = line[i:]
			i = len(line)
		} else {
			chunk = line[i : i+1+j]
			i = i + 1 + j
		}
		b.WriteString(inlineCodeToHTML(chunk))
	}
	return b.String()
}

// matchMentionAt reports whether the text starting at line[at] (which is '@') is
// "@"+DisplayName for one of the mentions, requiring the match to end at a word
// boundary so "@Sam" does not match inside "@Samuel". It returns the matched
// mention and the byte length consumed (including the '@').
func matchMentionAt(line string, at int, ordered []Mention) (Mention, int, bool) {
	rest := line[at+1:]
	for _, m := range ordered {
		if m.DisplayName == "" {
			continue
		}
		if !strings.HasPrefix(rest, m.DisplayName) {
			continue
		}
		after := rest[len(m.DisplayName):]
		if boundaryAfterMention(after) {
			return m, 1 + len(m.DisplayName), true
		}
	}
	return Mention{}, 0, false
}

// boundaryAfterMention reports whether the character following a candidate
// mention ends the name cleanly: end of line or a non-word character. Letters
// and digits continuing the run mean the "@Name" was actually a longer word, so
// it is not treated as a mention.
func boundaryAfterMention(after string) bool {
	if after == "" {
		return true
	}
	r := after[0]
	isWord := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
	return !isWord
}
