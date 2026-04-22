# codex-lover

> Ubuntu branch: headless multi-account `codex` manager with a local daemon, terminal dashboard, manual switching, auto-switch, and OpenCode sync.

## Scope

This branch is for Ubuntu and VPS usage.

- The supported workflow is terminal-first.
- The supported runtime is a local background daemon plus CLI commands.
- Windows desktop documentation has been intentionally removed from this branch.
- If the repo still contains old desktop-related code, treat it as out of scope for this Ubuntu branch unless you are explicitly working on another branch.

## What codex-lover Does

`codex-lover` exists for people who use multiple Codex accounts but only have one live runtime auth file at a time.

It helps with these problems:

- only one Codex account can be active in `~/.codex/auth.json` at once
- switching manually is repetitive and error-prone
- logged-out accounts disappear from normal CLI state even though their last known quota still matters
- once the active account hits the 5H or weekly limit, switching cleanly takes manual work
- `opencode` can drift away from the currently active Codex account

Current supported behavior:

- track known Codex accounts in local state
- list accounts with `auth`, `switchable`, `5h`, and `weekly` information
- keep logged-out accounts visible with cached usage when cached auth exists
- infer reset recovery for logged-out accounts when reset time has already passed
- add a new account through an isolated device-auth login flow
- manually switch to another cached account
- remove an account from local registry and cached auth
- run a background daemon that refreshes usage on a schedule
- auto-switch when the active account reaches effective limit and another cached account is ready
- sync OpenCode to the active Codex account
- expose machine-readable JSON output and a small local HTTP API

## Product Model

Current source of truth:

- Codex is the source of truth.
- OpenCode follows Codex.
- The daemon is the main live control loop.

Current UX model:

- `codex-lover` with no arguments prints a one-shot status summary
- `codex-lover server run` runs the live loop in the foreground
- `codex-lover server start` runs the live loop in the background
- `codex-lover watch` renders a live terminal dashboard
- `codex-lover account ...` manages accounts from the shell
- `codex-lover status --json` and `codex-lover account list --json` support scripting and integrations

Important limitation:

- visible does not mean switchable
- an account is only switchable if cached auth exists for it or for an equivalent known profile representing the same account
- auto-switch only works when another cached account still has usable quota

## Requirements

Required:

- Ubuntu or a compatible Linux distribution
- Bash if you want the install script's `.bashrc` auto-start behavior
- Go installed
- `codex` installed and available in `PATH`
- at least one working Codex login already present in `~/.codex/auth.json`

Optional:

- `opencode` if you want OpenCode sync
- `systemd --user` if you want a persistent user service instead of shell auto-start

Useful system tools:

- `pgrep` from `procps` is used by daemon management commands

## Install From Source

### 1. Clone the repo

```bash
git clone https://github.com/honam867/codex-lover.git
cd codex-lover
```

### 2. Verify Go

```bash
go version
```

If Go is missing on Ubuntu:

```bash
sudo apt update
sudo apt install -y golang-go
```

### 3. Verify Codex CLI

```bash
codex --version
```

If `codex` is not in `PATH`, fix that first.

### 4. Verify the machine already has one Codex login

```bash
test -f ~/.codex/auth.json && echo ok || echo missing
```

If the file is missing, open Codex normally once and finish a regular login before using `codex-lover`.

### 5. Optional: verify OpenCode

```bash
test -f ~/.local/share/opencode/auth.json && echo present || echo missing
```

OpenCode is optional. If it is missing, `codex-lover` can still create or update the auth file later when sync runs.

### 6. Build and install

```bash
bash ./install.sh
```

`install.sh` currently does all of this:

- builds `./cmd/codex-lover`
- installs the binary to `~/.local/bin/codex-lover`
- appends a small auto-start block to `~/.bashrc` if it is not already there
- starts the background daemon once immediately
- writes daemon logs to `~/.codex-lover/logs/server.log`

### 7. Make sure `~/.local/bin` is on `PATH`

Check:

```bash
printf '%s\n' "$PATH"
```

If needed, add this to your shell config:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Then reload your shell or run:

```bash
source ~/.bashrc
```

Note:

- the installer only auto-edits `.bashrc`
- if you use `zsh`, `fish`, or another shell, add the `PATH` update and daemon auto-start yourself or use the provided systemd user service

## First-Time Validation

Run these after installation:

```bash
codex-lover server status
codex-lover status
codex-lover account list
codex-lover watch
```

Expected results:

- `server status` shows `running`, an address, a pid, and a log path
- `status` prints at least one Codex account
- `account list` prints each account with `id`, `auth`, `5h`, and `weekly`
- `watch` opens a live terminal dashboard with colored badges and progress bars

