package graph

import "testing"

func TestMessageBodyPlainText(t *testing.T) {
	tests := []struct {
		name string
		body MessageBody
		want string
	}{
		{
			name: "plain text passthrough",
			body: MessageBody{ContentType: "text", Content: "just text 😀"},
			want: "just text 😀",
		},
		{
			name: "html with emoji preserved",
			body: MessageBody{
				ContentType: "html",
				Content:     `<p>nice work <emoji id="thumbsup" alt="👍" title="Thumbs up"></emoji></p>`,
			},
			want: "nice work 👍",
		},
		{
			name: "html emoji between words",
			body: MessageBody{
				ContentType: "html",
				Content:     `<div>let's <emoji alt="🎉"></emoji> celebrate</div>`,
			},
			want: "let's 🎉 celebrate",
		},
		{
			name: "tags stripped entities decoded",
			body: MessageBody{
				ContentType: "html",
				Content:     `<p>a &amp; b</p>`,
			},
			want: "a & b",
		},
		{
			name: "blockquote becomes quoted lines",
			body: MessageBody{
				ContentType: "html",
				Content:     `<blockquote><p>quoted</p></blockquote><p>my reply</p>`,
			},
			want: "> quoted\nmy reply",
		},
		{
			name: "multi-paragraph blockquote",
			body: MessageBody{
				ContentType: "html",
				Content:     `<blockquote><p>one</p><p>two</p></blockquote>`,
			},
			want: "> one\n> two",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.body.PlainText()
			if got != tt.want {
				t.Errorf("PlainText() = %q, want %q", got, tt.want)
			}
		})
	}
}
