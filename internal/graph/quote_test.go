package graph

import (
	"encoding/json"
	"strings"
	"testing"
)

// A real messageReference attachment content payload as stored by Teams (from a
// captured message body dump), trimmed to the fields we decode.
const sampleReferenceContent = `{"messageId":"1782988789222","messagePreview":"Hmm, I don't think the forms load via ajax do they?","messageSender":{"application":null,"device":null,"user":{"userIdentityType":"aadUser","tenantId":"7f4229be-5beb-46dd-99dc-292d406c09f3","id":"2e846435-5c17-4635-a1bf-8454b1fd228f","displayName":"John Van Ham"}}}`

func TestMessagePlainTextResolvesReply(t *testing.T) {
	m := Message{
		Body: MessageBody{
			ContentType: "html",
			Content:     `<attachment id="1782988789222"></attachment><p>Apparently they do</p>`,
		},
		Attachments: []Attachment{
			{ID: "1782988789222", ContentType: "messageReference", Content: sampleReferenceContent},
		},
	}

	want := "> John Van Ham wrote:\n" +
		"> Hmm, I don't think the forms load via ajax do they?\n" +
		"Apparently they do"
	if got := m.PlainText(); got != want {
		t.Errorf("PlainText() =\n%q\nwant\n%q", got, want)
	}
}

func TestMessagePlainTextNoReplyMatchesBody(t *testing.T) {
	m := Message{Body: MessageBody{ContentType: "html", Content: `<p>plain</p>`}}
	if got, want := m.PlainText(), m.Body.PlainText(); got != want {
		t.Errorf("PlainText() = %q, want body PlainText() %q", got, want)
	}
}

func TestMessagePlainTextMultiLinePreview(t *testing.T) {
	content, _ := json.Marshal(map[string]any{
		"messageId":      "42",
		"messagePreview": "one\ntwo",
		"messageSender":  map[string]any{"user": map[string]any{"displayName": "Ada"}},
	})
	m := Message{
		Body: MessageBody{ContentType: "html", Content: `<attachment id="42"></attachment><p>reply</p>`},
		Attachments: []Attachment{
			{ID: "42", ContentType: "messageReference", Content: string(content)},
		},
	}
	want := "> Ada wrote:\n> one\n> two\nreply"
	if got := m.PlainText(); got != want {
		t.Errorf("PlainText() =\n%q\nwant\n%q", got, want)
	}
}

func TestReplyReferenceAttachment(t *testing.T) {
	r := Reply{
		MessageID:  "1782988789222",
		SenderName: "John Van Ham",
		SenderID:   "2e846435",
		Preview:    "Hmm, I don't think the forms load via ajax do they?",
	}
	prefix, attachment := r.referenceAttachment()

	if want := `<attachment id="1782988789222"></attachment>`; prefix != want {
		t.Errorf("body prefix = %q, want %q", prefix, want)
	}
	if attachment["id"] != "1782988789222" {
		t.Errorf("attachment id = %v", attachment["id"])
	}
	if attachment["contentType"] != messageReferenceType {
		t.Errorf("attachment contentType = %v, want %q", attachment["contentType"], messageReferenceType)
	}

	// The content must round-trip back into the same preview/sender the receive
	// side reads, so our replies render like native ones.
	raw, ok := attachment["content"].(string)
	if !ok {
		t.Fatalf("attachment content is not a string: %T", attachment["content"])
	}
	var ref messageReferenceContent
	if err := json.Unmarshal([]byte(raw), &ref); err != nil {
		t.Fatalf("content is not valid JSON: %v", err)
	}
	if ref.MessageID != r.MessageID || ref.MessagePreview != r.Preview {
		t.Errorf("round-tripped content = %+v", ref)
	}
	if ref.senderName() != r.SenderName {
		t.Errorf("senderName = %q, want %q", ref.senderName(), r.SenderName)
	}
}

// Sending a reply then rendering it back should reproduce the same quote lines,
// proving the send and receive sides agree on the format.
func TestReplyRoundTrip(t *testing.T) {
	r := Reply{MessageID: "9", SenderName: "Bob", Preview: "hi there"}
	prefix, attachment := r.referenceAttachment()

	m := Message{
		Body: MessageBody{ContentType: "html", Content: prefix + "<p>my answer</p>"},
		Attachments: []Attachment{{
			ID:          attachment["id"].(string),
			ContentType: attachment["contentType"].(string),
			Content:     attachment["content"].(string),
		}},
	}
	got := m.PlainText()
	if !strings.Contains(got, "> Bob wrote:") || !strings.Contains(got, "> hi there") ||
		!strings.HasSuffix(got, "my answer") {
		t.Errorf("round-tripped PlainText() = %q", got)
	}
}
