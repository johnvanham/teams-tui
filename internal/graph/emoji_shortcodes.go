package graph

import (
	"regexp"
	"sort"
	"strings"
)

// Emoji shortcode support mirrors the conveniences of the desktop Teams client:
// users can type a colon-delimited name (:thumbsup:) or a classic text
// emoticon (:-)) and have it rendered as the Unicode emoji. Conversion happens
// at send time (ReplaceShortcodes); the autocomplete popup queries the same
// table via MatchShortcodePrefix while the user is still typing.

// EmojiShortcode pairs a canonical colon-name with its Unicode glyph. It is the
// item type surfaced to the UI's emoji picker.
type EmojiShortcode struct {
	Name  string // canonical name without the surrounding colons, e.g. "thumbsup"
	Emoji string // Unicode character, e.g. "👍"
}

// curatedShortcodes maps a small set of convenient :name: tokens to Unicode
// emoji. These are hand-picked aliases and Teams-flavoured names (e.g. "party",
// "+1") that sit on top of the full generated gemojiTable. Where a name here
// also exists in gemoji, this entry wins, so the readable/curated glyph is kept.
var curatedShortcodes = map[string]string{
	"thumbsup":         "👍",
	"+1":               "👍",
	"thumbsdown":       "👎",
	"-1":               "👎",
	"heart":            "❤️",
	"yellow_heart":     "💛",
	"broken_heart":     "💔",
	"smile":            "😄",
	"smiley":           "😃",
	"grin":             "😁",
	"laughing":         "😆",
	"joy":              "😂",
	"rofl":             "🤣",
	"slightly_smiling": "🙂",
	"wink":             "😉",
	"blush":            "😊",
	"heart_eyes":       "😍",
	"kissing_heart":    "😘",
	"thinking":         "🤔",
	"neutral_face":     "😐",
	"expressionless":   "😑",
	"unamused":         "😒",
	"smirk":            "😏",
	"sweat_smile":      "😅",
	"cry":              "😢",
	"sob":              "😭",
	"disappointed":     "😞",
	"pensive":          "😔",
	"confused":         "😕",
	"worried":          "😟",
	"frowning":         "🙁",
	"open_mouth":       "😮",
	"astonished":       "😲",
	"flushed":          "😳",
	"scream":           "😱",
	"angry":            "😠",
	"rage":             "😡",
	"sleeping":         "😴",
	"sunglasses":       "😎",
	"nerd":             "🤓",
	"shush":            "🤫",
	"zipper_mouth":     "🤐",
	"vomiting":         "🤮",
	"exploding_head":   "🤯",
	"cowboy":           "🤠",
	"clown":            "🤡",
	"poop":             "💩",
	"ghost":            "👻",
	"alien":            "👽",
	"robot":            "🤖",
	"wave":             "👋",
	"raised_hand":      "✋",
	"ok_hand":          "👌",
	"pinch":            "🤏",
	"v":                "✌️",
	"crossed_fingers":  "🤞",
	"point_up":         "☝️",
	"point_down":       "👇",
	"point_left":       "👈",
	"point_right":      "👉",
	"clap":             "👏",
	"raised_hands":     "🙌",
	"pray":             "🙏",
	"muscle":           "💪",
	"fire":             "🔥",
	"sparkles":         "✨",
	"star":             "⭐",
	"star2":            "🌟",
	"zap":              "⚡",
	"boom":             "💥",
	"100":              "💯",
	"tada":             "🎉",
	"party":            "🎉",
	"confetti":         "🎊",
	"gift":             "🎁",
	"balloon":          "🎈",
	"check":            "✅",
	"white_check_mark": "✅",
	"heavy_check_mark": "✔️",
	"x":                "❌",
	"cross":            "❌",
	"warning":          "⚠️",
	"question":         "❓",
	"exclamation":      "❗",
	"bulb":             "💡",
	"bell":             "🔔",
	"lock":             "🔒",
	"key":              "🔑",
	"hourglass":        "⏳",
	"alarm_clock":      "⏰",
	"watch":            "⌚",
	"calendar":         "📅",
	"pushpin":          "📌",
	"paperclip":        "📎",
	"memo":             "📝",
	"pencil":           "✏️",
	"book":             "📖",
	"books":            "📚",
	"email":            "📧",
	"phone":            "📞",
	"computer":         "💻",
	"keyboard":         "⌨️",
	"bug":              "🐛",
	"rocket":           "🚀",
	"airplane":         "✈️",
	"car":              "🚗",
	"coffee":           "☕",
	"beer":             "🍺",
	"beers":            "🍻",
	"cake":             "🍰",
	"birthday":         "🎂",
	"pizza":            "🍕",
	"hamburger":        "🍔",
	"apple":            "🍎",
	"banana":           "🍌",
	"eyes":             "👀",
	"see_no_evil":      "🙈",
	"hear_no_evil":     "🙉",
	"speak_no_evil":    "🙊",
	"dog":              "🐶",
	"cat":              "🐱",
	"unicorn":          "🦄",
	"snowman":          "⛄",
	"snowflake":        "❄️",
	"sunny":            "☀️",
	"cloud":            "☁️",
	"rainbow":          "🌈",
	"earth":            "🌍",
	"moon":             "🌙",
	"sun":              "🌞",
	"ok":               "🆗",
	"new":              "🆕",
	"up":               "⬆️",
	"down":             "⬇️",
	"left":             "⬅️",
	"right":            "➡️",
	"recycle":          "♻️",
	"musical_note":     "🎵",
	"trophy":           "🏆",
	"medal":            "🏅",
	"soccer":           "⚽",
	"basketball":       "🏀",
	"money":            "💰",
	"dollar":           "💵",
	"hammer":           "🔨",
	"wrench":           "🔧",
	"gear":             "⚙️",
	"link":             "🔗",
	"mag":              "🔍",
	"hand_wave":        "👋",
	"facepalm":         "🤦",
	"shrug":            "🤷",
}

