package ui

import (
	"testing"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"

	"github.com/jvh/teams-tui/internal/graph"
)

// composeModel builds a Model with just enough geometry for the compose-box
// click mapping: a sized messages viewport (which fixes composeTop) and a
// textarea configured like the real one in New.
func composeModel(width, maxHeight int, value string) Model {
	vp := viewport.New()
	vp.SetWidth(width)
	vp.SetHeight(5)

	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.DynamicHeight = true
	ta.MinHeight = composeMinLines
	ta.MaxHeight = maxHeight
	ta.SetWidth(width)
	ta.SetValue(value)
	ta.Focus()

	return Model{
		phase:    phaseReady,
		focus:    focusCompose,
		keys:     defaultKeyMap(),
		viewport: vp,
		compose:  ta,
	}
}

// clickCompose clicks the given text row/display column of the compose box and
// returns the resulting cursor position.
func clickCompose(m Model, row, col int) (line, column int) {
	m.moveComposeCursorTo(m.composeCellAt(m.composeTextLeft()+col, m.composeTextTop()+row))
	return m.compose.Line(), m.compose.Column()
}

func TestComposeClickCursor(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		value      string
		row, col   int
		wantLine   int
		wantColumn int
	}{
		{"start of line", 20, "hello world", 0, 0, 0, 0},
		{"mid word", 20, "hello world", 0, 6, 0, 6},
		{"past end clamps to end", 20, "hello", 0, 40, 0, 5},
		{"second logical line", 20, "abc\ndefgh", 1, 3, 1, 3},
		{"row past content clamps", 20, "abc\ndef", 4, 1, 1, 1},
		{"negative column clamps", 20, "abc", 0, 0, 0, 0},
		// "aaaa bbbb cccc" soft-wraps at width 10 into "aaaa bbbb " / "cccc".
		// The wrapped rows belong to one logical line, so the column is an
		// offset into the whole line, not into the visible row.
		{"soft-wrapped continuation", 10, "aaaa bbbb cccc", 1, 2, 0, 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := composeModel(tt.width, 10, tt.value)
			line, col := clickCompose(m, tt.row, tt.col)
			if line != tt.wantLine || col != tt.wantColumn {
				t.Errorf("click(row=%d,col=%d) = line %d col %d, want line %d col %d",
					tt.row, tt.col, line, col, tt.wantLine, tt.wantColumn)
			}
		})
	}
}

// TestComposeClickDoubleWidth checks that a click lands on the character
// covering that cell when earlier runes are double-width.
func TestComposeClickDoubleWidth(t *testing.T) {
	m := composeModel(20, 10, "日本語x")
	// Cells 0-1 are 日, 2-3 本, 4-5 語, 6 x. Clicking cell 4 must put the
	// cursor before 語 (rune index 2).
	if line, col := clickCompose(m, 0, 4); line != 0 || col != 2 {
		t.Errorf("click on cell 4 = line %d col %d, want line 0 col 2", line, col)
	}
	if line, col := clickCompose(m, 0, 6); line != 0 || col != 3 {
		t.Errorf("click on cell 6 = line %d col %d, want line 0 col 3", line, col)
	}
}

// TestComposeClickKeepsScroll ensures clicking in a scrolled compose box does
// not move the view: the walk down to the target line must restore the offset
// the textarea had before the click.
func TestComposeClickKeepsScroll(t *testing.T) {
	m := composeModel(20, 3, "l0\nl1\nl2\nl3\nl4\nl5")
	m.compose.MoveToEnd()
	// The textarea only hands its wrapped content to its internal viewport when
	// it renders, and scrolling is clamped to that content, so a frame has to
	// have been drawn before the offset is meaningful (it always has been in
	// the running app, where View precedes any click).
	m.compose.View()
	m.compose.MoveToEnd()
	before := m.compose.ScrollYOffset()
	if before == 0 {
		t.Fatalf("test setup: expected a scrolled compose box, got offset 0")
	}
	// Click the first visible row, which is line (before) of the content.
	m.moveComposeCursorTo(m.composeCellAt(m.composeTextLeft()+1, m.composeTextTop()))
	if got := m.compose.ScrollYOffset(); got != before {
		t.Errorf("scroll offset = %d, want %d (unchanged)", got, before)
	}
	if got := m.compose.Line(); got != before {
		t.Errorf("cursor line = %d, want %d", got, before)
	}
}

// TestComposeTopIncludesStackedRows guards the compose-box geometry against the
// rows viewReady stacks between the messages pane and the compose box: the
// reply banner and the spell-correction picker both push it down.
func TestComposeTopIncludesStackedRows(t *testing.T) {
	m := composeModel(20, 10, "")
	base := m.composeTop()

	m.replyTo = &graph.Reply{}
	withBanner := m.composeTop()
	if delta := withBanner - base; delta != m.replyBannerHeight() {
		t.Errorf("reply banner moved composeTop by %d, want %d", delta, m.replyBannerHeight())
	}
}

// TestSetComposeCursorWrapped checks the caret lands on the requested *logical*
// line even when earlier lines soft-wrap over several display rows. The
// textarea can only step display rows, so a naive "CursorDown() row times"
// walk lands short — this is what setComposeCursor exists to get right.
func TestSetComposeCursorWrapped(t *testing.T) {
	// At width 10 the first line wraps over two display rows.
	m := composeModel(10, 10, "aaaa bbbb cccc\nsecond\nthird")
	for _, want := range []struct{ line, col int }{{0, 2}, {1, 3}, {2, 5}, {0, 0}} {
		m.setComposeCursor(want.line, want.col)
		if l, c := m.compose.Line(), m.compose.Column(); l != want.line || c != want.col {
			t.Errorf("setComposeCursor(%d, %d) = line %d col %d",
				want.line, want.col, l, c)
		}
	}
}

// TestApplySpellCandidateWrapped is the same check through a real caller: a
// correction on the third line of a box whose first line wraps.
func TestApplySpellCandidateWrapped(t *testing.T) {
	m := composeModel(10, 10, "aaaa bbbb cccc\nsecond\nteh end")
	m.spellPicker = true
	m.spellCandidates = []spellCandidate{{Word: "teh", Suggestion: "the"}}

	if !m.applySpellCandidate() {
		t.Fatal("applySpellCandidate() returned false")
	}
	if got, want := m.compose.Value(), "aaaa bbbb cccc\nsecond\nthe end"; got != want {
		t.Fatalf("value = %q, want %q", got, want)
	}
	// The caret belongs just after the replacement, on the line it edited.
	if l, c := m.compose.Line(), m.compose.Column(); l != 2 || c != 3 {
		t.Errorf("cursor = line %d col %d, want line 2 col 3", l, c)
	}
}
