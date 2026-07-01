// Package spell provides spell checking for the compose box by shelling out to
// the system spell checker. On GNOME/Fedora the canonical entry point is
// enchant-2 (the abstraction layer GTK/gspell apps use, backed by hunspell),
// so we prefer it and fall back to hunspell directly. Both speak the classic
// ispell "pipe" protocol (`-a`), which reports misspelled words together with
// suggested corrections — exactly what the UI needs.
//
// Like internal/clipboard and internal/open, this is a thin subprocess wrapper:
// the terminal can't give us a spell-check API, so we pipe the text to the
// helper and parse its stdout. When no helper is installed the checker is
// simply disabled (Available() reports false) so the feature degrades quietly
// instead of erroring.
package spell

import (
	"bufio"
	"bytes"
	"os/exec"
	"strconv"
	"strings"
)

// Misspelling is a single flagged word plus its position in the checked text
// and the checker's suggested corrections (may be empty).
type Misspelling struct {
	// Word is the token the checker flagged as misspelled.
	Word string
	// Offset is the 0-based rune offset of Word within the checked line, as
	// reported by the ispell protocol. (We check text one line at a time.)
	Offset int
	// Suggestions are candidate corrections, best first (may be empty).
	Suggestions []string
}

// Checker runs spell checks against a configured system helper. A nil or
// unavailable Checker reports no misspellings, so callers can use it
// unconditionally.
type Checker struct {
	// path is the resolved helper executable ("" when none is available).
	path string
	// args are the fixed arguments passed on every invocation (pipe mode plus,
	// for hunspell, the dictionary selection).
	args []string
}

// candidate describes a helper we know how to drive, in preference order.
type candidate struct {
	name string
	// args builds the argument list for the given language ("" = helper's
	// default dictionary).
	args func(lang string) []string
}

// candidates lists the supported helpers, most preferred first. enchant-2 is
// the GNOME/gspell abstraction (honours the user's configured dictionaries and
// personal word list); hunspell is the direct fallback.
var candidates = []candidate{
	{
		name: "enchant-2",
		args: func(lang string) []string {
			a := []string{"-a"}
			if lang != "" {
				a = append(a, "-d", lang)
			}
			return a
		},
	},
	{
		name: "hunspell",
		args: func(lang string) []string {
			a := []string{"-a"}
			if lang != "" {
				a = append(a, "-d", lang)
			}
			return a
		},
	},
}

// lookPath is indirected so tests can stub executable resolution.
var lookPath = exec.LookPath

// New resolves an available spell helper for the given language (e.g. "en_US";
// "" uses the helper's default dictionary). It returns a disabled Checker (and
// ok=false) when no supported helper is installed, which is not an error: the
// caller should treat spell checking as unavailable.
func New(lang string) (*Checker, bool) {
	for _, c := range candidates {
		if path, err := lookPath(c.name); err == nil {
			return &Checker{path: path, args: c.args(lang)}, true
		}
	}
	return &Checker{}, false
}

// Available reports whether a spell helper was resolved. A zero-value or nil
// Checker is unavailable.
func (c *Checker) Available() bool {
	return c != nil && c.path != ""
}

// Check spell-checks a single line of text and returns the misspelled words it
// found. An empty/whitespace line, an unavailable checker, or any subprocess
// failure yields no misspellings (and no error surfaced to the UI): spell
// checking is best-effort and must never disrupt composing.
//
// We feed one line at a time so the reported offsets map directly onto that
// line, and prefix it with "^" — the ispell convention that disables the
// helper's command interpretation, so a line starting with '*', '&', '!', etc.
// is treated as literal text to check.
func (c *Checker) Check(line string) []Misspelling {
	if !c.Available() {
		return nil
	}
	if strings.TrimSpace(line) == "" {
		return nil
	}

	cmd := exec.Command(c.path, c.args...)
	cmd.Stdin = strings.NewReader("^" + line + "\n")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	// The "^" escape shifts every reported offset right by one; undo it.
	return parsePipeOutput(out, 1)
}

// CheckText spell-checks multi-line text and returns the distinct misspelled
// words in reading order (first occurrence wins). It's the convenience the
// compose box wants: the suggestions strip only needs the set of bad words and
// their corrections, not per-line offsets. Runs a single subprocess for the
// whole buffer.
func (c *Checker) CheckText(text string) []Misspelling {
	if !c.Available() || strings.TrimSpace(text) == "" {
		return nil
	}

	// Feed every non-empty line, each "^"-escaped so leading punctuation isn't
	// treated as an ispell command. enchant emits reports per input line; since
	// we dedupe by word, the collapsed offsets don't matter here.
	var in strings.Builder
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		in.WriteString("^")
		in.WriteString(line)
		in.WriteString("\n")
	}
	if in.Len() == 0 {
		return nil
	}

	cmd := exec.Command(c.path, c.args...)
	cmd.Stdin = strings.NewReader(in.String())
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	seen := make(map[string]bool)
	var res []Misspelling
	for _, m := range parsePipeOutput(out, 1) {
		if seen[m.Word] {
			continue
		}
		seen[m.Word] = true
		res = append(res, m)
	}
	return res
}

// parsePipeOutput parses ispell/enchant "-a" pipe output into misspellings.
// offsetAdjust is subtracted from each reported offset (to undo the leading
// "^" escape). The format, one report line per token:
//
//	@(#) ... version banner (ignored)
//	*                     -> token correct
//	& word N off: a, b    -> misspelled, N suggestions at char offset off
//	? word N off: a, b    -> misspelled (guessed root), suggestions
//	# word off            -> misspelled, no suggestions
//	(blank line)          -> end of the input line's reports
func parsePipeOutput(out []byte, offsetAdjust int) []Misspelling {
	var res []Misspelling
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '*' || line[0] == '+' || line[0] == '-' {
			continue // blank, correct, root-affix match, or compound
		}
		if strings.HasPrefix(line, "@(#)") {
			continue // version banner
		}
		switch line[0] {
		case '&', '?':
			if m, ok := parseSuggestionLine(line, offsetAdjust); ok {
				res = append(res, m)
			}
		case '#':
			if m, ok := parseNoSuggestionLine(line, offsetAdjust); ok {
				res = append(res, m)
			}
		}
	}
	return res
}

// parseSuggestionLine parses a "& word N off: a, b, c" (or "? …") line.
func parseSuggestionLine(line string, offsetAdjust int) (Misspelling, bool) {
	// Split off the suggestions after the first ": ".
	head, tail, hasSug := strings.Cut(line, ": ")
	fields := strings.Fields(head) // ["&", word, count, off]
	if len(fields) < 4 {
		return Misspelling{}, false
	}
	off, err := strconv.Atoi(fields[3])
	if err != nil {
		return Misspelling{}, false
	}
	m := Misspelling{Word: fields[1], Offset: off - offsetAdjust}
	if hasSug {
		for _, s := range strings.Split(tail, ", ") {
			if s = strings.TrimSpace(s); s != "" {
				m.Suggestions = append(m.Suggestions, s)
			}
		}
	}
	return m, true
}

// parseNoSuggestionLine parses a "# word off" line (misspelled, no guesses).
func parseNoSuggestionLine(line string, offsetAdjust int) (Misspelling, bool) {
	fields := strings.Fields(line) // ["#", word, off]
	if len(fields) < 3 {
		return Misspelling{}, false
	}
	off, err := strconv.Atoi(fields[2])
	if err != nil {
		return Misspelling{}, false
	}
	return Misspelling{Word: fields[1], Offset: off - offsetAdjust}, true
}
