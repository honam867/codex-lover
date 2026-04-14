# codex-lover

![codex-lover cover](./docs/cover.png)

`codex-lover` is a Windows-first desktop manager for multi-account `codex` usage, quota tracking, account switching, and OpenCode sync.

If you are searching for a tool related to:

- Codex account manager
- Codex multi-account manager
- Codex quota tracker
- Codex usage tracker
- Codex auto switch account
- Codex 5H limit monitor
- Codex weekly limit monitor
- OpenCode sync for Codex
- Windows desktop app for Codex accounts

this project is built for exactly that workflow.

It exists to remove the repetitive, error-prone flow of:

- opening `codex`
- checking quota manually
- logging out
- logging into another account
- trying to keep `opencode` on the same account
- remembering which logged-out account will reset next

Instead, `codex-lover` keeps a local account registry, tracks Codex usage, syncs OpenCode, and can switch Codex to another cached account when the active one hits the 5H or weekly limit.

## SEO Summary

`codex-lover` is a desktop app for managing multiple Codex accounts on Windows. It works as a Codex quota tracker, Codex usage monitor, Codex account switcher, and OpenCode sync helper. The app helps users monitor Codex 5H usage, weekly usage, active account state, logged-out accounts, cached quota, reset times, and automatic account switching when a Codex account reaches limit.

## Pain Points

Typical multi-account Codex usage has a few recurring problems:

- the active account lives in one auth file, so switching accounts manually is annoying
- logged-out accounts disappear from view even though you still care about their last known quota and reset time
- `codex` and `opencode` can drift onto different OpenAI accounts
- once an account reaches the 5H or weekly limit, switching cleanly takes too many manual steps
- terminal-only dashboards become fragile once you want account management, notifications, and stable layout

## What codex-lover Solves

`codex-lover` turns that into one desktop workflow:

- track all known Codex accounts in one place
- show which account is currently active
- keep logged-out accounts visible with cached usage
- infer reset recovery for logged-out accounts once reset time passes
- notify when the active account reaches 20% and 10%
- auto-switch to another cached account when the active account reaches limit
- sync OpenCode to the active Codex account automatically

## Keyword Focus

Main keyword targets for this repository:

- Codex account manager
- Codex multi-account manager
- Codex quota tracker
- Codex usage tracker
- Codex account switcher
- Codex auto switch
- Codex 5H limit tracker
- Codex weekly limit tracker
- OpenCode sync
- Codex Windows desktop app

Secondary keyword targets:

- manage multiple Codex accounts
- switch Codex account without logout
- track logged-out Codex account usage
- Codex quota dashboard
- Codex account monitor
- Codex OpenCode sync tool

## Core Features

- Desktop app powered by Wails + React.
- Multi-account account registry backed by local state in `%USERPROFILE%\.codex-lover`.
- `ACTIVE` vs `LOGGED_OUT` state tracking.
- 5H and weekly quota cards with colored progress bars.
- Threshold notifications at 20% and 10%.
- Auto-switch when the active account reaches effective limit.
- Background refresh for logged-out accounts with cached auth.
- OpenCode sync from active Codex account.
- Add account without logging out the current one first.
- Log into a cached account directly from the app.
- Log out and delete an account from local state and cache.

## Current Product Model

Current source of truth:

- Codex is the source of truth.
- OpenCode follows Codex.
- The desktop app is now the main control loop.

This means:

- `codex-lover` or `codex-lover run` opens the desktop app
- the desktop app refreshes usage every 15 seconds
- notifications, auto-switch, and OpenCode sync run in the desktop backend
- `codex-lover status` and `codex-lover refresh` still exist as text commands for verification or scripting

Compatibility note:

- `codex-lover watch` currently opens the desktop app as well

## Requirements

Minimum environment:

- Windows
- Go
- Node.js + npm
- Wails CLI
- WebView2 runtime
- Codex installed and logged in at least once

Optional:

- OpenCode installed if you want OpenCode sync

Common paths on the current target machine:

- Codex auth: `%USERPROFILE%\.codex\auth.json`
- OpenCode auth: `%USERPROFILE%\.local\share\opencode\auth.json`
- codex-lover state: `%USERPROFILE%\.codex-lover`

## Installation From Source

### 1. Install Wails CLI

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Optional verification:

```powershell
wails doctor
```

### 2. Build and install codex-lover

From the repo root:

```powershell
.\install.cmd
```

or:

