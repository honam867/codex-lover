# codex-lover Plan

## 1. Problem

The user works with multiple accounts across `codex` and later `opencode`.

Pain points:

- not knowing which account is currently active
- not knowing which account is near limit
- not knowing which account will recover first
- manual switching is slow and repetitive
- different tools may end up pointing at different accounts with no shared visibility
- logging out of one account and logging into another in the same home can hide the previous account from view

The project should solve this without forcing a new daily workflow. The user should still be able to type `codex` and `opencode` as usual.

## 2. Product Goal

Build a local manager that:

- runs as a lightweight background service
- installs shims for `codex` and `opencode`
- keeps usage/account state for multiple profiles
- exposes a CLI and TUI watch mode
- auto-selects the best profile at launch time

The core rule is:

- users keep typing `codex` / `opencode`
- the command actually passes through `codex-lover`
- `codex-lover` decides which profile to use

## 3. Scope

### In scope

- local daemon
- CLI commands
- terminal watch UI
- profile management
- usage polling
- launch through shims
- session tracking for processes started through the manager
- auto-select profile on launch
- realtime watch visibility for both currently logged-in and recently logged-out accounts

### Out of scope for V1

- web app
- tray app
- hot-switching account inside an already running process
- perfect detection of external processes launched outside the manager
- deep automation for every possible tool from day one

## 4. Recommended Stack

Use Go.

Reasons:

- single binary distribution
- low memory and CPU usage for a daemon
- easy cross-platform packaging
- strong fit for CLI + local service + process management
- good TUI ecosystem

Recommended libraries:

- CLI: `cobra`
- TUI: `bubbletea`
- styling: `lipgloss`
- local DB: `sqlite`
- config: `yaml` or `toml`

## 5. Core Architecture

There will be one binary named `codex-lover` that can operate in three modes:

1. daemon
2. CLI client
3. shim entrypoint

### Components

#### A. Daemon

Responsibilities:

- load profiles
- poll usage data
- store snapshots
- choose profile at launch time
- register running sessions
- expose local control API on `127.0.0.1`

#### B. CLI client

Responsibilities:

- profile management
- status inspection
- manual switching and pinning
- start watch mode
- request launch actions from the daemon

#### C. TUI watch mode

Responsibilities:

- display profiles and usage
- show active tool/profile pairings
- show progress bars and reset times
- refresh automatically

#### D. Shims

Responsibilities:

- intercept `codex` and `opencode`
- ask daemon which profile to use
- set env/config/home for the target tool
- start the real binary
- register the launched process

## 6. Daily Flow

### Install flow

1. user installs `codex-lover`
2. user runs `codex-lover install`
3. tool locates real `codex` and `opencode` binaries
4. tool installs shims into a directory placed before the real binaries in `PATH`
5. daemon config and local database are created

### Runtime flow

1. user opens a terminal
2. user types `codex`
3. shim runs instead of the real binary
4. shim asks daemon for the chosen profile
5. daemon picks the best available profile
6. shim launches the real `codex` with that profile's environment/home
7. daemon records PID, tool, profile, and start time
8. `codex-lover watch` shows that profile as active

The same model applies to `opencode` once its adapter exists.

### Watch flow

1. user runs `codex-lover watch`
2. TUI connects to daemon
3. daemon streams or serves fresh snapshots
4. watch shows all known accounts, including the currently logged-in account and previously seen accounts from the same home
5. screen refreshes every few seconds

## 7. Profile Model

Each account should be represented as one profile.

Fields:

- `id`
- `label`
- `tool_type`
- `home_path`
- `account_email`
- `account_id`
- `enabled`
- `priority`
- `notes`
- `auth_presence`
- `last_seen_logged_in_at`
- `last_seen_logged_out_at`
- `last_usage_snapshot_at`
- `last_known_primary_reset_at`
- `last_known_secondary_reset_at`

Important design decision:

- isolated homes are still the preferred runtime model for deterministic launching
- however, watch mode must also preserve account history when one shared `CODEX_HOME` logs out account A and later logs into account B
- the system should keep separate account records by stable account identity, even if they were observed through the same `home_path`

This keeps runtime switching deterministic while preserving realtime visibility across account changes.

## 8. Tool Adapters

### Codex adapter

Responsibilities:

- discover or import Codex profiles
- read account metadata from the relevant auth storage
- fetch usage from the same internal backend path Codex uses
- construct runtime env for launching Codex under a specific profile

### OpenCode adapter

Responsibilities:

- define equivalent profile import/discovery for opencode
- read auth/account metadata from opencode storage
- fetch usage if supported through the same or equivalent auth/session path
- construct runtime env for launching opencode under a specific profile

Important:

- adapters must be isolated
- each tool can have different auth storage and runtime semantics

## 9. Usage Polling Logic

Polling should be adaptive.

### Suggested intervals

- active profile: every 10 to 15 seconds
- idle profile: every 60 to 120 seconds
- stale/error profile: exponential backoff up to 5 minutes

### Snapshot state

For each profile store:

- captured time
- plan type
- primary limit percent used
- primary reset time
- secondary limit percent used
- secondary reset time
- credits state
- freshness state
- auth presence state
- last successful usage snapshot even after logout
- derived human-readable reset label for UI and future notifications

