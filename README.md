# codex-lover

`codex-lover` is a local Windows-first account watcher and switcher for `codex`, with automatic OpenCode credential sync.

Current behavior:

- reads the active Codex account from `~/.codex/auth.json`
- fetches Codex usage from the same backend Codex uses
- keeps a local profile/state database in `~/.codex-lover`
- shows multiple previously-seen accounts in `watch`, including `ACTIVE` and `LOGGED OUT`
- keeps last known usage for logged-out accounts
- infers reset recovery for logged-out accounts from cached reset timestamps
- auto-syncs OpenCode OpenAI OAuth credentials to match the active Codex account
- auto-switches Codex to another cached account when the active account reaches limit

OpenCode is currently treated as a follower:

- Codex is the source of truth
- `watch` syncs OpenCode to the active Codex account
- auto-switch changes Codex first, then syncs OpenCode to the same account

## Status

This is still a prototype, but it is already useful for:

- tracking Codex accounts and quota in realtime
- keeping OpenCode aligned with Codex
- switching away from a limited Codex account without manually editing auth files

What is not finished yet:

- a formal OpenCode profile/import adapter
- a separate command for manual auto-switch policy tuning
- a polished TUI framework
- cross-platform packaging

## Requirements

Minimum environment:

- Windows
- Go installed
- Codex installed and already logged in at least once
- OpenCode installed if you want OpenCode sync

Observed paths on this machine:

- Codex auth: `%USERPROFILE%\.codex\auth.json`
- OpenCode auth: `%USERPROFILE%\.local\share\opencode\auth.json`
- codex-lover config/state: `%USERPROFILE%\.codex-lover`

## Installation

From the repo root:

```powershell
.\install.cmd
```

That script:

- finds `go.exe`
- builds `cmd/codex-lover`
- installs `codex-lover.exe` to `%LOCALAPPDATA%\codex-lover\bin`
- creates `codex-lover.cmd`
- adds `%LOCALAPPDATA%\codex-lover\bin` to your user `PATH`

If your current terminal was opened before installation, either:

```powershell
set PATH=%LOCALAPPDATA%\codex-lover\bin;%PATH%
```

or open a new terminal.

## Quick Start

### 1. Import your current Codex profile

If your active Codex login lives in the default location:

```powershell
codex-lover
```

Then choose:

```text
1. Import default Codex profile (~/.codex)
```

Or import manually:

```powershell
codex-lover profile import codex --label default --home %USERPROFILE%\.codex
```

### 2. Check current status

```powershell
codex-lover status
```

### 3. Start realtime watch

```powershell
codex-lover watch
```

`watch` is the main operational mode right now. It does more than rendering.

While `watch` is running it will:

- refresh Codex usage every 15 seconds
- blink the live dot on the active account
- show `ACTIVE`, `LOGGED OUT`, `CACHED`, `FRESH`, and `ERROR` states
- sync OpenCode OpenAI OAuth to the active Codex account
- auto-switch Codex if the active account reaches limit and another cached account is ready

## Commands

Currently implemented:

```powershell
codex-lover
codex-lover profile import codex --label NAME --home PATH
codex-lover profile list
codex-lover refresh
codex-lover status
codex-lover watch
codex-lover daemon
codex-lover daemon-status
```

## How Watch Works

`watch` is effectively a small control loop.

Each refresh cycle:

1. read the current active Codex auth from `~/.codex/auth.json`
2. fetch usage
3. update local profile state
4. cache the active Codex auth for future switching
5. if the active account is limited, try to restore another cached account into `~/.codex/auth.json`
6. sync OpenCode OpenAI auth to match the active Codex account
7. redraw the terminal UI

The UI includes status lines above the cards, for example:

- `Auto switch: standing by`
- `Auto switch: default -> account-b`
- `OpenCode sync: already on 31a4a...`
- `OpenCode sync: updated OpenAI oauth for 31a4a...`

## Multi-Account Model

The project supports one shared Codex home that changes accounts over time.

If you:

- log in as account A
- later log out
- later log in as account B

then `codex-lover` will keep both accounts visible in status/watch as long as it has seen them before.

You will see:

- one `ACTIVE` account
- older accounts as `LOGGED OUT`
- cached usage for logged-out accounts
- reset inference for logged-out accounts when the stored reset time has passed

## Auto-Switch Rules

Current auto-switch rules are intentionally conservative.

The active Codex account is considered limited when:

- primary window is effectively at 0%
- or secondary window is effectively at 0%

When that happens, `watch` tries to switch to the best candidate that:

- belongs to Codex
- is in the same home group
- has cached credentials
- still has usable quota according to current or inferred data

The candidate score is the minimum remaining percent across its 5H and weekly windows.

Important limitation:

- `codex-lover` can only auto-switch to accounts whose Codex auth has already been cached locally

That means:

- the currently active account is cached automatically
- an older logged-out account that was only observed in the UI but never cached with credentials cannot be switched to yet
- to make an account switchable later, it needs to become active at least once while running this newer build

Cached auth files are stored at:

```text
%USERPROFILE%\.codex-lover\codex-auth
```

Example:

```text
%USERPROFILE%\.codex-lover\codex-auth\codex-example-profile.json
```

## OpenCode Sync

OpenCode sync is automatic while `watch` runs.

Current implementation:

- reads the active Codex account
- writes OpenCode `openai` OAuth credentials to `~/.local/share/opencode/auth.json`
- preserves other OpenCode providers such as `google`, `openrouter`, and `anthropic`
- skips file writes when nothing changed
- creates a backup before overwriting OpenCode auth

OpenCode sync direction is one-way:

- Codex -> OpenCode

If OpenCode is logged into a different OpenAI account, `watch` will sync it back to the active Codex account on the next refresh.

## Local Files

Runtime data lives outside the repo:

- config: `%USERPROFILE%\.codex-lover\config.json`
- state: `%USERPROFILE%\.codex-lover\state.json`
- cached Codex auth: `%USERPROFILE%\.codex-lover\codex-auth\`

The repo itself should not contain real auth files.

## Development

Build locally:

```powershell
& 'C:\Program Files\Go\bin\go.exe' build .\cmd\codex-lover
```

Format:

```powershell
& 'C:\Program Files\Go\bin\gofmt.exe' -w .\internal\app\app.go
```

Install locally after code changes:

```powershell
.\install.cmd
```

## Troubleshooting

### `codex-lover status` shows no profiles

Import a profile first:

```powershell
codex-lover profile import codex --label default --home %USERPROFILE%\.codex
```

### OpenCode does not match Codex

Run:

```powershell
codex-lover watch
```

If the sync line says `updated`, OpenCode was rewritten to the active Codex account.

### Auto-switch never happens

Common reasons:

- the active account has not actually hit limit yet
- the other accounts only have cached usage, not cached credentials
- no other cached account currently has usable quota

### Account appears in watch but cannot be switched to

That account likely has usage history but no credential cache yet.

Make it active once, let `watch` or `status` run, and it will be cached for later switching.

## Roadmap

Near-term:

- better switch heuristics
- manual switch command
- full OpenCode adapter
- background daemon + shim integration
- better TUI rendering

Design notes live in [PLAN.md](./PLAN.md).
