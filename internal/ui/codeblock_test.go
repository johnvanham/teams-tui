package ui

import "testing"

func TestInOpenCodeBlock(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"empty", "", false},
		{"plain prose", "hello world", false},
		{"just opened fence", "```", true},
		{"opened with language", "```go", true},
		{"inside block", "```\nfmt.Println()", true},
		{"closed block", "```\nx := 1\n```", false},
		{"prose after closed block", "```\nx := 1\n```\ndone", false},
		{"second block opened", "```\na\n```\nmid\n```\nb", true},
		{"indented fence still counts", "  ```\ncode", true},
		{"backticks mid-line are not a fence", "use `code` here", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newComposeModel(tt.value) // cursor at end of value
			if got := m.inOpenCodeBlock(); got != tt.want {
				t.Errorf("inOpenCodeBlock() = %v, want %v\nvalue:\n%s", got, tt.want, tt.value)
			}
		})
	}
}