### Freshness labels

- `fresh`
- `stale`
- `error`
- `unknown`

### Auth presence labels

- `active`
- `logged_out`
- `missing_auth`
- `error`

### Derived usage helpers

The system should not rely only on preformatted strings from the transport layer.

It should provide helper functions that derive UI and notification text from structured fields such as:

- remaining percent
- used percent
- reset timestamp
- capture timestamp
- freshness state

Example output:

- `19% left  resets 2026-04-10 18:46`

This helper should be shared by:

- watch cards
- future reset notifications
- account selection heuristics
- CLI status output

## 10. Launch Selection Logic

Profile selection happens at launch time only.

Order:

1. if user explicitly requested a profile, use it
2. if tool has a pinned profile and it is healthy, use it
3. if current default profile is still available, use it
4. otherwise choose the enabled profile with the best remaining capacity
5. if all are limited, choose the one with the earliest reset

Auto-switch should mean:

- switch for the next launch

Auto-switch should not mean:

- mutate a live session that is already running

## 11. Session Tracking Logic

The manager only guarantees accurate session ownership for processes launched through its own shims.

For those processes it should track:

- tool type
- profile id
- pid
- start time
- exit time
- exit status

If a tool is launched outside the shim:

- the manager may detect it as external if possible
- but it must not claim exact profile ownership unless it can prove it

## 12. TUI Design

Primary screen sections:

### Header

- daemon status
- last refresh time
- number of active sessions

### Account table

Columns:

- profile
- tool
- account
- auth tag
- state
- 5h
- weekly
- credits
- reset

### Footer

Shortcuts:

- `r` refresh now
- `p` pin profile
- `e` enable/disable profile
- `c` launch codex
- `o` launch opencode
- `q` quit

Visual rules:

- active profile highlighted
- account currently logged in gets an `ACTIVE` tag
- account no longer present in current auth gets a `LOGGED OUT` tag
- limited profiles in warning color
- stale data visually marked
- progress bars compact and stable
- logged-out accounts still show last known usage and reset times when available
- watch should favor card-style layout when it improves readability for multiple accounts

## 13. CLI Commands

Initial command set:

- `codex-lover daemon`
- `codex-lover watch`
- `codex-lover status`
- `codex-lover install`
- `codex-lover install-shims`
- `codex-lover profile list`
- `codex-lover profile import codex`
- `codex-lover profile add`
- `codex-lover profile enable`
- `codex-lover profile disable`
- `codex-lover profile pin`
- `codex-lover run codex`
- `codex-lover run opencode`
- `codex-lover shim codex`
- `codex-lover shim opencode`

Even after shims are installed, the direct `run` commands remain useful for testing.

## 14. Local Data Storage

Use SQLite.

Suggested tables:

- `profiles`
- `usage_snapshots`
- `sessions`
- `settings`
- `tool_binaries`

Why SQLite:

- durable local state
- simple queries for dashboard and selection logic
- easy packaging

## 15. Distribution Model

Target distribution:

- prebuilt binaries per platform
- no runtime dependency on Node or Python
- user installs one executable
- first-run command sets up daemon + shims

Goal:

- clone or download
- install
- import profiles
- start using immediately

## 16. Risks

### Technical risks

- internal usage endpoints may change
- auth storage layout may differ between versions
- opencode may require a separate adapter path
- Windows path and shim behavior need careful handling

### Product risks

- users may expect the manager to understand sessions launched outside its control
- auto-switching across multiple accounts may cross policy or compliance boundaries depending on usage context

## 17. V1 / V2 / V3 Roadmap

### V1

- daemon
- SQLite state
- Codex adapter only
- profile import for Codex
- usage polling for Codex
- `watch` TUI
- shim for `codex`
- launch-time auto-selection
- session tracking for shim-launched Codex

### V1.1

- detect account identity changes inside the same `CODEX_HOME`
- preserve previously seen accounts in watch even after logout
- mark accounts as `ACTIVE` or `LOGGED OUT` in realtime
- keep last known usage snapshot for logged-out accounts
- add shared helper for formatted remaining/reset text used by watch and notifications

### V2

- OpenCode adapter
- shim for `opencode`
- pinning and per-tool default profile rules
- notifications for limit reached / reset soon
- manual switch helpers like `next-ready`

### V3

- richer discovery/import
- optional local web dashboard
- tray integration if needed
- better external session heuristics

## 18. Acceptance Criteria For V1

V1 is successful if:

- user can install the manager and shims
- user can still type `codex` as usual
- manager launches Codex with a selected profile
- watch mode shows all imported Codex profiles
- watch mode shows active profile and usage bars
- watch mode keeps previously seen accounts visible after logout when identity is known
- watch mode clearly marks which account is currently logged in
- watch mode can still show last known usage for logged-out accounts
- manager can pick a different profile when the current one is limited

## 19. Final Design Summary

The design that best matches the intended user experience is:

- one lightweight local daemon
- one CLI binary
- shims for `codex` and later `opencode`
- one watch TUI for continuous visibility
- one profile per account/home
- launch-time profile selection
- usage polling in the background

This keeps the normal terminal workflow intact while adding visibility and account orchestration on top.
