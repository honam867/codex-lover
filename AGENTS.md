# AGENTS.md

This file is for coding agents working on `codex-lover`.

Read this file first if you need to:

- install the project from scratch
- get the app running on a fresh Windows machine
- understand what the product currently does
- explain usage to another user
- modify the current desktop-first architecture safely

## What This Project Is

`codex-lover` is a Windows-first desktop manager for multi-account Codex usage.

It exists to solve a specific workflow problem:

- users may have multiple Codex accounts
- only one account is active in the runtime auth file at a time
- manually switching accounts is tedious
- logged-out accounts still matter because their last known usage and reset time matter
- OpenCode can drift away from the currently active Codex account

`codex-lover` keeps a local account registry, tracks usage, preserves logged-out accounts in the UI, syncs OpenCode, and auto-switches Codex when the active account reaches limit.

## Current Product Model

Current source of truth:

- Codex is the source of truth.
- OpenCode follows Codex.
- The desktop app is the main live control loop.

Current UX model:

- `codex-lover` opens the desktop app
- `codex-lover run` opens the desktop app
- `codex-lover watch` also opens the desktop app for compatibility
- `codex-lover status` and `codex-lover refresh` remain plain text commands

The old terminal watch UI is no longer the primary interface.

## What The App Currently Does

When the desktop app is open, it:

1. refreshes active usage every 15 seconds
2. refreshes logged-out accounts with cached auth every 15 minutes
3. shows all known accounts in a card grid
4. marks one account as `ACTIVE`
5. keeps logged-out accounts visible as `LOGGED_OUT`
6. keeps last known usage for logged-out accounts
7. infers reset recovery for logged-out accounts when reset time passes
8. notifies at 20% and 10% thresholds
9. auto-switches to another cached account if the active account reaches limit
10. syncs OpenCode to the active Codex account

## Important Product Limitation

Auto-switch only works for accounts whose Codex auth has already been cached locally.

That means:

- an active account becomes switchable automatically
- a logged-out account can appear in the UI with cached usage
- but it cannot be switched into unless cached auth exists for that account

Do not describe “visible in the app” as equivalent to “switchable”.

## Environment Assumptions

Working environment on the target machine:

- Windows
- PowerShell
- Go installed
- Node.js + npm installed
- Wails CLI installed
- WebView2 runtime installed
- Codex installed and logged in at least once

Observed paths on the current machine:

- Codex auth: `%USERPROFILE%\.codex\auth.json`
- OpenCode auth: `%USERPROFILE%\.local\share\opencode\auth.json`
- codex-lover runtime state: `%USERPROFILE%\.codex-lover`
- installed binaries: `%LOCALAPPDATA%\codex-lover\bin`

Always verify paths if the machine differs.

## Repo Layout

Top-level files:

- [README.md](README.md): end-user documentation
- [AGENTS.md](AGENTS.md): this file
- [PLAN.desktop-app.md](PLAN.desktop-app.md): desktop plan/history
- [install.cmd](install.cmd): Windows wrapper
- [install.ps1](install.ps1): build and install script

CLI:

- [main.go](cmd\codex-lover\main.go)
- [app.go](internal\app\app.go)

Desktop launcher:

- [app.go](internal\desktop\app.go)

Desktop app:

- [main.go](desktop-app\main.go)
- [app.go](desktop-app\app.go)
- [runtime.go](desktop-app\runtime.go)
- [notifications.go](desktop-app\notifications.go)
- [App.tsx](desktop-app\frontend\src\App.tsx)
- [style.css](desktop-app\frontend\src\style.css)

Core backend packages:

- [service.go](internal\service\service.go)
- [types.go](internal\model\types.go)
- [auth.go](internal\codex\auth.go)
- [usage.go](internal\codex\usage.go)
- [cache.go](internal\codex\cache.go)
- [sync.go](internal\opencode\sync.go)
- [notify.go](internal\notify\notify.go)

## Setup From Scratch

This section is the direct playbook another agent should follow.

### 1. Clone or open the repo

```powershell
git clone https://github.com/example/codex-lover.git
cd codex-lover
```

If git is unavailable, copy the repo to a writable folder.

### 2. Verify Go

```powershell
go version
```

If that fails, install Go first.

### 3. Verify Node and npm

```powershell
node -v
npm -v
```

If either fails, install Node.js first.

### 4. Install Wails CLI

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Optional verification:

```powershell
wails doctor
```

### 5. Verify Codex auth exists

```powershell
Test-Path "$env:USERPROFILE\.codex\auth.json"
```

If false:

- open Codex normally
- log in once
- confirm the auth file exists

### 6. Optional: verify OpenCode auth

```powershell
Test-Path "$env:USERPROFILE\.local\share\opencode\auth.json"
```

OpenCode is optional, but if it exists the app can sync it automatically.

### 7. Build and install the project

From repo root:

```powershell
.\install.cmd
```

or:

