# AGENTS.md

This file is for coding agents working on `codex-lover`.

It explains what the project does, how to set it up from scratch, what files matter, and what assumptions are currently true. An agent should be able to read only this file plus the referenced source files and get productive quickly.

## Project Summary

`codex-lover` is a local Go CLI for monitoring and controlling Codex account usage on Windows.

Current scope:

- imports Codex profiles from local auth storage
- fetches Codex usage from OpenAI backend endpoints
- tracks multiple accounts observed over time in one shared Codex home
- shows realtime terminal cards in `watch`
- keeps logged-out accounts visible with cached usage
- infers quota recovery for logged-out accounts when reset time passes
- auto-syncs OpenCode OpenAI OAuth credentials to match the active Codex account
- auto-switches Codex to another cached account when the active account reaches limit

The project is still a prototype. There is no full shim-based launch flow yet.

## Core Product Behavior

The current mental model is:

- Codex is the source of truth
- OpenCode follows Codex
- `watch` is not just a viewer, it is the main control loop

When `codex-lover watch` runs, it:

1. refreshes Codex usage
2. updates the local profile/state store
3. caches active Codex auth for future switching
4. auto-switches Codex if the active account is limited and a better cached account is available
5. syncs OpenCode OpenAI OAuth to the active Codex account
6. redraws the watch UI

## Important Current Limitation

Auto-switch only works for accounts whose Codex auth was cached locally before.

That means:

- an account that is currently active gets cached automatically
- a logged-out account can still appear in the UI from cached usage alone
- but that account cannot be auto-switched into until it has been active at least once while this build was running

This distinction matters. Do not describe "visible in watch" as equivalent to "switchable".

## Environment Assumptions

Observed working environment on the target machine:

- OS: Windows
- shell: PowerShell
- Go installed at `C:\Program Files\Go\bin`
- Codex auth at `%USERPROFILE%\.codex\auth.json`
- OpenCode auth at `%USERPROFILE%\.local\share\opencode\auth.json`
- runtime state at `%USERPROFILE%\.codex-lover`

If an agent sees a different environment, it should verify paths before making assumptions.

## Repo Layout

Top-level files:

- `README.md`: end-user overview and usage
- `PLAN.md`: broader roadmap and design direction
- `AGENTS.md`: this file
- `install.cmd`: Windows install wrapper
- `install.ps1`: builds and installs `codex-lover.exe`
- `go.mod`: module definition

Entrypoint:

- `cmd/codex-lover/main.go`

Main packages:

- `internal/app`: CLI and watch UI
- `internal/service`: refresh logic, selection logic, OpenCode sync orchestration
- `internal/store`: local config/state persistence in `~/.codex-lover`
- `internal/codex`: Codex auth loading, usage fetch, auth cache/restore
- `internal/opencode`: OpenCode auth sync helper
- `internal/model`: shared types
- `internal/daemon`: daemon HTTP endpoints

## Files That Matter Most

If you are changing behavior, start here:

- `internal/app/app.go`
  Purpose:
  CLI commands, watch loop, terminal rendering, watch status text.

- `internal/service/service.go`
  Purpose:
  refresh logic, active account detection, auto-switch decision making, OpenCode sync orchestration.

- `internal/codex/auth.go`
  Purpose:
  reads `~/.codex/auth.json`, extracts account info, computes auth fingerprints.

- `internal/codex/usage.go`
  Purpose:
  fetches usage from the Codex/OpenAI backend and refreshes tokens when needed.

- `internal/codex/cache.go`
  Purpose:
  stores/restores cached Codex auth files used for auto-switch.

- `internal/opencode/sync.go`
  Purpose:
  copies active Codex OpenAI OAuth credentials into OpenCode auth storage.

- `internal/model/types.go`
  Purpose:
  status model, auth state, usage windows.

## Local Runtime Files

These are not part of the repo, but the code depends on them:

- `%USERPROFILE%\.codex\auth.json`
  Active Codex auth.

- `%USERPROFILE%\.local\share\opencode\auth.json`
  OpenCode provider credentials.

- `%USERPROFILE%\.codex-lover\config.json`
  Imported profiles and runtime config.

- `%USERPROFILE%\.codex-lover\state.json`
  Cached profile state and usage history.

- `%USERPROFILE%\.codex-lover\codex-auth\*.json`
  Cached Codex auth copies used for auto-switch.

Do not commit or print secrets from these files.

## Setup From Scratch

This section is written for another agent that needs to install the project on a fresh machine and get it running end-to-end.

### 1. Clone or unpack the repo

Place the repo somewhere writable, for example:

```powershell
git clone https://github.com/example/codex-lover.git
cd codex-lover
```

If git is unavailable, copy the repo contents manually.

### 2. Verify Go

Expected:

```powershell
& 'C:\Program Files\Go\bin\go.exe' version
```

If that path does not exist:

- install Go
- or adjust commands to the local Go path

### 3. Verify Codex auth exists

The project expects a file-backed Codex login:

