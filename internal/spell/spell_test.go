package spell

import (
	"errors"
	"reflect"
	"testing"
)

func TestParsePipeOutput(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []Misspelling
	}{
		{
			name: "banner and all correct",
			out:  "@(#) International Ispell Version 3.1.20\n*\n*\n\n",
			want: nil,
		},
		{
			name: "misspelled with suggestions",
			out:  "@(#) banner\n& teh 7 0: the, eh, tee, tea, ten, heh, t eh\n\n",
			want: []Misspelling{{
				Word:        "teh",
				Offset:      0,
				Suggestions: []string{"the", "eh", "tee", "tea", "ten", "heh", "t eh"},
			}},
		},
		{
			name: "mixed correct and misspelled with offsets",
			out:  "@(#) banner\n& teh 2 0: the, tea\n*\n& recieve 1 8: receive\n\n",
			want: []Misspelling{
				{Word: "teh", Offset: 0, Suggestions: []string{"the", "tea"}},
				{Word: "recieve", Offset: 8, Suggestions: []string{"receive"}},
			},
		},
		{
			name: "no-suggestion hash line",
			out:  "@(#) banner\n# xyzzyq 3\n\n",
			want: []Misspelling{{Word: "xyzzyq", Offset: 3}},
		},
		{
			name: "question-mark guessed root treated as misspelled",
			out:  "@(#) banner\n? runnable 1 0: running\n\n",
			want: []Misspelling{{Word: "runnable", Offset: 0, Suggestions: []string{"running"}}},
		},
		{
			name: "root/compound markers ignored",
			out:  "@(#) banner\n+ root\n- compound\n\n",
			want: nil,
		},
		{
			name: "malformed lines skipped",
			out:  "@(#) banner\n& teh\n& good 1 x: g\n\n",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePipeOutput([]byte(tt.out), 0)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parsePipeOutput() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParsePipeOutputOffsetAdjust(t *testing.T) {
	// The "^" escape shifts reported offsets right by one; offsetAdjust=1
	// should restore the true position within the line.
	out := "@(#) banner\n& teh 1 1: the\n\n"
	got := parsePipeOutput([]byte(out), 1)
	want := []Misspelling{{Word: "teh", Offset: 0, Suggestions: []string{"the"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsePipeOutput(adjust=1) = %#v, want %#v", got, want)
	}
}

func TestNewPrefersEnchant(t *testing.T) {
	orig := lookPath
	defer func() { lookPath = orig }()

	lookPath = func(name string) (string, error) {
		switch name {
		case "enchant-2":
			return "/usr/bin/enchant-2", nil
		case "hunspell":
			return "/usr/bin/hunspell", nil
		}
		return "", errors.New("not found")
	}

	c, ok := New("en_US")
	if !ok || !c.Available() {
		t.Fatal("expected an available checker")
	}
	if c.path != "/usr/bin/enchant-2" {
		t.Errorf("path = %q, want enchant-2 (preferred)", c.path)
	}
	if got, want := c.args, []string{"-a", "-d", "en_US"}; !reflect.DeepEqual(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
}

func TestNewFallsBackToHunspell(t *testing.T) {
	orig := lookPath
	defer func() { lookPath = orig }()

	lookPath = func(name string) (string, error) {
		if name == "hunspell" {
			return "/usr/bin/hunspell", nil
		}
		return "", errors.New("not found")
	}

	c, ok := New("")
	if !ok || c.path != "/usr/bin/hunspell" {
		t.Fatalf("expected hunspell fallback, got ok=%v path=%q", ok, c.path)
	}
	if got, want := c.args, []string{"-a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("args = %v, want %v (no -d when lang empty)", got, want)
	}
}

func TestNewNoHelperDisabled(t *testing.T) {
	orig := lookPath
	defer func() { lookPath = orig }()

	lookPath = func(string) (string, error) { return "", errors.New("not found") }

	c, ok := New("en_US")
	if ok {
		t.Error("expected ok=false when no helper installed")
	}
	if c.Available() {
		t.Error("expected checker to be unavailable")
	}
	// An unavailable checker must be safe to call and return nothing.
	if got := c.Check("teh recieve"); got != nil {
		t.Errorf("Check on unavailable checker = %v, want nil", got)
	}
}

func TestCheckEmptyLine(t *testing.T) {
	// Even with a resolved path, blank/whitespace lines short-circuit without
	// spawning a subprocess.
	c := &Checker{path: "/usr/bin/enchant-2", args: []string{"-a"}}
	if got := c.Check("   "); got != nil {
		t.Errorf("Check(whitespace) = %v, want nil", got)
	}
}

func TestNilCheckerSafe(t *testing.T) {
	var c *Checker
	if c.Available() {
		t.Error("nil checker should be unavailable")
	}
	if got := c.Check("teh"); got != nil {
		t.Errorf("nil checker Check = %v, want nil", got)
	}
	if got := c.CheckText("teh recieve"); got != nil {
		t.Errorf("nil checker CheckText = %v, want nil", got)
	}
}

// TestCheckTextIntegration exercises the real system helper when one is
// installed; it's skipped in environments without enchant-2/hunspell.
func TestCheckTextIntegration(t *testing.T) {
	c, ok := New("en_US")
	if !ok {
		t.Skip("no system spell helper (enchant-2/hunspell) available")
	}
	got := c.CheckText("teh quick recieve\nbrown fox")
	words := map[string]bool{}
	for _, m := range got {
		words[m.Word] = true
	}
	if !words["teh"] || !words["recieve"] {
		t.Errorf("expected teh and recieve flagged, got %#v", got)
	}
	if words["quick"] || words["brown"] || words["fox"] {
		t.Errorf("correct words should not be flagged, got %#v", got)
	}
}

func TestCheckTextUnavailableAndEmpty(t *testing.T) {
	// Unavailable checker returns nothing without spawning a process.
	var unavailable Checker
	if got := unavailable.CheckText("teh"); got != nil {
		t.Errorf("unavailable CheckText = %v, want nil", got)
	}
	// Available checker with only-whitespace text short-circuits too.
	c := &Checker{path: "/usr/bin/enchant-2", args: []string{"-a"}}
	if got := c.CheckText("\n  \n"); got != nil {
		t.Errorf("CheckText(whitespace) = %v, want nil", got)
	}
}
