package graph

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Teams stores a reply/quote not as inline <blockquote> HTML but as a
// "messageReference" attachment: the body carries an empty <attachment id="…">
// element whose id matches an entry in the message's attachments array with
// contentType "messageReference". That entry's Content is a JSON blob naming the
// referenced message, a plaintext preview of the quoted text, and its sender.
// This file turns that structure into the same "> "-prefixed lines the rest of
// the app already uses for quotes (see text.go / ui/view.go), so received
// replies render like any other quote, and builds the same structure on the
// send side so our replies match the native Teams client.

const messageReferenceType = "messageReference"

// attachmentTagRe matches an <attachment id="…"></attachment> placeholder that
// Teams inserts into the body for each reference attachment. Group 1 is the id,
// which ties the placeholder to an entry in Message.Attachments.
var attachmentTagRe = regexp.MustCompile(`(?is)<attachment\s+id\s*=\s*("([^"]*)"|'([^']*)')\s*>\s*</attachment\s*>`)

// messageReferenceContent is the JSON payload stored in a messageReference
// attachment's Content. Only the fields we render are decoded.
type messageReferenceContent struct {
	MessageID      string `json:"messageId"`
	MessagePreview string `json:"messagePreview"`
	MessageSender  struct {
		User *struct {
			DisplayName string `json:"displayName"`
			ID          string `json:"id"`
		} `json:"user"`
		Application *struct {
			DisplayName string `json:"displayName"`
		} `json:"application"`
	} `json:"messageSender"`
}

// Quote is a resolved reply reference: the quoted message's id, the sender's
// display name, and a plaintext preview of the quoted text.
type Quote struct {
	MessageID  string
	SenderName string
	Preview    string
}

// senderName returns the referenced message sender's display name, preferring
// the user over an application (bot), or "" when neither is present.
func (c messageReferenceContent) senderName() string {
	if c.MessageSender.User != nil && c.MessageSender.User.DisplayName != "" {
		return c.MessageSender.User.DisplayName
	}
	if c.MessageSender.Application != nil && c.MessageSender.Application.DisplayName != "" {
		return c.MessageSender.Application.DisplayName
	}
	return ""
}

// Quotes returns the reply references carried by the message as messageReference
// attachments, keyed by attachment id, in the order they appear in the
// attachments list. Returns nil when the message has no reply references.
func (m *Message) Quotes() map[string]Quote {
	var quotes map[string]Quote
	for _, a := range m.Attachments {
		if !strings.EqualFold(a.ContentType, messageReferenceType) || a.Content == "" {
			continue
		}
		var ref messageReferenceContent
		if err := json.Unmarshal([]byte(a.Content), &ref); err != nil {
			continue
		}
		if quotes == nil {
			quotes = make(map[string]Quote)
		}
		quotes[a.ID] = Quote{
			MessageID:  ref.MessageID,
			SenderName: ref.senderName(),
			Preview:    ref.MessagePreview,
		}
	}
	return quotes
}

// PlainText renders the message body to plain text with reply references
// resolved: each <attachment id="…"> placeholder for a messageReference is
// replaced by the quoted text as "> "-prefixed lines (a "> Sender wrote:"
// header when the sender is known), so received replies render like any other
// quote. Messages without references behave exactly like Body.PlainText.
func (m *Message) PlainText() string {
	quotes := m.Quotes()
	if len(quotes) == 0 {
		return m.Body.PlainText()
	}

	// Replace each <attachment> placeholder in the raw HTML with the quote's
	// "> " lines before the body is converted, so the existing text pipeline
	// (and the UI's quote styling) handles them uniformly. Non-HTML bodies
	// never contain the placeholder, so this is a no-op for them.
	content := attachmentTagRe.ReplaceAllStringFunc(m.Body.Content, func(tag string) string {
		sub := attachmentTagRe.FindStringSubmatch(tag)
		if sub == nil {
			return tag
		}
		q, ok := quotes[attrValue(sub)]
		if !ok {
			// Not a reply reference (e.g. a card attachment): drop the
			// placeholder rather than leave a stray tag for the strip below.
			return ""
		}
		return "\n" + quoteLines(q) + "\n"
	})

	body := MessageBody{ContentType: m.Body.ContentType, Content: content}
	return body.PlainText()
}

// Reply describes the message an outgoing message is replying to. It carries
// everything Teams needs to build a messageReference attachment so the reply
// renders as a native quote for every participant.
type Reply struct {
	MessageID  string // the referenced message's id
	SenderName string // the referenced sender's display name (for the preview)
	SenderID   string // the referenced sender's AAD user id (optional)
	Preview    string // plaintext preview of the quoted message
}

// referenceAttachment builds the messageReference attachment (matching what the
// native Teams client stores) plus the <attachment> placeholder that must be
// prepended to the body HTML so the two are linked by id. Returns the body
// prefix and the attachment payload for the request's "attachments" array.
func (r Reply) referenceAttachment() (bodyPrefix string, attachment map[string]any) {
	content := messageReferenceContent{
		MessageID:      r.MessageID,
		MessagePreview: r.Preview,
	}
	if r.SenderName != "" || r.SenderID != "" {
		content.MessageSender.User = &struct {
			DisplayName string `json:"displayName"`
			ID          string `json:"id"`
		}{DisplayName: r.SenderName, ID: r.SenderID}
	}
	// Marshalling can't fail for this plain struct; ignore the error.
	raw, _ := json.Marshal(content)

	bodyPrefix = `<attachment id="` + r.MessageID + `"></attachment>`
	attachment = map[string]any{
		"id":          r.MessageID,
		"contentType": messageReferenceType,
		"content":     string(raw),
	}
	return bodyPrefix, attachment
}

// quoteLines formats a resolved Quote as the "> "-prefixed lines the renderer
// styles. The sender header is omitted when unknown, and a multi-line preview is
// prefixed line-by-line so the whole quote sits inside the styled block.
func quoteLines(q Quote) string {
	var out []string
	if q.SenderName != "" {
		out = append(out, "> "+q.SenderName+" wrote:")
	}
	for _, ln := range strings.Split(q.Preview, "\n") {
		out = append(out, "> "+ln)
	}
	return strings.Join(out, "\n")
}
