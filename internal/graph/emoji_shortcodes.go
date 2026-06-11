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

// shortcodeEmoji maps :name: tokens to Unicode emoji. Names follow the common
// GitHub/Slack/Teams convention so muscle memory carries over. Aliases point at
// the same glyph (e.g. "+1" and "thumbsup").
var shortcodeEmoji = map[string]string{
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
		name := strings.ToLower(tok[1 : len(tok)-1])
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
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return nil
	}
	var out []EmojiShortcode
	for name, glyph := range shortcodeEmoji {
		if strings.HasPrefix(name, prefix) {
			out = append(out, EmojiShortcode{Name: name, Emoji: glyph})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		// Exact match wins, then shorter names, then alphabetical.
		ei, ej := out[i].Name == prefix, out[j].Name == prefix
		if ei != ej {
			return ei
		}
		if len(out[i].Name) != len(out[j].Name) {
			return len(out[i].Name) < len(out[j].Name)
		}
		return out[i].Name < out[j].Name
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// MatchEmoticonSuffix reports whether text ends with a recognized ASCII
// emoticon (e.g. ":-)" or "<3") and, if so, returns the emoticon's Unicode
// glyph and the byte length of the matched emoticon. It is used for live,
// as-you-type conversion in the composer: a caller checks the text up to the
// cursor after each keystroke and, on a match, replaces the trailing emoticon
// with the glyph.
//
// To avoid mangling words and URLs (e.g. "http://"), a match only counts when
// the character immediately before the emoticon is whitespace or the start of
// the text. The longest emoticon wins (emoticonKeys is longest-first).
func MatchEmoticonSuffix(text string) (glyph string, matchLen int, ok bool) {
	for _, k := range emoticonKeys {
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
