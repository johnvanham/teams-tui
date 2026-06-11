# teams-tui

An intuitive terminal UI for Microsoft Teams **chats**, built in Go with
[Bubble Tea v2](https://github.com/charmbracelet/bubbletea). It focuses on
one-to-one, group, and meeting chats, and gives you a heads-up when a meeting
is about to start.

```
 Teams — Ada Lovelace
┌──────────────────────────────┐┌────────────────────────────────────────────┐
│ Chats                        ││ Grace Hopper          14:02                  │
│ [>] Grace Hopper             ││ Did you push the nanosecond demo?            │
│     Grace: did you push…     ││                                              │
│ [#] Compiler Team            ││ You                   14:03                  │
│     You: shipping v2 now     ││ Yes — merged and shipping v2 now.            │
│ [@] Standup                  ││                                              │
│     Alan: notes attached     │└──────────────────────────────────────────────┘
│                              │┌────────────────────────────────────────────┐
│                              ││ Type a message… (enter to send)              │
└──────────────────────────────┘└────────────────────────────────────────────┘
 COMPOSE · 3 chats
 tab next pane · enter send · ctrl+r refresh · ? help · ctrl+c quit
```

## Features

- **Chats only, done well:** one-to-one, multi-person group, and meeting chats.
- **Live updates** via lightweight polling of Microsoft Graph.
- **Send messages** inline from the compose box.
- **Emoji:** incoming Teams emoji render as Unicode glyphs. When composing, type
  a shortcode (`:thumbsup:`) or a classic emoticon (`:-)`, `<3`) and it is
  converted to the emoji on send. Typing `:` followed by at least two characters
  opens an inline picker above the compose box (`↑`/`↓` to select, `tab`/`enter`
  to insert, `esc` to dismiss) without interrupting your typing.
- **Presence:** see each participant's Teams status next to their name, and set
  your own status from a popup (`ctrl+s`). While running, the app maintains a
  presence session so your chosen status persists like a native client.
- **Meeting alerts:** an in-TUI banner *and* a desktop notification when a
  meeting is starting within your lookahead window.
- **Hybrid-friendly auth:** uses the OAuth 2.0 **device authorization grant**,
  so sign-in happens in your real browser. This works for both fully
  Entra-hosted tenants and **hybrid Entra/Active Directory federated** setups —
  if your tenant redirects to a company-hosted web login (ADFS or similar),
  that page simply appears during the browser step.
- **Secure token cache:** refresh tokens are stored in your OS keyring
  (Keychain / Secret Service / Credential Manager), so you don't re-auth every
  launch.
- Built almost entirely from off-the-shelf
  [Bubbles](https://github.com/charmbracelet/bubbles) components (`list`,
  `viewport`, `textarea`, `spinner`, `help`, `key`) and
  [Lip Gloss](https://github.com/charmbracelet/lipgloss) styling.

## Requirements

- Go 1.24+
- A registered Microsoft Entra application (see below)
- On Linux, a Secret Service provider (e.g. GNOME Keyring / KWallet) for the
  token cache

## Register an Entra application

The device-code flow needs a public client application registered in your
tenant. An administrator performs these one-time steps in the
[Entra admin center](https://entra.microsoft.com):

1. **Identity → Applications → App registrations → New registration.**
   - Name: `teams-tui` (anything).
   - Supported account types: *Accounts in this organizational directory only*
     (single tenant) is recommended for corporate/hybrid setups.
   - Leave the Redirect URI blank.
2. Open the new app → **Authentication**.
   - Under **Advanced settings**, set **Allow public client flows** to **Yes**.
     (This enables the device-code grant.)
3. Open **API permissions → Add a permission → Microsoft Graph → Delegated
   permissions** and add:
   - `User.Read`
   - `Chat.ReadWrite`
   - `People.Read` (to list contacts and start new chats from them)
   - `Calendars.Read` (for meeting notifications; optional)
   - `Presence.Read.All` (to show participants' Teams status; needs admin consent)
   - `Presence.ReadWrite` (to show and set your own status)
   - Click **Grant admin consent** if required by your tenant.

   > **Note:** `openid`, `profile`, and `offline_access` are OpenID Connect
   > scopes, **not** Graph resource permissions — they won't appear in the main
   > Graph permissions list. You do **not** need to add them in the portal:
   > teams-tui requests them as scopes during sign-in and Entra grants them
   > automatically. (If you want them listed explicitly, they live under the
   > collapsed **"OpenId permissions"** group on the same *Microsoft Graph →
   > Delegated permissions* screen.)
4. Note the **Application (client) ID** and the **Directory (tenant) ID** from
   the app's **Overview** page.

> For hybrid Entra/AD federated tenants, always use the specific **tenant GUID**
> (not `common`/`organizations`) so the browser sign-in is routed to your
> federation/web login.

## Configuration

teams-tui reads configuration from environment variables and/or a JSON file.
Environment variables take precedence.

### Environment variables

| Variable                   | Description                                  |
| -------------------------- | -------------------------------------------- |
| `TEAMS_TUI_CLIENT_ID`      | Application (client) ID. **Required.**       |
| `TEAMS_TUI_TENANT_ID`      | Directory (tenant) ID or GUID. Default `organizations`. |
| `TEAMS_TUI_AUTH_HOST`      | Login host. Default `https://login.microsoftonline.com`. |
| `TEAMS_TUI_GRAPH_BASE_URL` | Graph base URL. Default `https://graph.microsoft.com/v1.0`. |
| `TEAMS_TUI_SCOPES`         | Space-separated scope override.              |
| `TEAMS_TUI_CONFIG`         | Path to the JSON config file.                |

### Config file

Default location: `$XDG_CONFIG_HOME/teams-tui/config.json` (typically
`~/.config/teams-tui/config.json`).

```json
{
  "client_id": "00000000-0000-0000-0000-000000000000",
  "tenant_id": "11111111-1111-1111-1111-111111111111",
  "poll_interval_seconds": 10,
  "meeting_lookahead_minutes": 5,
  "disable_desktop_notify": false
}
```

For a sovereign cloud or a custom federation host, override `auth_host` and
`graph_base_url` accordingly.

## Build & run

```sh
go build -o teams-tui ./cmd/teams-tui
TEAMS_TUI_CLIENT_ID=<client-id> TEAMS_TUI_TENANT_ID=<tenant-id> ./teams-tui
```

On first launch you'll see a verification URL and a short code. Open the URL in
any browser, sign in (your company login page will appear if your tenant is
federated), enter the code, and the TUI loads your chats. Subsequent launches
reuse the cached refresh token from your keyring.

### Re-authenticating after adding a permission

If you add a new Graph permission (e.g. `Presence.Read.All`), the cached token
won't include it and calls will fail with `403 InsufficientPrivileges`. The app
detects this automatically — on the next launch it sees the cached token no
longer covers the requested scopes and starts a fresh sign-in (with the new
consent prompt). To force it immediately:

```sh
./teams-tui --logout   # clears the cached token from the keyring
./teams-tui            # next launch prompts for sign-in + new consent
```

Make sure **admin consent** is granted for the new permission in the app
registration first if your tenant requires it.

## Key bindings

| Key            | Action                                   |
| -------------- | ---------------------------------------- |
| `tab` / `shift+tab` | Move focus between Chats / Messages / Compose |
| `↑`/`↓` `j`/`k`| Navigate the focused pane                |
| `enter`        | Open selected chat (Chats) / send message (Compose) / start chat (Contacts) |
| `alt+enter`    | Insert a newline in the compose box      |
| `:` + 2 chars  | Open the inline emoji picker while composing (`↑`/`↓` select, `tab`/`enter` insert, `esc` close) |
| `/`            | Filter the chat list / search contacts   |
| `ctrl+o`       | Toggle the sidebar between Chats and Contacts (start a new chat) |
| `ctrl+e`       | Edit your most recent message            |
| `ctrl+y` / click | Open an image in your default viewer/browser (`ctrl+y` = newest; click a placeholder for that one) |
| `ctrl+v`       | Paste an image from the clipboard and attach it to the next message (type a caption, then `enter` to send; `esc` to discard) |
| `ctrl+r`       | Refresh now                              |
| `ctrl+s`       | Open the status picker (set your presence) |
| `ctrl+g`       | Toggle full help                         |
| `ctrl+c`       | Quit                                     |

## Architecture

```
cmd/teams-tui        program entrypoint, signal handling, wiring
internal/config      config loading (env + JSON) and endpoint URLs
internal/auth        OAuth device-code flow, refresh, keyring token store
internal/graph       Microsoft Graph client (chats, messages, calendar) + types
internal/notify      desktop notifications (beeep), degrade-gracefully
internal/ui          Bubble Tea v2 model/update/view, commands, components
internal/ui/styles   Lip Gloss styles
```

The UI follows The Elm Architecture: all Graph/auth I/O happens inside
`tea.Cmd`s that return typed messages, which the single `Update` function folds
into the model. Polling is driven by `tea.Tick`.

## Notes & limitations

- Reading tokens is intentionally avoided in code; tokens are treated as opaque
  per Microsoft guidance.
- Message history is fetched per chat on demand and refreshed on the poll tick.
- Chats with unread messages are shown in orange text in the sidebar (the
  selected chat keeps its pink highlight). Unread state comes from Graph's
  per-chat `viewpoint` read marker; opening a chat marks it read locally and on
  the server (via `markChatReadForUser`, covered by `Chat.ReadWrite`), so it
  syncs to your other devices. Your own most recent message never counts as
  unread.
- This is a chat-focused client; it does not implement calls, channels/teams
  browsing, files, or app tabs.
- Reactions are rendered; threaded replies are not.
- Images in messages are shown as placeholders (`🖼 [n] name`); press `ctrl+y`
  to open the newest one, or click a placeholder to open that specific image, in
  your OS default image viewer/browser (inline Graph hosted content is
  downloaded with your token first). Other attachment types are not yet rendered.
- You can paste an image from the clipboard with `ctrl+v` to send it inline.
  This shells out to a platform clipboard helper, so the relevant tool must be
  installed: `wl-paste` (Wayland) or `xclip` (X11) on Linux, `osascript` on
  macOS, and `powershell` on Windows. The image is sent as Teams hosted content
  via `Chat.ReadWrite` (no extra scope required); any text you type becomes the
  image's caption.
