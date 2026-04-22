# AGENTS.md

This file is for coding agents working on `codex-lover`.

Read this file first if you need to:

- understand the Ubuntu branch quickly
- install the project from scratch on another Ubuntu machine
- explain the product to a user
- modify the supported command surface safely
- debug daemon, account, watch, or OpenCode sync behavior

## Branch Scope

This branch is Ubuntu-first and headless.

Current supported model:

- background daemon
- CLI commands
- terminal dashboard
- local JSON/API integration

Out of scope for this branch:

- Windows setup instructions
- Windows desktop UX documentation
- telling users to rely on a desktop app as the primary workflow

If you see old desktop or Windows-specific code in the repo, do not document it as the supported flow for this branch unless the user explicitly asks about another branch.

## What This Project Is

`codex-lover` manages multi-account Codex usage on Ubuntu.

The problem it solves:

- only one Codex account can be active in `~/.codex/auth.json` at once
- people may use multiple Codex accounts
- manual switching is annoying
- logged-out accounts still matter because their cached quota and reset times matter
- OpenCode can drift onto a different account than the currently active Codex auth

`codex-lover` keeps a local registry of known accounts, caches switchable auth, refreshes usage, preserves logged-out account visibility, auto-switches when the active account is limited, and keeps OpenCode aligned to the active Codex account.

## Current Product Model

Current source of truth:

- Codex is the source of truth.
- OpenCode follows Codex.
- The daemon is the main live control loop.

Current UX model:

- `codex-lover` with no args prints one-shot status
- `codex-lover server run` runs the daemon in the foreground
- `codex-lover server start` runs it in the background
- `codex-lover watch` is the live terminal dashboard
- `codex-lover account ...` manages accounts from the shell
- JSON and HTTP API endpoints are supported for integrations

Important limitation:

- visible is not the same as switchable
- an account is only switchable if cached auth exists for it, or for another equivalent profile representing the same account
- auto-switch only works when another cached account has usable quota

## User-Facing Features

Current supported features:

- background daemon with foreground and managed-background modes
- `status` and `refresh` in text or JSON
- colorized `watch` dashboard with `5h` and `weekly` bars for every account
- `account add` through isolated `codex login --device-auth`
- `account list`
- `account switch`
- `account remove` and `account delete`
- `profile import codex --label NAME --home PATH`
- `profile list`
- auto-switch on limit
- OpenCode sync from active Codex account
- local HTTP API for status and account actions

## Fresh Ubuntu Setup Playbook

This is the direct playbook another agent should follow on a new Ubuntu machine.

### 1. Open the repo

```bash
git clone https://github.com/honam867/codex-lover.git
cd codex-lover
```

If the repo is already present, just open it in a writable working directory.

### 2. Verify Go

```bash
go version
```

If Go is missing:

```bash
sudo apt update
sudo apt install -y golang-go
```

### 3. Verify Codex CLI

```bash
codex --version
```

If this fails, stop and fix Codex installation first.

### 4. Verify a base Codex login already exists

```bash
test -f ~/.codex/auth.json && echo ok || echo missing
```

If the file is missing:

- open Codex normally
- log in once
- confirm `~/.codex/auth.json` exists

### 5. Optional: verify OpenCode

```bash
test -f ~/.local/share/opencode/auth.json && echo present || echo missing
```

OpenCode is optional. The sync code can create or update the file later when an active Codex account exists.

### 6. Install codex-lover

```bash
bash ./install.sh
```

What the install script does right now:

- builds `./cmd/codex-lover`
- installs the binary to `~/.local/bin/codex-lover`
- appends a `.bashrc` auto-start snippet if absent
- starts the background daemon once immediately
- writes daemon logs to `~/.codex-lover/logs/server.log`

### 7. Verify `PATH`

```bash
which codex-lover
```

If that prints nothing, add this to the shell config:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Then reload the shell.

Important:

- `install.sh` only edits `.bashrc`
- if the user uses `zsh` or `fish`, either add the same logic there or use systemd user service startup instead

### 8. Verify the daemon

```bash
codex-lover server status
```

Expected result:

