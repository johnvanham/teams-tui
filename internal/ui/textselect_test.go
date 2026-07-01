package ui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/jvh/teams-tui/internal/ui/styles"
)

// modelWithContent builds a Model whose messages viewport holds the given
// content lines, enough for the selection helpers (which read selContent) to
// operate without a full render pipeline.
func modelWithContent(lines ...string) Model {
	return Model{selContent: lines}
}

func TestSelectionTextSingleLine(t *testing.T) {
	m := modelWithContent("hello world")
	m.selecting = true
	m.selAnchorLn, m.selAnchorCol = 0, 0
	m.selCurLn, m.selCurCol = 0, 5
	if got := m.selectionText(); got != "hello" {
		t.Fatalf("selectionText() = %q, want %q", got, "hello")
	}
}

func TestSelectionTextReversedDrag(t *testing.T) {
	// Dragging right-to-left (cursor before anchor) should still yield the
	// text in reading order.
	m := modelWithContent("hello world")
	m.selecting = true
	m.selAnchorLn, m.selAnchorCol = 0, 11
	m.selCurLn, m.selCurCol = 0, 6
	if got := m.selectionText(); got != "world" {
		t.Fatalf("selectionText() = %q, want %q", got, "world")
	}
}

func TestSelectionTextMultiLine(t *testing.T) {
	m := modelWithContent("first line", "second line", "third line")
	m.selecting = true
	// From column 6 of line 0 ("line") through column 6 of line 2 ("third ").
	m.selAnchorLn, m.selAnchorCol = 0, 6
	m.selCurLn, m.selCurCol = 2, 6
	want := "line\nsecond line\nthird "
	if got := m.selectionText(); got != want {
		t.Fatalf("selectionText() = %q, want %q", got, want)
	}
}

func TestSelectionTextRejoinsSoftWrap(t *testing.T) {
	// A single logical prose line wrapped across two content lines must copy
	// back as one line (joined with a space), while a real newline stays a
	// newline.
	m := modelWithContent("the quick brown", "fox jumps", "second paragraph")
	// Line 1 ("fox jumps") is a soft continuation of line 0; line 2 is a hard
	// break (a real newline in the source).
	m.selWrapCont = []bool{false, true, false}
	m.selecting = true
	m.selAnchorLn, m.selAnchorCol = 0, 0
	m.selCurLn, m.selCurCol = 2, ansi.StringWidth("second paragraph")
	want := "the quick brown fox jumps\nsecond paragraph"
	if got := m.selectionText(); got != want {
		t.Fatalf("selectionText() = %q, want %q", got, want)
	}
}

func TestSelectionTextPartialSoftWrap(t *testing.T) {
	// Selecting from the middle of a wrapped line through its continuation still
	// rejoins with a space.
	m := modelWithContent("the quick brown", "fox jumps")
	m.selWrapCont = []bool{false, true}
	m.selecting = true
	m.selAnchorLn, m.selAnchorCol = 0, 4 // start at "quick"
	m.selCurLn, m.selCurCol = 1, 3       // end after "fox"
	want := "quick brown fox"
	if got := m.selectionText(); got != want {
		t.Fatalf("selectionText() = %q, want %q", got, want)
	}
}

func TestSelectionTextStripsANSI(t *testing.T) {
	// A styled (ANSI-carrying) content line must yield plain text.
	styled := styles.SenderName.Render("Alice") + " " + styles.Timestamp.Render("hi there")
	m := modelWithContent(styled)
	plainWidth := ansi.StringWidth(styled)
	m.selecting = true
	m.selAnchorLn, m.selAnchorCol = 0, 0
	m.selCurLn, m.selCurCol = 0, plainWidth
	got := m.selectionText()
	want := ansi.Strip(styled)
	if got != want {
		t.Fatalf("selectionText() = %q, want %q", got, want)
	}
	if ansi.Strip(got) != got {
		t.Fatalf("selectionText() still contains ANSI escapes: %q", got)
	}
}

func TestHasSelectionEmptyWhenAnchorEqualsCursor(t *testing.T) {
	m := modelWithContent("hello")
	m.selecting = true
	m.selAnchorLn, m.selAnchorCol = 0, 3
	m.selCurLn, m.selCurCol = 0, 3
	if m.hasSelection() {
		t.Fatal("hasSelection() = true for a zero-width selection, want false")
	}
	if got := m.selectionText(); got != "" {
		t.Fatalf("selectionText() = %q, want empty", got)
	}
}

func TestHasSelectionFalseWhenNotSelecting(t *testing.T) {
	m := modelWithContent("hello")
	m.selecting = false
	m.selAnchorLn, m.selAnchorCol = 0, 0
	m.selCurLn, m.selCurCol = 0, 5
	if m.hasSelection() {
		t.Fatal("hasSelection() = true while not selecting, want false")
	}
}

func TestApplySelectionHighlightPreservesWidth(t *testing.T) {
	// Highlighting must not change the displayed width of a line (so layout and
	// the scrollbar stay correct).
	content := "hello world"
	m := modelWithContent(content)
	m.selecting = true
	m.selAnchorLn, m.selAnchorCol = 0, 0
	m.selCurLn, m.selCurCol = 0, 5
	out := m.applySelectionHighlight(content)
	if ansi.StringWidth(out) != ansi.StringWidth(content) {
		t.Fatalf("highlight changed width: got %d want %d",
			ansi.StringWidth(out), ansi.StringWidth(content))
	}
	if ansi.Strip(out) != content {
		t.Fatalf("highlight changed plain text: %q", ansi.Strip(out))
	}
}

func TestRenderBodyLinesMarksSoftWrap(t *testing.T) {
	// A prose line longer than the width wraps into multiple content lines; only
	// the continuations are flagged. A hard newline in the source is not.
	text := "alpha beta gamma delta\nepsilon"
	lines, cont := renderBodyLines(text, 12, nil)
	if len(lines) != len(cont) {
		t.Fatalf("lines/cont length mismatch: %d vs %d", len(lines), len(cont))
	}
	if len(lines) < 3 {
		t.Fatalf("expected the first line to wrap into >=2 lines plus 'epsilon', got %d lines: %q", len(lines), lines)
	}
	if cont[0] {
		t.Errorf("first content line should be a hard break, cont[0]=true")
	}
	// The last line is "epsilon" (a real newline in the source), so it must be a
	// hard break, and at least one line before it must be a soft continuation.
	if cont[len(cont)-1] {
		t.Errorf("line after a real newline should be a hard break, got continuation")
	}
	sawContinuation := false
	for _, c := range cont[1 : len(cont)-1] {
		if c {
			sawContinuation = true
		}
	}
	if !sawContinuation {
		t.Errorf("expected at least one soft-wrap continuation, cont=%v", cont)
	}
}

func TestClearSelectionResets(t *testing.T) {
	m := modelWithContent("hello")
	m.selecting = true
	m.selAnchorLn, m.selAnchorCol = 1, 2
	m.selCurLn, m.selCurCol = 3, 4
	m.clearSelection()
	if m.selecting || m.selAnchorLn != 0 || m.selAnchorCol != 0 || m.selCurLn != 0 || m.selCurCol != 0 {
		t.Fatalf("clearSelection() left stale state: %+v", struct {
			selecting              bool
			aLn, aCol, cLn, cCol int
		}{m.selecting, m.selAnchorLn, m.selAnchorCol, m.selCurLn, m.selCurCol})
	}
}
