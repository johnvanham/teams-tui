package graph

import "testing"

func TestComposeHTML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "single prose line",
			in:   "hello world",
			want: "<p>hello world</p>",
		},
		{
			name: "html is escaped",
			in:   "a < b && c > d",
			want: "<p>a &lt; b &amp;&amp; c &gt; d</p>",
		},
		{
			name: "blank line becomes empty paragraph",
			in:   "one\n\ntwo",
			want: "<p>one</p><p></p><p>two</p>",
		},
		{
			name: "inline code",
			in:   "run `go test` now",
			want: "<p>run <code>go test</code> now</p>",
		},
		{
			name: "inline code is escaped",
			in:   "use `a < b`",
			want: "<p>use <code>a &lt; b</code></p>",
		},
		{
			name: "unmatched backtick is literal",
			in:   "a ` b",
			want: "<p>a ` b</p>",
		},
		{
			name: "fenced block preserves lines",
			in:   "```\ndef f():\n    return 1\n```",
			want: "<pre><code>def f():\n    return 1</code></pre>",
		},
		{
			name: "fenced block with language",
			in:   "```go\nfmt.Println(\"hi\")\n```",
			want: `<pre data-language="go" class="language-go"><code>fmt.Println(&#34;hi&#34;)</code></pre>`,
		},
		{
			name: "prose then code block",
			in:   "see:\n```\nx = 1\n```",
			want: "<p>see:</p><pre><code>x = 1</code></pre>",
		},
		{
			name: "unterminated fence still closes",
			in:   "```\nx = 1",
			want: "<pre><code>x = 1</code></pre>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ComposeHTML(tt.in); got != tt.want {
				t.Errorf("ComposeHTML(%q) =\n%q\nwant\n%q", tt.in, got, tt.want)
			}
		})
	}
}

// TestComposeRoundTrip verifies that text sent through ComposeHTML parses back
// to the same logical content via PlainText, so what the user typed is what
// they (and others) see rendered.
func TestComposeRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "code block round trips",
			in:   "```go\nfmt.Println(\"hi\")\n```",
			want: "```go\nfmt.Println(\"hi\")\n```",
		},
		{
			name: "inline code round trips",
			in:   "run `go test` now",
			want: "run `go test` now",
		},
		{
			name: "prose with code block",
			in:   "see:\n```\nx = 1\n```",
			want: "see:\n\n```\nx = 1\n```",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := MessageBody{ContentType: "html", Content: ComposeHTML(tt.in)}
			if got := body.PlainText(); got != tt.want {
				t.Errorf("round trip of %q =\n%q\nwant\n%q", tt.in, got, tt.want)
			}
		})
	}
}