// shortcodeEntry is a display name paired with its glyph, used to surface a
// readable :name: in autocomplete/browser results while lookups go through the
// normalized index.
type shortcodeEntry struct {
	Name  string // display name, e.g. "nauseated_face" or "thumbsup"
	Emoji string
}

// shortcodeEmoji is the lookup index: a normalized shortcode name (lowercased
// with '_' and '-' separators stripped) → glyph. Normalization lets the same
// emoji resolve whether the user types the standard ":nauseated_face:" or the
// Teams ":nauseatedface:" form. Built once by buildShortcodeIndex.
var shortcodeEmoji = buildShortcodeIndex()

// shortcodeNames is the de-duplicated set of display names (one per glyph,
// preferring a readable alias) backing AllShortcodes and prefix matching. Built
// alongside shortcodeEmoji.
var shortcodeNames = buildShortcodeNames()

// normalizeShortcode folds a shortcode name to its lookup key: lowercase with
// '_' and '-' removed. So "Nauseated_Face", "nauseated-face" and "nauseatedface"
// all map to the same key, matching how Teams writes shortcodes without
// separators while still accepting the canonical underscore form.
func normalizeShortcode(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range strings.ToLower(name) {
		if r == '_' || r == '-' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// buildShortcodeIndex assembles the normalized lookup map from the full gemoji
// table plus the curated aliases. Curated entries are applied last so they win
// on collisions (keeping the hand-picked glyph/name).
func buildShortcodeIndex() map[string]string {
	idx := make(map[string]string, len(gemojiTable)*2+len(curatedShortcodes))
	for _, e := range gemojiTable {
		for _, alias := range e.Aliases {
			idx[normalizeShortcode(alias)] = e.Glyph
		}
	}
	for name, glyph := range curatedShortcodes {
		idx[normalizeShortcode(name)] = glyph
	}
	return idx
}

// buildShortcodeNames produces one readable display name per glyph for the
// browser/autocomplete. It walks gemoji (first alias = canonical name), then
// lets a curated name override when it is "nicer" (preferredShortcodeName), so
// e.g. 👍 shows as "thumbsup" rather than gemoji's "+1".
func buildShortcodeNames() []shortcodeEntry {
	best := make(map[string]string) // glyph -> chosen display name
	consider := func(name, glyph string) {
		cur, ok := best[glyph]
		if !ok || preferredShortcodeName(name, cur) {
			best[glyph] = name
		}
	}
	for _, e := range gemojiTable {
		consider(e.Aliases[0], e.Glyph)
	}
	for name, glyph := range curatedShortcodes {
		consider(name, glyph)
	}
	out := make([]shortcodeEntry, 0, len(best))
	for glyph, name := range best {
		out = append(out, shortcodeEntry{Name: name, Emoji: glyph})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// emoticonEmoji maps classic ASCII text emoticons to Unicode emoji. These are
// matched and replaced automatically at send time (e.g. ":-)" -> "🙂"), like
// the desktop client's auto-replace. Longer keys are tried before shorter ones
// (see emoticonReplacer) so ":-)" wins over a hypothetical ":)".
var emoticonEmoji = map[string]string{
	":-)":  "🙂",
	":)":   "🙂",
	":-D":  "😃",
	":D":   "😃",
	";-)":  "😉",
	";)":   "😉",
	":-(":  "🙁",
	":(":   "🙁",
	":-P":  "😛",
	":P":   "😛",
	":-p":  "😛",
	":p":   "😛",
	":-O":  "😮",
	":O":   "😮",
	":-o":  "😮",
	":o":   "😮",
	":'(":  "😢",
	"<3":   "❤️",
	"</3":  "💔",
	":-|":  "😐",
	":|":   "😐",
	":-\\": "😕",
	":\\":  "😕",
	"8-)":  "😎",
	"B-)":  "😎",
	">:(":  "😠",
	":-*":  "😘",
	":*":   "😘",
	"\\o/": "🙌",
}

// shortcodeRe matches a :name: token: a colon, one or more name characters
// (letters, digits, underscore, plus, minus), then a closing colon. We require
// at least one inner character so a bare "::" is left alone.
var shortcodeRe = regexp.MustCompile(`:([a-zA-Z0-9_+-]+):`)

// emoticonKeys lists every emoticon, longest first (ties lexical), so callers
// that try matches in order resolve overlaps (":-)" before ":)") to the longer
// emoticon.
var emoticonKeys = sortedEmoticonKeys()

// emoticonReplacer is built once from emoticonEmoji with the longest emoticons
// first, so overlapping prefixes (":-)" vs ":)") resolve to the longer match.
var emoticonReplacer = buildEmoticonReplacer()

func sortedEmoticonKeys() []string {
	keys := make([]string, 0, len(emoticonEmoji))
	for k := range emoticonEmoji {
		keys = append(keys, k)
	}
	// Longer first; ties broken lexically for determinism.
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	return keys
}

func buildEmoticonReplacer() *strings.Replacer {
	pairs := make([]string, 0, len(emoticonKeys)*2)
	for _, k := range emoticonKeys {
		pairs = append(pairs, k, emoticonEmoji[k])
	}
	return strings.NewReplacer(pairs...)
}

// ReplaceShortcodes converts emoji shortcodes (:name:) and text emoticons (:-))
// in s to their Unicode characters. Unknown :name: tokens are left untouched so
// non-emoji uses of colons (timestamps, ratios, URLs) survive. It is safe to
// call on already-converted or emoji-free text.
func ReplaceShortcodes(s string) string {
	// :name: tokens first, only substituting known names.
	s = shortcodeRe.ReplaceAllStringFunc(s, func(tok string) string {
		name := normalizeShortcode(tok[1 : len(tok)-1])
		if glyph, ok := shortcodeEmoji[name]; ok {
			return glyph
		}
		return tok
	})
	// Then classic emoticons.
	return emoticonReplacer.Replace(s)
}

// MatchShortcodePrefix returns emoji whose shortcode name begins with prefix
// (case-insensitive), for the autocomplete popup. Results are sorted with exact
// and shorter names first, then alphabetically, and capped at limit (limit <= 0
// means no cap). The leading ':' should be stripped by the caller.
func MatchShortcodePrefix(prefix string, limit int) []EmojiShortcode {
	norm := normalizeShortcode(strings.TrimSpace(prefix))
	if norm == "" {
		return nil
	}
	var out []EmojiShortcode
	for _, e := range shortcodeNames {
		// Compare on the normalized name so ":nauseated" and ":nauseatedface"
		// both match the "nauseated_face" entry.
		if strings.HasPrefix(normalizeShortcode(e.Name), norm) {
			out = append(out, EmojiShortcode{Name: e.Name, Emoji: e.Emoji})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		// Exact (normalized) match wins, then shorter names, then alphabetical.
		ni, nj := normalizeShortcode(out[i].Name), normalizeShortcode(out[j].Name)
		ei, ej := ni == norm, nj == norm
		if ei != ej {
			return ei
		}
		if len(ni) != len(nj) {
			return len(ni) < len(nj)
		}
		return out[i].Name < out[j].Name
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// AllShortcodes returns the full emoji table as a sorted list, deduplicated by
// glyph so each emoji appears once. When several shortcodes alias the same glyph
// (e.g. "+1" and "thumbsup" → 👍), the most readable name wins: a longer,
// purely alphabetic name is preferred over a short or symbol-bearing alias.
// Results are sorted alphabetically by name. It backs the full emoji browser,
// which lists every emoji and lets the user filter interactively.
func AllShortcodes() []EmojiShortcode {
	out := make([]EmojiShortcode, len(shortcodeNames))
	for i, e := range shortcodeNames {
		out[i] = EmojiShortcode{Name: e.Name, Emoji: e.Emoji}
	}
	return out
}

// preferredShortcodeName reports whether candidate is a "nicer" canonical name
// than current for the same glyph: alphabetic names beat ones containing digits
// or symbols (so "thumbsup" beats "+1"); among equally-alphabetic names the
// longer one wins (more descriptive), with alphabetical order as a tiebreak.
func preferredShortcodeName(candidate, current string) bool {
	ca, cu := isAlphaName(candidate), isAlphaName(current)
	if ca != cu {
		return ca
	}
	if len(candidate) != len(current) {
		return len(candidate) > len(current)
	}
	return candidate < current
}

// isAlphaName reports whether name consists solely of a–z letters (no digits,
// '+', '-' or '_'), marking it as a clean, readable shortcode.
func isAlphaName(name string) bool {
	for _, r := range name {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return name != ""
}

// MatchEmoticonSuffix reports whether text ends with a recognized ASCII
// emoticon (e.g. ":-)" or "<3") and, if so, returns the emoticon's Unicode
// glyph and the byte length of the matched emoticon. It is used for live,
// as-you-type conversion in the composer: a caller checks the text up to the
// cursor after each keystroke and, on a match, replaces the trailing emoticon
// with the glyph.
//
// Emoticons that begin with ':' (":)", ":p", ":D", …) are intentionally NOT
// matched here: they collide with the start of :shortcode: tokens, so replacing
// ":p" the instant it is typed would make ":party" impossible. Those convert
// only at a word boundary via MatchEmoticonBeforeBoundary. Non-colon emoticons
// ("<3", "8-)", "\\o/", ">:(") are safe to convert immediately.
//
// To avoid mangling words and URLs (e.g. "http://"), a match only counts when
// the character immediately before the emoticon is whitespace or the start of
// the text. The longest emoticon wins (emoticonKeys is longest-first).
func MatchEmoticonSuffix(text string) (glyph string, matchLen int, ok bool) {
	for _, k := range emoticonKeys {
		if strings.HasPrefix(k, ":") {
			continue // colon-led emoticons defer to MatchEmoticonBeforeBoundary
		}
		if !strings.HasSuffix(text, k) {
			continue
		}
		before := text[:len(text)-len(k)]
		if before == "" || endsWithSpace(before) {
			return emoticonEmoji[k], len(k), true
		}
	}
	return "", 0, false
}

// MatchEmoticonBeforeBoundary reports whether the text immediately preceding the
// cursor ends in a colon-led emoticon (":)", ":p", ":-D", …). It is called when
// the user types a word boundary (space or newline), at which point the
// preceding token is final and safe to convert — this is how ":p" becomes 😛
// without preventing ":party" from being typed first. The returned matchLen is
// the byte length of the emoticon (not including the boundary the caller just
// inserted). A match requires the emoticon to be preceded by whitespace or the
// start of text, mirroring MatchEmoticonSuffix.
func MatchEmoticonBeforeBoundary(text string) (glyph string, matchLen int, ok bool) {
	for _, k := range emoticonKeys {
		if !strings.HasPrefix(k, ":") {
			continue // non-colon emoticons already convert as you type
		}
		if !strings.HasSuffix(text, k) {
			continue
		}
		before := text[:len(text)-len(k)]
		if before == "" || endsWithSpace(before) {
			return emoticonEmoji[k], len(k), true
		}
	}
	return "", 0, false
}

// endsWithSpace reports whether s ends in a space or tab.
func endsWithSpace(s string) bool {
	if s == "" {
		return false
	}
	c := s[len(s)-1]
	return c == ' ' || c == '\t' || c == '\n'
}