```powershell
Test-Path "$env:USERPROFILE\.codex\auth.json"
```

If false:

- run Codex normally
- log in once
- confirm `auth.json` appears

### 4. Optionally verify OpenCode

If OpenCode sync matters:

```powershell
Get-Command opencode -ErrorAction SilentlyContinue
```

And check:

```powershell
Test-Path "$env:USERPROFILE\.local\share\opencode\auth.json"
```

OpenCode is optional for the basic Codex watch flow.

### 5. Build the project

From repo root:

```powershell
& 'C:\Program Files\Go\bin\go.exe' build .\cmd\codex-lover
```

### 6. Install the local binary

Use the provided installer:

```powershell
.\install.cmd
```

Or:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\install.ps1
```

This installs the binary to:

```text
%LOCALAPPDATA%\codex-lover\bin
```

### 7. Refresh the shell PATH

If still in the same shell:

```powershell
set PATH=%LOCALAPPDATA%\codex-lover\bin;%PATH%
```

Or open a new terminal.

### 8. Import the active Codex profile

Menu flow:

```powershell
codex-lover
```

Choose:

```text
1. Import default Codex profile (~/.codex)
```

Or direct CLI:

```powershell
codex-lover profile import codex --label default --home %USERPROFILE%\.codex
```

### 9. Confirm status

```powershell
codex-lover status
```

Expected:

- a card for the active account
- quota bars
- `ACTIVE` auth tag once refresh has run

### 10. Start the realtime control loop

```powershell
codex-lover watch
```

Expected:

- active account card with a blinking red dot in the header
- an `Auto switch:` line
- an `OpenCode sync:` line
- logged-out accounts shown with cached or inferred quota

## What “Working” Means

A healthy current setup should show:

- `Auto switch: standing by`
- `OpenCode sync: already on ...` or `updated ...`
- one `ACTIVE` Codex account
- any previously seen accounts as `LOGGED OUT`

If the active account has already hit limit and another cached account is ready, `watch` should eventually show:

- `Auto switch: account-a -> account-b`

and then OpenCode should sync to the new active account.

## Security and Secret Handling

Do not:

- print raw access tokens
- print raw refresh tokens
- paste auth.json contents into chat
- commit any auth file into git

It is acceptable to inspect:

- account IDs
- emails
- token expiry timestamps
- provider names
- whether files exist

When discussing auth files, prefer redacted or structural summaries only.

## Development Conventions

When editing:

- use `apply_patch` for source changes
- keep ASCII by default
- avoid unnecessary dependencies
- keep Windows behavior in mind

When validating:

- run `gofmt`
- run `go build .\cmd\codex-lover`
- if behavior depends on install paths, rerun `install.ps1`

Recommended validation loop:

```powershell
& 'C:\Program Files\Go\bin\gofmt.exe' -w .\internal\app\app.go
& 'C:\Program Files\Go\bin\go.exe' build .\cmd\codex-lover
.\install.cmd
codex-lover status
codex-lover watch
```

## Current Watch-Specific Behavior

The watch loop has two clocks:

- usage refresh: every 15 seconds
- live dot blink redraw: every 500 ms

OpenCode sync is already optimized:

- sync check runs when watch refreshes status
- file writes are skipped if Codex active auth fingerprint did not change
- UI still shows the last sync status

This means watch is safe to leave running continuously.

## Common Tasks For Future Agents

### Add a new watch status line

Start in:

- `internal/app/app.go`

Look for:

- `printWatch`
- `syncOpenCodeFromWatch`
- `autoSwitchCodexFromWatch`

### Adjust auto-switch policy

Start in:

- `internal/service/service.go`

Look for:

- `AutoSwitchLimitedCodex`
- `bestSwitchCandidate`
- `quotaScore`
- `usageLimitReached`

### Change how Codex auth is cached/restored

Start in:

- `internal/codex/cache.go`

### Change OpenCode sync behavior

Start in:

- `internal/opencode/sync.go`

### Change how usage is displayed for logged-out accounts

Start in:

- `internal/service/service.go`
- `internal/app/app.go`

Look for:

- `EffectiveWindowForDisplay`
- `WindowResetInferred`
- `renderUsageLine`

## Known Limitations

- Auto-switch only works for accounts with cached Codex auth.
- OpenCode follows Codex; reverse sync is not implemented.
- Existing running OpenCode/Codex sessions may not hot-reload auth mid-process.
- The UI is still ANSI-rendered text, not a full TUI framework.

## If You Need To Push This Repo From Scratch

If the repo is not yet a git repo:

```powershell
git init
git branch -M main
git remote add origin https://github.com/example/codex-lover.git
git add .
git commit -m "Initial commit"
git push -u origin main
```

Before pushing:

- confirm no auth files are tracked
- confirm `*.exe`, backups, and temp files are ignored
- check `git status --short`

## Reference Documents

- `README.md`: user-facing install and usage
- `PLAN.md`: roadmap and broader design direction
