# AGENTS.md

Guidance for AI coding agents (and humans) working in this repository.

## Project

`teams-tui` is a terminal UI for Microsoft Teams chat, written in **Go** using
the **Bubble Tea v2** framework (The Elm Architecture). It talks to Microsoft
Graph v1.0 directly via a small hand-rolled REST client. Syntax highlighting of
code blocks is the one notable third-party feature dependency
([chroma](https://github.com/alecthomas/chroma)).

## Build / Run / Test

```sh
# Build
go build -o teams-tui ./cmd/teams-tui

# Run (requires an Entra app registration)
TEAMS_TUI_CLIENT_ID=<client-id> TEAMS_TUI_TENANT_ID=<tenant-id> ./teams-tui

# Clear cached auth token
./teams-tui --logout

# Vet + test (run these before committing)
go vet ./...
go test ./...
go build ./...
```

Always run `go build ./...` and `go vet ./...` after making changes. Unit tests
live next to the code they cover (e.g. `internal/auth`, `internal/graph`,
`internal/ui`); add new tests in the same package as the code under test.

## Architecture

Elm-style: all I/O is wrapped in `tea.Cmd`s that return typed messages, folded
into a single `Update`. Never block in `Update` or `View`; do I/O in a `tea.Cmd`.

| Package | Path | Role |
|---|---|---|
| entrypoint | `cmd/teams-tui/main.go` | flags, signal ctx, wiring |
| config | `internal/config/` | env + JSON config, endpoint URLs |
| auth | `internal/auth/` | device-code OAuth, refresh, OS keyring |
| graph | `internal/graph/` | Microsoft Graph REST client + models |
| notify | `internal/notify/` | desktop notifications (beeep + D-Bus clickable actions) |
| focus | `internal/focus/` | raise terminal window + select tmux pane on notification click |
| spell | `internal/spell/` | compose-box spell check (enchant-2/hunspell subprocess) |
| ui | `internal/ui/` | Bubble Tea model/update/view |
| styles | `internal/ui/styles/` | Lip Gloss styling |

### Key UI files (`internal/ui/`)
- `model.go` — root `Model` struct + `New` constructor. All state lives here.
- `update.go` — `Update` event loop + `handleKey` keyboard dispatch.
- `view.go` — rendering + `layout()` geometry. Constants like `sidebarWidth`.
  Message bodies are rendered by `renderBody`, which splits prose from fenced
  code blocks and styles each (`renderCodeBlock` for blocks).
- `highlight.go` — chroma-based syntax highlighting (`highlightCode`). Resolves a
  lexer from the fence language (or content analysis) and colours tokens from the
  configured chroma `*chroma.Style`, which is resolved once in `New` and stored
  on `Model.codeStyle`.
- `commands.go` — `tea.Cmd` constructors and the message types they return.
- `keys.go` — `keyMap` keybinding definitions + help text.
- `chatitem.go`, `statusitem.go` — `list.Item` adapters.

### Graph client (`internal/graph/`)
- `client.go` — `Client` + `do()` (the single request helper). All calls funnel
  through `do()`, which handles auth headers and absolute-vs-relative URLs
  (absolute is used to follow `@odata.nextLink`).
- `types.go` — Graph data models (`Chat`, `Message`, `Presence`, etc.).
- `text.go` — HTML→plaintext conversion for message bodies.
- `code.go` — receive side: extracts code from message HTML (Teams'
  `<codeblock>` element and `<pre>`/`<code>`) into Markdown-ish fences the UI
  renders, detecting the language from `data-language`, `class="language-…"`, or
  Teams' bare `class="Php"` form.
- `compose.go` — send side: `ComposeHTML` converts compose-box text (Markdown
  fences + inline `` `code` ``) to the `<pre><code>` HTML Teams stores, so code
  blocks and newlines render for every participant.
- `debug.go` — opt-in raw message-body dump gated by `TEAMS_TUI_DEBUG_BODIES`
  (a file path); used to inspect exactly how Graph stores a message.

The code-block fence convention (the literal ```` ``` ```` plus optional
language) is shared between `graph/code.go` (parsing), `graph/compose.go`
(sending), and `ui/view.go` (rendering) — keep the three in sync when changing
it.

### Notifications & click-to-focus (`internal/notify/`, `internal/focus/`)
- `notify/notify.go` — `Notifier` wraps beeep for plain `Notify`/`Alert` and
  adds `NotifyWithAction(title, msg, onClick)` for clickable notifications.
  `onClick` receives the XDG activation token and runs on a background goroutine.
- `notify/dbus_linux.go` — the actionable backend. Talks to
  `org.freedesktop.Notifications` over D-Bus directly (godbus), sends a
  `default` action, and listens on one signal channel for `ActivationToken`
  (stashed per notification id) and `ActionInvoked` (fires the stored callback
  with that token). `newActionBackend` returns nil (→ plain-notify fallback)
  when there's no session bus or the daemon lacks the `actions` capability.
  `dbus_other.go` is the non-Linux stub.
- `focus/focus.go` — `RaiseTerminal(template, token)` runs the configured
  `focus_command` (via `sh -c`, `{token}` substituted + `XDG_ACTIVATION_TOKEN`
  exported); `SelectTmuxPane(pane)` runs `tmux select-window`/`select-pane`.
  Both are best-effort no-ops when unconfigured / outside tmux.
- Wiring: `notifyNewChatMessages` (`ui/update.go`) builds the click callback via
  `onNotificationClick(chatID)` → raise + tmux-select + `sender.Send(openChatByIDMsg{})`.
  The `sender` (`*programSender` on `Model`) is pointed at `p.Send` by
  `Model.SetProgram` in `main.go` after the program is created (the Model is
  copied by value, so the send hook must be behind a shared pointer). `Update`
  handles `openChatByIDMsg` via `handleOpenChatByID`, which mirrors
  `handleChatCreated`. tmux target auto-detected from `$TMUX_PANE` in `New`.

### Spell check (`internal/spell/`)
- `spell.go` — thin subprocess wrapper. `New(lang)` resolves a system helper
  (`enchant-2`, then `hunspell`) and returns a disabled `Checker` when none is
  installed (feature stays dormant, never errors). `CheckText` pipes the compose
  text to the helper's ispell `-a` mode and parses the reply for misspellings +
  suggestions. In the UI it's driven off `Update`: a compose edit bumps
  `Model.spellGen` and arms   `spellDebounceCmd`; when the debounce fires (and the
  generation still matches) `spellCheckCmd` runs the check in a `tea.Cmd`, and
  `viewSpellStrip` renders the result beneath the compose box. `spellStripHeight`
  feeds `layout()` so the strip steals a row from the messages viewport.
- `ui/spellpicker.go` — the `ctrl+f` correction picker. `openSpellPicker`
  flattens the current misspellings into `word → suggestion` candidates;
  `applySpellCandidate` splices the chosen suggestion into the compose box
  (whole-word match via `findWord`) using the same cursor-reposition pattern as
  the emoji/mention pickers. `handleSpellPickerKey` drives it and re-checks after
  a fix; `viewSpellPicker`/`spellPickerHeight` render it like the other popups.

## Conventions

- **Adding a Graph call:** add a method on `*Client` in `client.go` using the
  `do()` helper; add any new response types to `types.go`.
- **Adding a keybinding:** add a field to `keyMap` (`keys.go`), register it in
  `defaultKeyMap()`, add to `ShortHelp`/`FullHelp`, and handle it in
  `handleKey()` (`update.go`).
- **Adding async work:** add a `tea.Cmd` constructor + a message type in
  `commands.go`, then a `case` in `Update` (`update.go`).
- **State** belongs on `Model` (`model.go`); initialize maps/slices in `New`.
- **Styling** goes in `internal/ui/styles/styles.go`; don't hardcode colors in
  view code — reuse or add a named style. Exception: code-block token colours
  come from the chroma theme (`highlight.go`), not the app palette.
- **Adding a config option:** add the field (with a `json` tag) to `Config` in
  `internal/config/config.go`, apply a default in `applyDefaults`, optionally add
  a `TEAMS_TUI_*` env override in `Load`, then document it in the README and
  `config.example.json`.
- Keep comments explaining *why*, in the existing godoc style. Match the
  surrounding code; this codebase favors small, well-documented helpers.
- Graph delegated permissions (scopes) live in `config.DefaultScopes`. If a new
  feature needs a new scope, add it there and note it in the README.

## Git

This repo uses automatic per-feature commits. After completing a self-contained
feature or fix that builds cleanly, commit it with a concise, conventional
message (e.g. `feat: ...`, `fix: ...`, `docs: ...`). Do not commit the built
`teams-tui` binary (it is gitignored) or any `config.json`.
