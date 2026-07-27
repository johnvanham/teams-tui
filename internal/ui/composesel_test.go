package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"
)

// dragCompose selects from one cell of the compose box to another, as a
// press-drag-release would.
func dragCompose(m *Model, fromRow, fromCol, toRow, toCol int) {
	m.startComposeSelection(m.composeTextLeft()+fromCol, m.composeTextTop()+fromRow)
	m.extendComposeSelection(m.composeTextLeft()+toCol, m.composeTextTop()+toRow)
}

func TestComposeSelectionText(t *testing.T) {
	tests := []struct {
		name                    string
		width                   int
		value                   string
		fr, fc, tr, tc          int
		want                    string
		wantAfterDelete         string
		wantCursorLine, wantCol int
	}{
		{
			name: "within a line", width: 20, value: "hello world",
			fr: 0, fc: 6, tr: 0, tc: 11,
			want: "world", wantAfterDelete: "hello ", wantCursorLine: 0, wantCol: 6,
		},
		{
			name: "backwards drag", width: 20, value: "hello world",
			fr: 0, fc: 11, tr: 0, tc: 6,
			want: "world", wantAfterDelete: "hello ", wantCursorLine: 0, wantCol: 6,
		},
		{
			name: "across a real newline", width: 20, value: "abc\ndef",
			fr: 0, fc: 2, tr: 1, tc: 2,
			want: "c\nde", wantAfterDelete: "abf", wantCursorLine: 0, wantCol: 2,
		},
		{
			// "aaaa bbbb cccc" wraps to "aaaa bbbb " / "cccc" at width 10.
			// The wrap is not a newline, so the copied text must not gain one.
			name: "across a soft wrap", width: 10, value: "aaaa bbbb cccc",
			fr: 0, fc: 5, tr: 1, tc: 2,
			want: "bbbb cc", wantAfterDelete: "aaaa cc", wantCursorLine: 0, wantCol: 5,
		},
		{
			name: "whole buffer", width: 20, value: "one\ntwo",
			fr: 0, fc: 0, tr: 1, tc: 3,
			want: "one\ntwo", wantAfterDelete: "", wantCursorLine: 0, wantCol: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := composeModel(tt.width, 10, tt.value)
			dragCompose(&m, tt.fr, tt.fc, tt.tr, tt.tc)
			if !m.hasComposeSelection() {
				t.Fatal("expected a selection")
			}
			if got := m.composeSelectionText(); got != tt.want {
				t.Errorf("composeSelectionText() = %q, want %q", got, tt.want)
			}
			if !m.deleteComposeSelection() {
				t.Fatal("deleteComposeSelection() reported nothing deleted")
			}
			if got := m.compose.Value(); got != tt.wantAfterDelete {
				t.Errorf("value after delete = %q, want %q", got, tt.wantAfterDelete)
			}
			if l, c := m.compose.Line(), m.compose.Column(); l != tt.wantCursorLine || c != tt.wantCol {
				t.Errorf("cursor after delete = line %d col %d, want line %d col %d",
					l, c, tt.wantCursorLine, tt.wantCol)
			}
			if m.hasComposeSelection() {
				t.Error("selection survived the delete")
			}
		})
	}
}

// TestComposeSelectionStale checks the guards that stand in for clearing the
// selection by hand everywhere the compose box is rewritten.
func TestComposeSelectionStale(t *testing.T) {
	t.Run("text changed", func(t *testing.T) {
		m := composeModel(20, 10, "hello world")
		dragCompose(&m, 0, 0, 0, 5)
		m.compose.SetValue("something else")
		if m.hasComposeSelection() {
			t.Error("selection outlived the text it was taken against")
		}
	})
	t.Run("focus moved", func(t *testing.T) {
		m := composeModel(20, 10, "hello world")
		dragCompose(&m, 0, 0, 0, 5)
		m.focus = focusMessages
		if m.hasComposeSelection() {
			t.Error("selection stayed live after focus left the compose box")
		}
	})
	t.Run("empty drag", func(t *testing.T) {
		m := composeModel(20, 10, "hello world")
		dragCompose(&m, 0, 3, 0, 3)
		if m.hasComposeSelection() {
			t.Error("a click without a drag selected something")
		}
	})
}

