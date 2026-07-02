// Package styles centralizes lipgloss v2 styling for the TUI.
package styles

import "charm.land/lipgloss/v2"

// Teams-ish purple palette.
var (
	Purple    = lipgloss.Color("#6264A7")
	PurpleLt  = lipgloss.Color("#8B8CC7")
	Grey      = lipgloss.Color("#6E6E6E")
	LightGrey = lipgloss.Color("#9E9E9E")
	White     = lipgloss.Color("#FFFFFF")
	Green     = lipgloss.Color("#13A10E")
	Red       = lipgloss.Color("#C50F1F")
	Yellow    = lipgloss.Color("#FFB900")
	Bg        = lipgloss.Color("#1F1F2E")
	// Orange is the bright warm accent used to mark chats with unread messages
	// (OpenCode's "warning" orange). It reads clearly against the dark sidebar
	// while staying distinct from the pink used for the selected chat.
	Orange = lipgloss.Color("#F5A742")
)

var (
	// App is the outer container.
	App = lipgloss.NewStyle()

	// Title bar at the top of the app.
	TitleBar = lipgloss.NewStyle().
			Foreground(White).
			Background(Purple).
			Bold(true).
			Padding(0, 1)

	// StatusBar at the bottom.
	StatusBar = lipgloss.NewStyle().
			Foreground(LightGrey).
			Padding(0, 1)

	// Sidebar holds the chat list; focused vs unfocused borders.
	SidebarFocused = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Purple).
			Padding(0, 1)

	SidebarBlurred = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Grey).
			Padding(0, 1)

	// MessagePane holds the conversation viewport.
	MessagePaneFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(Purple).
				Padding(0, 1)

	MessagePaneBlurred = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(Grey).
				Padding(0, 1)

	// Compose box for typing messages.
	ComposeFocused = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Purple).
			Padding(0, 1)

	ComposeBlurred = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Grey).
			Padding(0, 1)

	// SidebarHeader is the "Chats" header above the chat list.
	SidebarHeader = lipgloss.NewStyle().
			Foreground(White).
			Background(Purple).
			Bold(true).
			Padding(0, 1)

	// PopupBox frames a centered popup such as the status picker.
	PopupBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Purple).
			Padding(1, 2)

	// ParticipantsBar shows chat participants and presence as a header row
	// inside the messages pane, styled like the sidebar's "Chats" header
	// (purple background, light text) so the two columns line up.
	ParticipantsBar = lipgloss.NewStyle().
			Foreground(White).
			Background(Purple).
			Padding(0, 1)

	// SenderName for a message author.
	SenderName = lipgloss.NewStyle().Foreground(PurpleLt).Bold(true)

	// SenderSelf highlights the current user's own messages.
	SenderSelf = lipgloss.NewStyle().Foreground(Green).Bold(true)

	// Timestamp next to a message.
	Timestamp = lipgloss.NewStyle().Foreground(Grey)

	// Reaction renders the reaction summary under a message.
	Reaction = lipgloss.NewStyle().
			Foreground(PurpleLt).
			Background(lipgloss.Color("#2A2A3D")).
			Padding(0, 1)

	// ImagePlaceholder marks an inline image the terminal can't render; the
	// user opens it in their default viewer/browser with the image keybinding.
	ImagePlaceholder = lipgloss.NewStyle().
				Foreground(PurpleLt).
				Background(lipgloss.Color("#2A2A3D")).
				Italic(true).
				Padding(0, 1)

	// Banner for meeting/notification alerts.
	Banner = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#000000")).
		Background(Yellow).
		Bold(true).
		Padding(0, 1)

	// ErrorBanner for error messages.
	ErrorBanner = lipgloss.NewStyle().
			Foreground(White).
			Background(Red).
			Bold(true).
			Padding(0, 1)

	// DeviceCode highlights the user code during sign-in.
	DeviceCode = lipgloss.NewStyle().
			Foreground(White).
			Background(Purple).
			Bold(true).
			Padding(1, 3)

	// DeviceURL highlights the verification URL during sign-in.
	DeviceURL = lipgloss.NewStyle().
			Foreground(White).
			Bold(true)

	// Hint is dim helper text.
	Hint = lipgloss.NewStyle().Foreground(Grey)

	// UnreadTitle / UnreadDesc colour a chat row with unread messages in the
	// orange accent so it stands out from already-read chats. They mirror the
	// default delegate's normal title/description layout (left padding) so the
	// only visible change is the foreground colour (and a bold title).
	UnreadTitle = lipgloss.NewStyle().
			Foreground(Orange).
			Bold(true).
			Padding(0, 0, 0, 2)

	UnreadDesc = lipgloss.NewStyle().
			Foreground(Orange).
			Padding(0, 0, 0, 2)

	// CodeBlockBg is the dim background behind fenced code blocks in messages.
	CodeBlockBg = lipgloss.Color("#15151F")

	// CodeFg is the default foreground for un-highlighted code text (used when
	// the language is unknown, so the syntax highlighter can't colour tokens).
	CodeFg = lipgloss.Color("#C8C8D8")

	// InlineCode highlights a `backtick` snippet within prose.
	InlineCode = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E0E0F0")).
			Background(CodeBlockBg)

	// MessageSelected marks the currently selected message in the messages
	// pane (a small caret before its header) so react/quote targets are clear.
	MessageSelected = lipgloss.NewStyle().Foreground(Orange).Bold(true)

	// SelectionHighlight marks a mouse-dragged text selection in the messages
	// viewport (inverse-ish: purple background, white text) so the copied/quoted
	// range is visible.
	SelectionHighlight = lipgloss.NewStyle().Background(Purple).Foreground(White)

	// Quote styles the text of a quoted reply (a ">"-prefixed run in a message
	// body). QuoteBar colours the left bar that marks the quoted block.
	Quote = lipgloss.NewStyle().Foreground(LightGrey).Italic(true)

	QuoteBar = lipgloss.NewStyle().Foreground(Purple)

	// EmojiPicker frames the inline emoji autocomplete popup shown above the
	// compose box while typing a :shortcode:.
	EmojiPicker = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Purple).
			Padding(0, 1)

	// EmojiPickerItem is a non-selected row in the emoji popup.
	EmojiPickerItem = lipgloss.NewStyle().Foreground(LightGrey)

	// EmojiPickerSelected highlights the active row in the emoji popup.
	EmojiPickerSelected = lipgloss.NewStyle().
				Foreground(White).
				Background(Purple).
				Bold(true)

	// ReplyBar / ReplyLabel / ReplyText style the reply-preview banner shown
	// above the compose box while replying to a message (q). The bar echoes the
	// quoted-message left bar (QuoteBar) so the two read as the same concept;
	// the label names the sender being replied to and the text is the quoted
	// snippet.
	ReplyBar   = lipgloss.NewStyle().Foreground(Purple)
	ReplyLabel = lipgloss.NewStyle().Foreground(PurpleLt).Bold(true)
	ReplyText  = lipgloss.NewStyle().Foreground(LightGrey).Italic(true)

	// SpellLabel prefixes the spell-check strip under the compose box (e.g.
	// "spelling:").
	SpellLabel = lipgloss.NewStyle().Foreground(Grey)

	// SpellWord marks a misspelled word in the spell-check strip (red +
	// underline, echoing how GUI editors flag misspellings).
	SpellWord = lipgloss.NewStyle().Foreground(Red).Underline(true)

	// SpellSuggestion styles a suggested correction shown after a misspelling.
	SpellSuggestion = lipgloss.NewStyle().Foreground(LightGrey)

	// ScrollbarTrack is the dim gutter drawn down the right edge of the
	// messages viewport; ScrollbarThumb is the brighter handle marking the
	// visible slice of the conversation within it.
	ScrollbarTrack = lipgloss.NewStyle().Foreground(Grey)
	ScrollbarThumb = lipgloss.NewStyle().Foreground(PurpleLt)
)
