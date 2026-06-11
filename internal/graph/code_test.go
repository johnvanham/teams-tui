package graph

import "testing"

func TestPlainTextCodeBlocks(t *testing.T) {
	tests := []struct {
		name string
		body MessageBody
		want string
	}{
		{
			name: "fenced block preserves indentation and blank lines",
			body: MessageBody{
				ContentType: "html",
				Content:     "<p>see this:</p><pre><code>def f():\n    if x:\n        return 1\n\n    return 0</code></pre>",
			},
			want: "see this:\n\n```\ndef f():\n    if x:\n        return 1\n\n    return 0\n```",
		},
		{
			name: "language hint from data-language",
			body: MessageBody{
				ContentType: "html",
				Content:     `<pre data-language="go"><code>fmt.Println("hi")</code></pre>`,
			},
			want: "```go\nfmt.Println(\"hi\")\n```",
		},
		{
			name: "language hint from class language-python",
			body: MessageBody{
				ContentType: "html",
				Content:     `<pre><code class="language-python">print(1)</code></pre>`,
			},
			want: "```python\nprint(1)\n```",
		},
		{
			name: "br inside pre becomes newline",
			body: MessageBody{
				ContentType: "html",
				Content:     "<pre>line one<br>line two</pre>",
			},
			want: "```\nline one\nline two\n```",
		},
		{
			name: "entities decoded inside code",
			body: MessageBody{
				ContentType: "html",
				Content:     "<pre><code>if a &lt; b &amp;&amp; c &gt; d</code></pre>",
			},
			want: "```\nif a < b && c > d\n```",
		},
		{
			name: "inline code wrapped in backticks",
			body: MessageBody{
				ContentType: "html",
				Content:     "<p>run <code>go test ./...</code> first</p>",
			},
			want: "run `go test ./...` first",
		},
		{
			name: "syntax-highlight spans stripped from code",
			body: MessageBody{
				ContentType: "html",
				Content:     `<pre><code><span class="hljs-keyword">return</span> nil</code></pre>`,
			},
			want: "```\nreturn nil\n```",
		},
		{
			name: "prose around block normalized but code untouched",
			body: MessageBody{
				ContentType: "html",
				Content:     "<p>before</p><pre><code>  spaced  </code></pre><p>after</p>",
			},
			want: "before\n\n```\n  spaced  \n```\nafter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.body.PlainText(); got != tt.want {
				t.Errorf("PlainText() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{`data-language="Go"`, "go"},
		{`class="language-rust"`, "rust"},
		{`class="lang-js"`, "js"},
		{`class="hljs language-c++"`, "c++"},
		{`class="plain"`, ""},
		{``, ""},
	}
	for _, tt := range tests {
		if got := detectLanguage(tt.in); got != tt.want {
			t.Errorf("detectLanguage(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
