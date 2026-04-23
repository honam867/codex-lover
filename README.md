# codex-lover

![codex-lover cover](./docs/cover.png)

`codex-lover` is a Windows-first desktop control panel for people who juggle multiple AI coding accounts and do not want to babysit quota windows, auth files, and account switching by hand.

It keeps your active and logged-out accounts visible in one desktop app, tracks quota usage, preserves cached account state, and can rotate to another cached Codex account when the active one is exhausted.

## Why People Use It

- See all known accounts in one place instead of losing track after logout.
- Track 5H and weekly quota without manually checking each account.
- Keep logged-out accounts visible with their last known usage and reset timing.
- Switch back into cached accounts without repeating the whole login dance.
- Keep OpenCode aligned with the active Codex account.

## Who This Is For

`codex-lover` is for you if you:

- rotate across multiple Codex accounts regularly
- want a desktop UI instead of managing auth files by hand
- care about reset timing, cached usage, and quick switching
- use OpenCode and want it to follow the currently active Codex account

## Why This Exists

Heavy multi-account Codex usage usually turns into a repetitive loop:

- open Codex
- check quota manually
- log out
- log into another account
- try to remember which account resets next
- hope OpenCode is still on the same account

`codex-lover` turns that into one local desktop workflow.

## What You Get

- A Wails + React desktop app as the primary UI.
- One card grid for active and logged-out accounts.
- Quota bars for 5H and weekly windows.
- Notifications when active Codex quota drops to 20% and 10%.
- Auto-switch to another cached Codex account when the active one reaches the limit.
- Automatic OpenCode sync from the active Codex account.
- Add, activate, and delete accounts from the desktop app.
- Local profile support for Codex, Claude, and Kimi.
- Plain-text CLI commands for quick status checks and scripting.

## Current Focus

This project already supports profiles for `codex`, `claude`, and `kimi`.

Current automation is still Codex-first:

- Codex is the source of truth for the live switching loop.
- OpenCode sync follows the active Codex account.
- The desktop runtime is where refresh, notifications, auto-switch, and sync happen.

## Important Limitation

Visible does not always mean switchable.

Auto-switch only works for accounts whose auth has already been cached locally. A logged-out account can still appear in the app with cached usage, but it cannot become active again unless cached auth exists for that account.

## Quick Start

### 1. Requirements

- Windows
- Go
- Node.js and npm
- Wails CLI
- WebView2 runtime
- Codex installed and logged in at least once

Optional:

- OpenCode installed if you want automatic OpenCode sync
- Claude CLI if you want to add Claude accounts
- Kimi CLI if you want to add Kimi accounts

### 2. Install Wails CLI

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Optional verification:

```powershell
wails doctor
```

### 3. Build and install `codex-lover`

From the repo root:

```powershell
.\install.cmd
```

or:

```powershell
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

The installer:

- builds the Wails desktop app
- builds the CLI launcher
- installs `codex-lover.exe`
- installs `codex-lover-desktop.exe`
- creates `codex-lover.cmd`
- updates the user `PATH` when needed

### 4. Make sure Codex already has one login

```powershell
Test-Path "$env:USERPROFILE\.codex\auth.json"
```

If this is `False`, open Codex once and log in normally first.

### 5. Launch the desktop app

```powershell
codex-lover
```

or:

```powershell
codex-lover run
```

### 6. Add another account

Use the add-account flow in the desktop app.

What happens:

- a separate console opens
- it runs `codex-lover account add <provider>`
- the provider's normal login flow opens
- after login succeeds, the account is imported into local state

### 7. Switch to another cached account

Use the login action on a logged-out account card.

### 8. Remove an account

Use the `Delete` action on a card.

This removes:

- cached auth for that account
- local stored profile and state
- runtime auth too, if that account is currently active

## Commands

```powershell
codex-lover
codex-lover run
codex-lover watch
codex-lover status
codex-lover refresh
codex-lover account add [codex|claude|kimi]
codex-lover profile import <codex|claude|kimi> --label NAME --home PATH
codex-lover profile list
codex-lover daemon
codex-lover daemon-status
```

Command intent:

- `codex-lover` and `codex-lover run` open the desktop app
- `codex-lover watch` currently opens the desktop app too
- `status` prints a one-shot text summary
- `refresh` refreshes and prints a one-shot text summary
- `account add` opens the interactive login flow for a provider
- `profile import` manually imports an existing provider home
- `profile list` prints stored profiles

## Desktop Runtime Behavior

When the desktop app is open, the live backend loop runs automatically.

Current behavior:

- refresh active usage every 15 seconds
- refresh logged-out cached accounts every 15 minutes
- notify when active Codex quota drops to 20%
- notify when active Codex quota drops to 10%
- auto-switch when the active Codex account reaches its effective limit
- notify after a successful auto-switch
- sync OpenCode to the active Codex account

## Runtime Files

Runtime data lives outside the repo:

- `%USERPROFILE%\.codex\auth.json`
- `%USERPROFILE%\.local\share\opencode\auth.json`
- `%USERPROFILE%\.codex-lover\config.json`
- `%USERPROFILE%\.codex-lover\state.json`
- `%USERPROFILE%\.codex-lover\codex-auth\*.json`
- `%USERPROFILE%\.codex-lover\homes\codex\`
- `%USERPROFILE%\.codex-lover\homes\claude\`
- `%USERPROFILE%\.codex-lover\homes\kimi\`

Do not commit auth files or print raw tokens.

## Architecture

High-level layout:

- [`cmd/codex-lover/main.go`](./cmd/codex-lover/main.go): CLI entrypoint
- [`internal/app/app.go`](./internal/app/app.go): command routing and login flows
- [`internal/desktop/app.go`](./internal/desktop/app.go): desktop launcher
- [`desktop-app/main.go`](./desktop-app/main.go): Wails desktop app entry
- [`desktop-app/app.go`](./desktop-app/app.go): desktop bindings and actions
- [`desktop-app/runtime.go`](./desktop-app/runtime.go): refresh loop, notifications, auto-switch, OpenCode sync
- [`internal/service/service.go`](./internal/service/service.go): shared business logic
- [`internal/opencode/sync.go`](./internal/opencode/sync.go): OpenCode auth sync

## Troubleshooting

### Installer fails because `codex-lover-desktop.exe` is in use

Close the app and rerun:

```powershell
taskkill /IM codex-lover-desktop.exe /F
.\install.cmd
```

### `Add account` opens a console and looks stuck

That is expected. The console hosts the interactive provider login flow. If you close the browser early, the console process may still be waiting.

### OpenCode did not change accounts

OpenCode only follows the currently active Codex account, and the desktop app must stay open for the live sync loop to keep running.

### Auto-switch does not happen

Common causes:

- the active Codex account has not actually hit the effective limit yet
- the target account has usage history but no cached auth
- no other cached account has usable quota

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

`codex-lover` is already useful as a daily local tool, but it is still an actively evolving Windows-first project.

Likely next areas:

- better desktop polish
- a better event feed inside the app
- smarter switch policy tuning
- a smoother onboarding flow for first-time users