## Command Reference

### Core commands

```bash
codex-lover
codex-lover help
codex-lover status
codex-lover status --json
codex-lover refresh
codex-lover refresh --json
codex-lover watch
```

Meaning:

- `codex-lover`: same as a one-shot `status`
- `status`: prints current known account state
- `status --json`: prints machine-readable statuses
- `refresh`: forces a refresh and prints the result
- `watch`: shows a live terminal dashboard

### Daemon commands

```bash
codex-lover server run
codex-lover server start
codex-lover server stop
codex-lover server status
```

Meaning:

- `server run`: foreground daemon with live logs in the current terminal
- `server start`: background daemon managed by a pid file and a log file
- `server stop`: stops the managed background daemon
- `server status`: prints `running` or `stopped`, address, pid, and log path

Compatibility aliases:

```bash
codex-lover run
codex-lover daemon
codex-lover daemon-status
```

Meaning:

- `run`: alias for `server run`
- `daemon`: alias for `server run`
- `daemon-status`: alias for `status`

### Account commands

```bash
codex-lover account add
codex-lover account add codex
codex-lover account list
codex-lover account list --json
codex-lover account switch <profile-id>
codex-lover account remove <profile-id>
codex-lover account delete <profile-id>
```

Meaning:

- `account add`: starts an isolated `codex login --device-auth` flow and imports the new account
- `account list`: prints accounts with switchability and quota summaries
- `account switch`: restores cached auth for the selected account into the runtime Codex home
- `account remove` or `delete`: removes cached auth and local registry/state for that account

### Profile commands

```bash
codex-lover profile import codex --label NAME --home PATH
codex-lover profile list
codex-lover profile list --json
```

Meaning:

- `profile import codex`: imports an existing Codex home from disk
- `profile list`: currently reuses the same output as `account list`

## Typical Flows

### Keep the daemon running

Recommended daily setup:

```bash
codex-lover server start
codex-lover server status
```

For debugging with foreground logs:

```bash
codex-lover server run
```

### Add a new account

```bash
codex-lover account add
```

What happens:

- `codex-lover` allocates a managed temporary home under `~/.codex-lover/homes/codex/...`
- it runs `codex login --device-auth`
- the login flow writes a temporary `auth.json` inside that managed home
- the new account is imported into local state
- the auth is cached under `~/.codex-lover/codex-auth`
- the temporary managed home is cleaned up

Important behavior:

- the active runtime account does not need to be logged out first
- account add is interactive and must run in a real terminal

### List accounts

```bash
codex-lover account list
```

Current text output includes:

- active marker
- label or email
- `id`
- `auth`
- `switchable` for non-active accounts
- `5h`
- `weekly`

### Manually switch accounts

1. List accounts:

```bash
codex-lover account list
```

2. Copy the target `id`.

3. Switch:

```bash
codex-lover account switch <profile-id>
```

Example:

```bash
codex-lover account switch codex-some-account-id
```

If switching fails because the account is visible but not cached, you will get a cached-credentials error. In that case the account is not currently switchable.

### Remove an account

1. List accounts and choose the `id`.

2. Remove it:

```bash
codex-lover account remove <profile-id>
```

This removes:

- local profile registry entry
- local state entry
- cached auth for that account
- runtime auth too, if that exact account is currently active and matches the live auth file

### Import an existing Codex home

```bash
codex-lover profile import codex --label work --home /absolute/path/to/.codex
```

Notes:

- `--home` is required
- `--label` is optional
- if `--home` is relative, `codex-lover` resolves it to an absolute path first

## Runtime Automation

The daemon is the component that performs ongoing work.

Current loop behavior:

- refresh live state every 15 seconds by default
- refresh logged-out cached usage every 15 minutes
- warm logged-out cached usage on startup, manual refresh, and account change flows
- auto-switch when the active account reaches effective limit and a better cached candidate exists
- sync OpenCode from the active Codex account
- emit one summary log line per cycle

Default config values:

- poll interval: `15`
- daemon listen address: `127.0.0.1:47070`

These values live in `~/.codex-lover/config.json`.

## Watch Dashboard

`codex-lover watch` is the human-friendly live terminal view.

Current behavior:

- refreshes every 15 seconds
- clears and redraws the terminal
- shows the active account summary first
- highlights account state with ANSI colors when the terminal supports them
- displays both `5h` and `weekly` progress bars for every account
- marks whether non-active accounts are switchable
- shows cached, fresh, and error states more clearly than plain `status`

Color behavior:

- if `NO_COLOR` is set, colors are disabled
- if `TERM=dumb`, colors are disabled

## OpenCode Sync

OpenCode sync is implemented and active in the daemon/runtime loop.

