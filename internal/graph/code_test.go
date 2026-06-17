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
			// The canonical form the Teams service stores for a sent code block:
			// a <codeblock> element wrapping a <code> with literal newlines.
			name: "teams codeblock element",
			body: MessageBody{
				ContentType: "html",
				Content:     "<codeblock class=\"\"><code>&lt;?php\n    echo 'another test';\n?&gt;</code></codeblock> ",
			},
			want: "```\n<?php\n    echo 'another test';\n?>\n```",
		},
		{
			name: "teams codeblock with language class",
			body: MessageBody{
				ContentType: "html",
				Content:     `<codeblock class="language-go"><code>fmt.Println("hi")</code></codeblock>`,
			},
			want: "```go\nfmt.Println(\"hi\")\n```",
		},
		{
			// Teams stores the language as a bare, often capitalized class value.
			name: "teams codeblock bare language class",
			body: MessageBody{
				ContentType: "html",
				Content:     "<codeblock class=\"Php\"><code>&lt;?php\necho 'test';\n?&gt;</code></codeblock>",
			},
			want: "```php\n<?php\necho 'test';\n?>\n```",
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
		name      string
		attrs     string
		inner     string
		bareClass bool
		want      string
	}{
		{"data-language", `data-language="Go"`, "", false, "go"},
		{"class language-", `class="language-rust"`, "", false, "rust"},
		{"class lang-", `class="lang-js"`, "", false, "js"},
		{"class list with token", `class="hljs language-c++"`, "", false, "c++"},
		{"bare class on codeblock", `class="Php"`, "", true, "php"},
		{"bare class capitalized", `class="Go"`, "", true, "go"},
		{"bare class ignored on pre", `class="plain"`, "", false, ""},
		{"inner language- token", "", `<code class="language-python">`, false, "python"},
		{"inner highlight class ignored", "", `<span class="hljs-keyword">`, true, ""},
		{"empty", "", "", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectLanguage(tt.attrs, tt.inner, tt.bareClass); got != tt.want {
				t.Errorf("detectLanguage(%q, %q, %v) = %q, want %q",
					tt.attrs, tt.inner, tt.bareClass, got, tt.want)
			}
		})
	}
}