- `server: running`
- `address: http://127.0.0.1:47070`
- a valid pid
- a log path under `~/.codex-lover/logs/server.log`

### 9. Verify the CLI views

```bash
codex-lover status
codex-lover account list
codex-lover watch
```

Expected result:

- `status` prints one-shot text output
- `account list` prints `id`, `auth`, `switchable`, `5h`, and `weekly`
- `watch` opens a colorized live terminal dashboard

### 10. Add another account

```bash
codex-lover account add
```

Expected flow:

- `codex-lover` creates an isolated managed login home under `~/.codex-lover/homes/codex/...`
- it runs `codex login --device-auth`
- login completes in the terminal/device-auth flow
- the new account is imported and cached
- temporary managed login files are cleaned up

### 11. Verify manual account management

List accounts:

```bash
codex-lover account list
```

Switch using the chosen id:

```bash
codex-lover account switch <profile-id>
```

Remove using the chosen id:

```bash
codex-lover account remove <profile-id>
```

### 12. Optional: enable systemd user service

```bash
mkdir -p ~/.config/systemd/user
cp docs/systemd/codex-lover.service ~/.config/systemd/user/codex-lover.service
systemctl --user daemon-reload
systemctl --user enable --now codex-lover
```

Logs:

```bash
journalctl --user -fu codex-lover
```

## How To Explain Usage To A User

If another agent needs a quick explanation, use this model:

- `codex-lover` shows current account state
- `codex-lover server start` keeps automation running in the background
- `codex-lover watch` is the live dashboard
- `codex-lover account add` adds another account without logging out the current one first
- `codex-lover account switch <profile-id>` makes a cached account active
- `codex-lover account remove <profile-id>` removes local data and cached auth for that account
- auto-switch happens when the active account reaches limit and another cached account is ready
- OpenCode follows the active Codex account automatically

Do not tell users that any visible account can always be switched into. Switchability depends on cached auth.

## Command Surface

Primary commands:

```bash
codex-lover
codex-lover help
codex-lover status
codex-lover status --json
codex-lover refresh
codex-lover refresh --json
codex-lover watch
codex-lover server run
codex-lover server start
codex-lover server stop
codex-lover server status
codex-lover account add
codex-lover account list
codex-lover account list --json
codex-lover account switch <profile-id>
codex-lover account remove <profile-id>
codex-lover profile import codex --label NAME --home PATH
codex-lover profile list
```

Compatibility aliases also exist:

- `codex-lover run` -> `server run`
- `codex-lover daemon` -> `server run`
- `codex-lover daemon-status` -> `status`

## Runtime And Data Files

Important files outside the repo:

- `~/.codex/auth.json`: active runtime Codex auth
- `~/.local/share/opencode/auth.json`: OpenCode auth file that follows Codex
- `~/.codex-lover/config.json`: config and registered profiles
- `~/.codex-lover/state.json`: cached profile state and usage data
- `~/.codex-lover/codex-auth/`: cached auth used for switch and auto-switch
- `~/.codex-lover/homes/codex/`: temporary managed login homes for `account add`
- `~/.codex-lover/logs/server.log`: managed daemon log
- `~/.codex-lover/server.pid`: managed daemon pid file

Config defaults from code:

- `poll_interval_seconds`: `15`
- `daemon.listen_address`: `127.0.0.1:47070`

## Repo Layout

Top-level docs and install files:

- [README.md](./README.md): end-user Ubuntu documentation
- [AGENTS.md](./AGENTS.md): this file
- [docs/ubuntu.md](./docs/ubuntu.md): shorter Ubuntu quick reference
- [install.sh](./install.sh): user-space Ubuntu install flow
- [docs/systemd/codex-lover.service](./docs/systemd/codex-lover.service): sample systemd user unit

CLI and command routing:

- [cmd/codex-lover/main.go](./cmd/codex-lover/main.go): CLI entrypoint
- [internal/app/app.go](./internal/app/app.go): command routing, status output, account and profile commands
- [internal/app/watch.go](./internal/app/watch.go): live terminal dashboard rendering
- [internal/app/server_manage.go](./internal/app/server_manage.go): background daemon start/stop/status
- [internal/app/remote.go](./internal/app/remote.go): daemon client used by CLI commands