// TestComposeHighlight checks the highlight lands on the selected cells and
// nowhere else.
func TestComposeHighlight(t *testing.T) {
	m := composeModel(20, 10, "hello world")
	dragCompose(&m, 0, 6, 0, 11)

	out := m.applyComposeHighlight(m.compose.View())
	if ansi.Strip(out) != ansi.Strip(m.compose.View()) {
		t.Errorf("highlight changed the text: %q", ansi.Strip(out))
	}
	first := strings.Split(out, "\n")[0]
	if !strings.Contains(ansi.Strip(first), "hello world") {
		t.Fatalf("unexpected first row %q", ansi.Strip(first))
	}
	// The selected span must carry styling the unselected one doesn't.
	if ansi.Cut(first, 6, 11) == ansi.Strip(ansi.Cut(first, 6, 11)) {
		t.Error("selected span is unstyled")
	}
	if plain := m.applyComposeHighlight(m.compose.View()); plain == "" {
		t.Error("empty render")
	}

	m.clearComposeSelection()
	if got := m.applyComposeHighlight(m.compose.View()); got != m.compose.View() {
		t.Error("highlight applied without a selection")
	}
}

// TestComposeSelectionDoubleWidth checks that selection endpoints are display
// columns, so a drag over double-width runes selects whole characters.
func TestComposeSelectionDoubleWidth(t *testing.T) {
	m := composeModel(20, 10, "日本語x")
	// Cells 2-3 are 本, 4-5 語.
	dragCompose(&m, 0, 2, 0, 6)
	if got := m.composeSelectionText(); got != "本語" {
		t.Errorf("composeSelectionText() = %q, want %q", got, "本語")
	}
}

// TestComposeSelectionKeys drives the real key handler to check that a held
// selection behaves like it would in any editor.
func TestComposeSelectionKeys(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
		want string
	}{
		{"typing replaces", tea.KeyPressMsg{Code: 'X', Text: "X"}, "hello X"},
		{"backspace deletes", tea.KeyPressMsg{Code: tea.KeyBackspace}, "hello "},
		{"delete deletes", tea.KeyPressMsg{Code: tea.KeyDelete}, "hello "},
		{"newline replaces", tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt}, "hello \n"},
		{"navigation keeps the text", tea.KeyPressMsg{Code: tea.KeyLeft}, "hello world"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := composeModel(20, 10, "hello world")
			dragCompose(&m, 0, 6, 0, 11)
			next, _ := m.handleKey(tt.key)
			got := next.(Model)
			if got.compose.Value() != tt.want {
				t.Errorf("value = %q, want %q", got.compose.Value(), tt.want)
			}
			if got.hasComposeSelection() {
				t.Error("selection survived the keystroke")
			}
		})
	}
}

// TestComposeSelectionCopyKey checks ctrl+c copies the selection rather than
// quitting, and that it goes back to quitting once the selection is gone. The
// copy command itself is never run here: it shells out to a clipboard helper,
// which would block the test.
func TestComposeSelectionCopyKey(t *testing.T) {
	ctrlC := tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}

	m := composeModel(20, 10, "hello world")
	dragCompose(&m, 0, 6, 0, 11)
	if got := m.composeSelectionText(); got != "world" {
		t.Fatalf("selection = %q, want %q", got, "world")
	}
	next, cmd := m.handleKey(ctrlC)
	if cmd == nil {
		t.Fatal("ctrl+c produced no command")
	}
	after := next.(Model)
	if after.hasComposeSelection() {
		t.Error("selection survived the copy")
	}

	// With nothing selected ctrl+c quits.
	_, cmd = after.handleKey(ctrlC)
	if cmd == nil {
		t.Fatal("ctrl+c without a selection produced no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c without a selection didn't quit; got %T", cmd())
	}
}

// TestComposePasteReplacesSelection checks bracketed-paste text lands over the
// selection rather than beside it.
func TestComposePasteReplacesSelection(t *testing.T) {
	m := composeModel(20, 10, "hello world")
	dragCompose(&m, 0, 6, 0, 11)
	next, _ := m.handlePaste(tea.PasteMsg{Content: "there"})
	if got := next.(Model).compose.Value(); got != "hello there" {
		t.Errorf("value after paste = %q, want %q", got, "hello there")
	}
}