```powershell
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

The installer currently does all of this:

- runs `wails build -clean` in `desktop-app`
- builds the CLI launcher
- installs `codex-lover.exe`
- installs `codex-lover-desktop.exe`
- creates `codex-lover.cmd`
- updates user `PATH` if needed

### 8. Open the app

```powershell
codex-lover
```

or:

```powershell
codex-lover run
```

Expected result:

- the desktop app opens
- account cards render
- active account appears as `ACTIVE`

### 9. Add another account

Inside the app:

- click `Add account`

Expected flow:

- a separate console opens
- it runs `codex-lover account add`
- Codex browser/device login opens
- after login succeeds, refresh the desktop app

### 10. Verify CLI summary still works

```powershell
codex-lover status
```

Expected result:

- a plain text summary of known profiles
- no old terminal card UI

## How To Explain Usage To A User

If another agent needs to explain usage quickly, use this model:

- `codex-lover` opens the desktop app
- `Add account` adds another Codex account without logging out the current one first
- `Log in` on a card makes that cached account active
- `Delete` removes cached auth and local account data for that account
- the app refreshes automatically
- notifications fire at 20% and 10%
- auto-switch happens when the active account reaches limit
- OpenCode is kept aligned with the active Codex account

## Runtime Files And Data

These files live outside the repo:

- `%USERPROFILE%\.codex\auth.json`
- `%USERPROFILE%\.local\share\opencode\auth.json`
- `%USERPROFILE%\.codex-lover\config.json`
- `%USERPROFILE%\.codex-lover\state.json`
- `%USERPROFILE%\.codex-lover\codex-auth\*.json`
- `%USERPROFILE%\.codex-lover\homes\codex\`

Do not commit or print real tokens from these files.

It is acceptable to inspect:

- email
- account id
- expiry timestamps
- provider names
- file existence

## Development Workflow

Recommended loop:

```powershell
go test ./...
.\install.cmd
codex-lover run
```

If you only need the desktop app build:

```powershell
cd .\desktop-app
wails build -clean
```

If frontend changes do not appear, rebuild and reinstall.

## Current Build Model

This matters because the desktop app is not the same executable as the CLI launcher.

Current install output:

- `codex-lover.exe`: CLI launcher and command entrypoint
- `codex-lover-desktop.exe`: actual desktop app window

Current behavior:

- `codex-lover run` launches `codex-lover-desktop.exe`
- the installer must rebuild both parts

Do not collapse these two executables unless you intentionally redesign the packaging model.

## Common Maintenance Tasks

### Change desktop UI

Start here:

- [App.tsx](desktop-app\frontend\src\App.tsx)
- [style.css](desktop-app\frontend\src\style.css)

### Change desktop live behavior

Start here:

- [runtime.go](desktop-app\runtime.go)
- [notifications.go](desktop-app\notifications.go)

### Change business logic

Start here:

- [service.go](internal\service\service.go)

Relevant methods:

- `RefreshAll`
- `RefreshLoggedOutCachedUsage`
- `AutoSwitchLimitedCodex`
- `SyncOpenCodeFromActiveCodex`
- `ActivateProfile`
- `LogoutProfile`

### Change add-account flow

Start here:

- [app.go](desktop-app\app.go)
- [app.go](internal\app\app.go)

### Change install behavior

Start here:

- [install.ps1](install.ps1)

## Troubleshooting

### Installer fails because desktop exe is in use

Close the app first or run:

```powershell
taskkill /IM codex-lover-desktop.exe /F
.\install.cmd
```

### `Add account` opens a console and browser flow looks stuck

That console is expected.

It hosts the interactive `codex login` flow. If the browser is closed early, the console may still be waiting. Closing the browser alone does not automatically cancel the login process.

### The desktop app opens but notifications do not fire

The desktop app must stay open for the live control loop to run. Notifications are not driven by `status`; they are driven by the desktop runtime loop.

### Auto-switch does not happen

Check these conditions:

- the active account actually reached effective limit
- another account has cached auth
- another account has usable quota

### UI changes do not appear

Rebuild and reinstall:

```powershell
.\install.cmd
```

If the desktop app is open, close it first.

## Security Rules

Never:

- print raw access tokens
- print raw refresh tokens
- commit auth files
- paste full auth payloads into chat or PR descriptions

Prefer:

- structural summaries
- redacted values
- account id/email only when necessary

## Push Checklist

Before pushing:

```powershell
git status --short
```

Confirm:

- no auth files are tracked
- generated binaries are ignored
- desktop build artefacts are ignored
- only intended source/docs changes remain

## Short Version For Another Agent

If another agent only needs the shortest possible handoff, this is enough:

1. Install Go, Node.js, and Wails.
2. Ensure Codex already created `%USERPROFILE%\.codex\auth.json`.
3. Run [install.ps1](install.ps1).
4. Launch `codex-lover`.
5. Use the desktop app as the main UI.
6. Use `Add account` to add more accounts.
7. Use `Log in` and `Delete` on cards to manage accounts.
8. Keep the app open if you want notifications, auto-switch, and OpenCode sync to run.
