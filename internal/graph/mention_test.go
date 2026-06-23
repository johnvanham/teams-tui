package graph

import (
	"strings"
	"testing"
)

func TestComposeHTMLWithMentionsNoMentions(t *testing.T) {
	body, payloads := ComposeHTMLWithMentions("hello world", nil)
	if body != ComposeHTML("hello world") {
		t.Errorf("body = %q, want identical to ComposeHTML", body)
	}
	if payloads != nil {
		t.Errorf("payloads = %v, want nil", payloads)
	}
}

func TestComposeHTMLWithMentionsBasic(t *testing.T) {
	mentions := []Mention{{DisplayName: "Ada Lovelace", UserID: "u-ada"}}
	body, payloads := ComposeHTMLWithMentions("hi @Ada Lovelace how are you", mentions)

	want := `<p>hi <at id="0">Ada Lovelace</at> how are you</p>`
	if body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
	if len(payloads) != 1 {
		t.Fatalf("got %d payloads, want 1", len(payloads))
	}
	p := payloads[0]
	if p.ID != 0 || p.MentionText != "Ada Lovelace" {
		t.Errorf("payload = %+v", p)
	}
	if p.Mentioned.User.ID != "u-ada" || p.Mentioned.User.UserIdentityType != "aadUser" {
		t.Errorf("mentioned user = %+v", p.Mentioned.User)
	}
}

func TestComposeHTMLWithMentionsEscapesSurroundingText(t *testing.T) {
	mentions := []Mention{{DisplayName: "Bob", UserID: "u-bob"}}
	body, _ := ComposeHTMLWithMentions("hey @Bob <script> & stuff", mentions)
	if strings.Contains(body, "<script>") {
		t.Errorf("surrounding HTML not escaped: %q", body)
	}
	if !strings.Contains(body, `<at id="0">Bob</at>`) {
		t.Errorf("mention markup missing: %q", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") || !strings.Contains(body, "&amp;") {
		t.Errorf("expected escaped entities in %q", body)
	}
}

func TestComposeHTMLWithMentionsLongestNameWins(t *testing.T) {
	// "Ann Marie" and "Ann" both participate; "@Ann Marie" must match the
	// longer name, not "@Ann" followed by " Marie".
	mentions := []Mention{
		{DisplayName: "Ann", UserID: "u-ann"},
		{DisplayName: "Ann Marie", UserID: "u-annmarie"},
	}
	body, payloads := ComposeHTMLWithMentions("@Ann Marie hi", mentions)
	if !strings.Contains(body, `<at id="0">Ann Marie</at>`) {
		t.Errorf("expected Ann Marie match, got %q", body)
	}
	if len(payloads) != 1 || payloads[0].Mentioned.User.ID != "u-annmarie" {
		t.Errorf("payloads = %+v", payloads)
	}
}

func TestComposeHTMLWithMentionsWordBoundary(t *testing.T) {
	// "@Sam" must not match inside "@Samuel".
	mentions := []Mention{{DisplayName: "Sam", UserID: "u-sam"}}
	body, payloads := ComposeHTMLWithMentions("ping @Samuel later", mentions)
	if strings.Contains(body, "<at") {
		t.Errorf("should not match inside a longer word: %q", body)
	}
	if payloads != nil {
		t.Errorf("payloads = %v, want nil", payloads)
	}
}

func TestComposeHTMLWithMentionsRepeated(t *testing.T) {
	mentions := []Mention{{DisplayName: "Cleo", UserID: "u-cleo"}}
	body, payloads := ComposeHTMLWithMentions("@Cleo and @Cleo again", mentions)
	if !strings.Contains(body, `<at id="0">Cleo</at>`) || !strings.Contains(body, `<at id="1">Cleo</at>`) {
		t.Errorf("expected two distinct ids: %q", body)
	}
	if len(payloads) != 2 || payloads[0].ID != 0 || payloads[1].ID != 1 {
		t.Errorf("payloads = %+v", payloads)
	}
}

func TestComposeHTMLWithMentionsNotInCodeBlock(t *testing.T) {
	mentions := []Mention{{DisplayName: "Dan", UserID: "u-dan"}}
	body, payloads := ComposeHTMLWithMentions("```\n@Dan\n```", mentions)
	if strings.Contains(body, "<at") {
		t.Errorf("mentions inside code fence should not be marked up: %q", body)
	}
	if payloads != nil {
		t.Errorf("payloads = %v, want nil", payloads)
	}
}