Current behavior:

- the active Codex account is the only source for sync
- `codex-lover` writes the OpenAI OAuth section inside `~/.local/share/opencode/auth.json`
- when the contents actually change, the previous file is backed up first
- if the file already matches the active Codex account, sync is a no-op

Practical meaning:

- after refresh, switch, or auto-switch, OpenCode should follow the same account as the active Codex runtime auth
- if there is no active Codex account, OpenCode sync stays idle

## Local HTTP API

The daemon exposes a small local API at the configured listen address.

Default base URL:

```text
http://127.0.0.1:47070
```

Current endpoints:

- `GET /v1/status`
- `POST /v1/refresh`
- `GET /v1/accounts`
- `POST /v1/accounts/switch`
- `POST /v1/accounts/remove`

The CLI uses this API when the daemon is available. Many commands also try to start the daemon automatically if it is not already running.

## Runtime Files And Data

Important files outside the repo:

- `~/.codex/auth.json`: active Codex runtime auth
- `~/.local/share/opencode/auth.json`: OpenCode auth file that follows the active Codex account
- `~/.codex-lover/config.json`: daemon config and registered profiles
- `~/.codex-lover/state.json`: cached profile state and usage snapshots
- `~/.codex-lover/codex-auth/`: cached Codex auth copies used for switching and auto-switch
- `~/.codex-lover/homes/codex/`: temporary managed homes used during `account add`
- `~/.codex-lover/logs/server.log`: background daemon log
- `~/.codex-lover/server.pid`: managed background daemon pid file

Security rules:

- do not commit auth files
- do not paste raw access tokens or refresh tokens into issues, chats, or commits
- when debugging, prefer account ids, emails, expiry timestamps, and high-level summaries

## Systemd User Service

A sample unit file exists at `docs/systemd/codex-lover.service`.

Install it like this:

```bash
mkdir -p ~/.config/systemd/user
cp docs/systemd/codex-lover.service ~/.config/systemd/user/codex-lover.service
systemctl --user daemon-reload
systemctl --user enable --now codex-lover
```

View logs:

```bash
journalctl --user -fu codex-lover
```

This is a good option if you do not want shell-based auto-start.

## Troubleshooting

### `codex-lover` command is not found

Check that `~/.local/bin` is on `PATH`.

Verify:

```bash
which codex-lover
```

### `watch` still shows old behavior after a reinstall

You are probably still running an old process.

Stop the running command with `Ctrl+C`, then run it again:

```bash
codex-lover watch
```

If needed, reinstall again:

```bash
bash ./install.sh
```

### `account add` looks stuck

That command is interactive by design.

- it opens the normal device-auth flow for Codex
- it expects you to finish the browser or device-code login
- the terminal will not finish until the login flow exits

### Auto-switch does not happen

Common causes:

- the active account has not actually reached effective limit yet
- no other cached account is ready
- the visible target account has usage data but does not have cached auth
- no candidate account still has usable quota

### Manual switch fails

The most common cause is missing cached auth.

Check `switchable: yes` in `codex-lover account list` before switching.

### OpenCode does not seem to follow Codex

Check these:

- an active Codex account exists
- the daemon is running or you have forced a refresh
- `~/.local/share/opencode/auth.json` is writable
- the active Codex account id matches `openai.accountId` in the OpenCode auth file

### `server start`, `server stop`, or `server status` behave strangely

Make sure `pgrep` is available:

```bash
pgrep --version
```

If needed on Ubuntu:

```bash
sudo apt install -y procps
```

## Development

Typical local loop:

```bash
go test ./...
go build ./cmd/codex-lover
install -m 0755 ./codex-lover ~/.local/bin/codex-lover
codex-lover server run
```

Useful source entry points:

- `cmd/codex-lover/main.go`: CLI entrypoint
- `internal/app/app.go`: command routing and text output
- `internal/app/watch.go`: watch dashboard rendering
- `internal/app/server_manage.go`: background daemon management
- `internal/app/remote.go`: local HTTP client for daemon-backed commands
- `internal/daemon/server.go`: daemon loop and API
- `internal/service/service.go`: account logic, auto-switch, refresh, and OpenCode sync wiring
- `internal/opencode/sync.go`: OpenCode auth sync implementation
- `install.sh`: Ubuntu install flow
- `docs/systemd/codex-lover.service`: systemd user service sample

## Status

This branch is already usable as a daily Ubuntu tool for multi-account Codex management.

The intended mental model is simple:

- keep the daemon running
- use `watch` to observe
- use `account add`, `account switch`, and `account remove` to manage accounts
- let auto-switch and OpenCode sync handle the repetitive runtime work
