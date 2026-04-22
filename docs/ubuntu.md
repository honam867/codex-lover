# Ubuntu Headless Usage

This branch targets Ubuntu and VPS usage only.

`codex-lover` is now a headless daemon plus CLI client:

- `codex-lover server run` starts the background runtime
- `codex-lover status` shows the current Codex account state
- `codex-lover watch` renders a live terminal dashboard
- `codex-lover account ...` manages accounts from anywhere in the shell
- logged-out Codex accounts keep cached quota/freshness data when cached auth exists

## Requirements

- Ubuntu
- Go installed
- `codex` installed and available in `PATH`
- at least one working Codex login already present in `~/.codex/auth.json`

## Install

If `go` is missing:

```bash
sudo apt update && sudo apt install -y golang-go
```

From the repo root:

```bash
bash ./install.sh
```

That builds and installs `codex-lover` into `~/.local/bin/codex-lover`.

## Commands

```bash
codex-lover server run
codex-lover status
codex-lover status --json
codex-lover refresh
codex-lover watch
codex-lover account list
codex-lover account add
codex-lover account switch <profile-id>
codex-lover account remove <profile-id>
codex-lover profile import codex --label NAME --home PATH
```

## Account Add Flow

`codex-lover account add` now uses device-code login.

It runs:

- `codex login --device-auth`
- with an isolated managed `CODEX_HOME`
- with file-based credential storage forced for the login session

After login finishes, the new `auth.json` is imported and cached under `~/.codex-lover/codex-auth`.

## Background Runtime

The server loop:

- refreshes account state every 15 seconds
- refreshes logged-out cached usage every 15 minutes
- warms logged-out cached usage immediately on startup, manual refresh, and account changes
- auto-switches to another cached Codex account when the active account reaches limit
- syncs `opencode` from the active Codex account
- writes one summary log line per poll cycle

## Realtime Logs

Foreground:

```bash
codex-lover server run
```

Live dashboard:

```bash
codex-lover watch
```

Machine-readable status:

```bash
codex-lover status --json
```

## Systemd User Service

Sample unit file: `docs/systemd/codex-lover.service`

Install it as a user service:

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

## Integration For Other CLIs

Other local tools can either:

- call `codex-lover status --json`
- call the local daemon on `http://127.0.0.1:47070`

Endpoints currently available:

- `GET /v1/status`
- `POST /v1/refresh`
- `GET /v1/accounts`
- `POST /v1/accounts/switch`
- `POST /v1/accounts/remove`