```powershell
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

What the installer does:

- builds the Wails desktop app
- builds the CLI launcher
- installs `codex-lover.exe` to `%LOCALAPPDATA%\codex-lover\bin`
- installs `codex-lover-desktop.exe` to the same folder
- creates `codex-lover.cmd`
- adds `%LOCALAPPDATA%\codex-lover\bin` to user `PATH`

If your current shell was opened before installation, either open a new terminal or run:

```powershell
set PATH=%LOCALAPPDATA%\codex-lover\bin;%PATH%
```

## Quick Start

### 1. Make sure Codex already has one login

Verify:

```powershell
Test-Path "$env:USERPROFILE\.codex\auth.json"
```

If this is false, open Codex once and log in normally first.

### 2. Launch the desktop app

```powershell
codex-lover
```

or:

```powershell
codex-lover run
```

### 3. Add another account

Use the `Add account` button in the desktop app.

What happens:

- `codex-lover` opens a separate console
- that console runs `codex-lover account add`
- `codex login` opens its normal browser/device flow
- after login finishes, refresh the desktop app

Important:

- this does not require logging out the currently active account first

### 4. Switch to another cached account

Use the `Log in` button on a logged-out account card.

That restores cached auth into the runtime Codex home and makes that account active.

### 5. Remove an account

Use the `Delete` button on a card.

This removes:

- cached auth for that account
- local stored profile/state
- runtime auth too, if that account is currently active

## Notifications And Automation

When the desktop app is open, the backend loop runs automatically.

Current loop behavior:

- refresh active usage every 15 seconds
- refresh logged-out accounts with cached auth every 15 minutes
- notify when active account drops to 20%
- notify when active account drops to 10%
- auto-switch when the active account reaches effective limit
- notify after a successful auto-switch
- sync OpenCode to the active Codex account

## Commands

Primary commands:

```powershell
codex-lover
codex-lover run
codex-lover watch
codex-lover status
codex-lover refresh
codex-lover account add
codex-lover profile import codex --label NAME --home PATH
codex-lover profile list
codex-lover daemon
codex-lover daemon-status
```

Command intent:

- `codex-lover` / `run`: open desktop app
- `watch`: compatibility alias to desktop app
- `status`: print one-shot text summary
- `refresh`: refresh and print one-shot text summary
- `account add`: open interactive Codex login flow in terminal
- `profile import`: manually import a Codex home
- `profile list`: list stored profiles

## Runtime Files

Runtime data is stored outside the repo:

- config: `%USERPROFILE%\.codex-lover\config.json`
- state: `%USERPROFILE%\.codex-lover\state.json`
- cached Codex auth: `%USERPROFILE%\.codex-lover\codex-auth\`
- managed login homes: `%USERPROFILE%\.codex-lover\homes\codex\`

Do not commit auth files or print raw tokens.

## Architecture

High-level layout:

- [main.go](./cmd/codex-lover/main.go): CLI entrypoint
- [app.go](./internal/app/app.go): CLI command routing and launcher behavior
- [app.go](./internal/desktop/app.go): CLI-side desktop launcher
- [main.go](./desktop-app/main.go): Wails desktop app entry
- [app.go](./desktop-app/app.go): desktop bindings and actions
- [runtime.go](./desktop-app/runtime.go): desktop watch loop, auto-switch, notifications, OpenCode sync
- [service.go](./internal/service/service.go): shared business logic
- [sync.go](./internal/opencode/sync.go): OpenCode credential sync

## Troubleshooting

### The installer fails because `codex-lover-desktop.exe` is in use

Close the app and rerun:

```powershell
taskkill /IM codex-lover-desktop.exe /F
.\install.cmd
```

### `Add account` opens a console and does not close immediately

That is expected.

The console is hosting the interactive `codex login` flow. If you close the browser early, the login process may still be waiting in the console.

### OpenCode did not change accounts

OpenCode only follows the currently active Codex account, and the desktop app must be open for the live control loop to keep syncing.

### Auto-switch does not happen

Common causes:

- the active account has not actually hit effective limit yet
- the target account has usage history but no cached auth
- there is no other cached account with usable quota

## Development

Typical local loop:

```powershell
go test ./...
.\install.cmd
codex-lover run
```

If you only want the desktop app build:

```powershell
cd .\desktop-app
wails build -clean
```

## Status

This is already usable as a daily local tool, but it is still a prototype and still Windows-first.

Likely next areas:

- better desktop polish
- better event feed/history inside the app
- smarter switch policy tuning
- better onboarding flow for first-time users
