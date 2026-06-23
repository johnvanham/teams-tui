package graph

import "testing"

func TestAllShortcodes(t *testing.T) {
	all := AllShortcodes()
	if len(all) == 0 {
		t.Fatal("AllShortcodes returned nothing")
	}

	// Sorted ascending by name and every entry populated.
	for i, e := range all {
		if e.Name == "" || e.Emoji == "" {
			t.Errorf("entry %d incomplete: %+v", i, e)
		}
		if i > 0 && all[i-1].Name > e.Name {
			t.Errorf("not sorted: %q before %q", all[i-1].Name, e.Name)
		}
	}

	// Deduplicated by glyph: each emoji appears at most once even though the
	// table has aliases (e.g. "+1" and "thumbsup" both map to 👍).
	seen := make(map[string]string)
	for _, e := range all {
		if prev, ok := seen[e.Emoji]; ok {
			t.Errorf("glyph %q duplicated (names %q and %q)", e.Emoji, prev, e.Name)
		}
		seen[e.Emoji] = e.Name
	}

	// The readable alias wins over a symbol/number alias for the same glyph.
	for _, e := range all {
		if e.Emoji == "👍" && e.Name != "thumbsup" {
			t.Errorf("👍 canonical name = %q, want thumbsup", e.Name)
		}
	}
}

func TestPreferredShortcodeName(t *testing.T) {
	cases := []struct {
		cand, cur string
		want      bool
	}{
		{"thumbsup", "+1", true},  // alphabetic beats symbol
		{"+1", "thumbsup", false}, // symbol loses to alphabetic
		{"smiley", "smile", true}, // longer alphabetic wins
		{"smile", "smiley", false},
		{"abc", "abd", true}, // alphabetical tiebreak on equal length
	}
	for _, c := range cases {
		if got := preferredShortcodeName(c.cand, c.cur); got != c.want {
			t.Errorf("preferredShortcodeName(%q,%q)=%v want %v", c.cand, c.cur, got, c.want)
		}
	}
}