Daemon and core logic:

- [internal/daemon/server.go](./internal/daemon/server.go): HTTP API and runtime loop
- [internal/service/service.go](./internal/service/service.go): refresh, activation, logout, auto-switch, OpenCode sync wiring
- [internal/store/store.go](./internal/store/store.go): persistent config and state storage
- [internal/model/types.go](./internal/model/types.go): config/state/profile data model

Codex and OpenCode integration:

- [internal/codex/auth.go](./internal/codex/auth.go): load runtime auth and derive profile identity
- [internal/codex/usage.go](./internal/codex/usage.go): fetch usage data
- [internal/codex/cache.go](./internal/codex/cache.go): cache and restore auth files
- [internal/opencode/sync.go](./internal/opencode/sync.go): sync active Codex OAuth tokens into OpenCode auth

## Maintenance Entry Points

When changing user-facing commands:

- start with `internal/app/app.go`

When changing the live dashboard:

- start with `internal/app/watch.go`

When changing background daemon lifecycle:

- start with `internal/app/server_manage.go`
- also inspect `internal/app/remote.go`

When changing poll behavior, API output, or automation flow:

- start with `internal/daemon/server.go`

When changing business rules:

- start with `internal/service/service.go`

Key methods to know:

- `RefreshAll`
- `RefreshLoggedOutCachedUsage`
- `ActivateProfile`
- `LogoutProfile`
- `AutoSwitchLimitedCodex`
- `SyncOpenCodeFromActiveCodex`

When changing install behavior:

- start with `install.sh`
- also inspect `docs/systemd/codex-lover.service`

## Verification Workflow

Recommended loop:

```bash
go test ./...
go build ./cmd/codex-lover
install -m 0755 ./codex-lover ~/.local/bin/codex-lover
codex-lover server run
```

Useful validation commands:

```bash
codex-lover server status
codex-lover status
codex-lover account list
codex-lover watch
```

If the change touches OpenCode sync, validate at least these facts without exposing raw tokens:

- OpenCode auth file exists or is created
- `openai.type` is `oauth`
- `openai.accountId` matches the active Codex account id

## Troubleshooting

### `watch` or `status` does not reflect the newest build

Likely cause:

- an old installed binary is still being used

Check:

```bash
which codex-lover
stat ~/.local/bin/codex-lover
```

Then reinstall and restart the command.

### Auto-switch does not happen

Check these conditions:

- the active account actually reached effective limit
- another account has cached auth
- another account still has usable quota
- the daemon is actually running

### Manual switch fails

Most likely cause:

- the target account has no cached credentials

Check `switchable: yes` in `codex-lover account list`.

### OpenCode does not change

Check these conditions:

- there is an active Codex account
- daemon refresh has run
- `~/.local/share/opencode/auth.json` is writable
- `openai.accountId` matches the active Codex account id

### `server start` or `server status` behaves oddly

Check that `pgrep` is installed:

```bash
pgrep --version
```

On Ubuntu that usually comes from `procps`.

## Security Rules

Never:

- print raw access tokens
- print raw refresh tokens
- commit auth files
- paste full auth payloads into chat or docs

Prefer:

- account ids
- emails if needed
- expiry timestamps
- redacted or structural summaries

## Push Checklist

Before pushing:

```bash
git status --short
```

Confirm:

- no auth files are tracked
- generated binaries are not accidentally staged
- docs match the Ubuntu branch, not a Windows desktop flow
- only intended source and doc changes remain

## Short Version For Another Agent

If another agent only needs the shortest possible handoff, this is enough:

1. This branch is Ubuntu headless, not Windows desktop.
2. Install Go and Codex, and make sure `~/.codex/auth.json` already exists.
3. Run `bash ./install.sh`.
4. Ensure `~/.local/bin` is on `PATH`.
5. Keep the daemon running with `codex-lover server start` or systemd user service.
6. Use `codex-lover watch` for the live dashboard.
7. Use `codex-lover account add`, `account switch`, and `account remove` for account management.
8. Treat OpenCode as a follower of the active Codex account.
9. Never promise switching for accounts that do not have cached auth.
