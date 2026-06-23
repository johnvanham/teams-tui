package ui

import "testing"

func TestEmojiBrowserOpenListsAll(t *testing.T) {
	m := newComposeModel("")
	m.openEmojiBrowser()
	if !m.emojiBrowser {
		t.Fatal("browser should be open")
	}
	if len(m.browserMatches) == 0 {
		t.Fatal("empty query should list every emoji")
	}
	if m.browserSel != 0 {
		t.Errorf("selection should start at 0, got %d", m.browserSel)
	}
}

func TestEmojiBrowserFilters(t *testing.T) {
	m := newComposeModel("")
	m.openEmojiBrowser()
	all := len(m.browserMatches)

	m.browserQuery = "thumbs"
	m.refreshBrowserMatches()
	if len(m.browserMatches) == 0 {
		t.Fatal("query 'thumbs' should match at least one emoji")
	}
	if len(m.browserMatches) >= all {
		t.Errorf("filtered list (%d) should be smaller than full list (%d)", len(m.browserMatches), all)
	}
	for _, e := range m.browserMatches {
		if len(e.Name) < 6 || e.Name[:6] != "thumbs" {
			t.Errorf("match %q does not start with prefix", e.Name)
		}
	}
}

func TestEmojiBrowserMoveWraps(t *testing.T) {
	m := newComposeModel("")
	m.emojiBrowser = true
	m.browserQuery = "thumbsup"
	m.refreshBrowserMatches()
	n := len(m.browserMatches)
	if n == 0 {
		t.Fatal("expected matches for 'thumbsup'")
	}
	m.browserMove(-1) // from 0 wraps to last
	if m.browserSel != n-1 {
		t.Errorf("move(-1) sel = %d, want %d", m.browserSel, n-1)
	}
	m.browserMove(1) // back to 0
	if m.browserSel != 0 {
		t.Errorf("move(1) sel = %d, want 0", m.browserSel)
	}
}

func TestEmojiBrowserWindowKeepsSelectionVisible(t *testing.T) {
	m := newComposeModel("")
	m.openEmojiBrowser() // many matches
	if len(m.browserMatches) <= emojiBrowserMax {
		t.Skip("not enough emoji to exercise scrolling")
	}
	// Selection near the end: it must appear within the returned window.
	m.browserSel = len(m.browserMatches) - 1
	win, sel := m.browserWindow()
	if len(win) != emojiBrowserMax {
		t.Errorf("window size = %d, want %d", len(win), emojiBrowserMax)
	}
	if sel < 0 || sel >= len(win) {
		t.Errorf("selection index %d out of window bounds %d", sel, len(win))
	}
	if win[sel].Name != m.browserMatches[m.browserSel].Name {
		t.Errorf("windowed selection %q != actual %q", win[sel].Name, m.browserMatches[m.browserSel].Name)
	}
}

func TestApplyBrowserSelectionInsertsAtCursor(t *testing.T) {
	m := newComposeModel("hello world")
	// Move cursor to just after "hello" (column 5).
	m.compose.MoveToBegin()
	m.compose.SetCursorColumn(5)

	m.emojiBrowser = true
	m.browserQuery = "thumbsup"
	m.refreshBrowserMatches()
	if len(m.browserMatches) == 0 {
		t.Fatal("expected a thumbsup match")
	}
	glyph := m.browserMatches[m.browserSel].Emoji

	if !m.applyBrowserSelection() {
		t.Fatal("applyBrowserSelection returned false")
	}
	want := "hello" + glyph + " world"
	if got := m.compose.Value(); got != want {
		t.Errorf("compose value = %q, want %q", got, want)
	}
	if m.emojiBrowser {
		t.Error("browser should close after applying a selection")
	}
}
