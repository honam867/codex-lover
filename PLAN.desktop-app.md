# codex-lover Desktop App Plan

## 1. Objective

Add a real Windows desktop UI to `codex-lover` so the user can run:

```powershell
codex-lover run
```

and get a proper app window instead of relying only on terminal rendering.

This desktop app should reuse the current Go backend and replace the current watch-heavy terminal workflow for the main use cases:

- viewing all accounts
- seeing `ACTIVE` vs `LOGGED OUT`
- seeing 5H and weekly quota cards
- adding accounts
- logging into a chosen cached account
- logging out and deleting account data
- watching auto-switch and OpenCode sync state
- receiving clear, stable visual status without terminal flicker

## 2. Why This Direction

The current terminal UI is already functional, but it has structural limits:

- layout depends on terminal width and height
- redraw behavior can feel unstable
- scrolling is awkward
- rich interactions are harder to maintain
- adding deeper account management will make the TUI increasingly fragile

A desktop app solves the actual UX problem more directly.

The backend is already strong enough to support this:

- `internal/service` already owns most stateful behavior
- `internal/store` already persists config and state
- `internal/codex` already handles auth and usage
- `internal/opencode` already handles sync behavior

So the right move is not a rewrite. The right move is a new presentation layer.

## 3. Product Goal

Build a Windows-first desktop app that:

- launches from `codex-lover run`
- shows all tracked accounts in a clean card layout
- refreshes usage automatically
- supports account add / activate / logout flows
- shows auto-switch, notification, and OpenCode sync status
- stays aligned with the same rules currently used by CLI `watch`

Important product rule:

- CLI stays available
- `status`, `watch`, `profile`, `account add`, and daemon commands remain usable
- desktop mode becomes the preferred UI, not the only UI

## 4. Recommended Stack

Use `Wails`.

Reasons:

- Go backend can stay intact
- desktop window opens as a real native app shell
- frontend can use HTML/CSS/JS for a much better UI
- Windows packaging is straightforward enough for this project scale
- the project can keep shipping as one app-oriented distribution instead of becoming a web server product

Frontend choice:

- plain TypeScript + lightweight state store is enough
- React is acceptable if you want faster UI iteration

Recommended practical direction:

- `Wails` for the desktop shell
- `React + TypeScript` for the frontend

Reason for React here:

- the app already has stateful cards, lists, notifications, filters, and actions
- account cards and live updates are easier to keep coherent in React than in hand-written DOM code

## 5. Scope For V1 Desktop

### In scope

- `codex-lover run`
- main dashboard window
- account cards with progress bars
- status tags: `ACTIVE`, `LOGGED OUT`, `FRESH`, `CACHED`, `ERROR`
- live refresh
- OpenCode sync status line
- auto-switch status line
- notifications list inside the app
- add account via Codex login
- activate cached account
- logout and delete account data
- manual refresh

### Out of scope for the first desktop version

- system tray
- background startup with Windows
- multi-window UI
- embedded browser-based auth flow owned by `codex-lover`
- hot-switching a running Codex or OpenCode process
- complex analytics/history charts
- macOS/Linux packaging polish

## 6. User Experience

### Primary flow

1. user runs `codex-lover run`
2. a desktop window opens
3. dashboard loads current profile statuses
4. cards show current account states
5. app refreshes periodically without flicker
6. user can add, activate, or remove accounts from the UI

### Main screen sections

#### Header

- app name
- last refresh time
- refresh status
- current active Codex account
- current OpenCode sync status

#### Dashboard body

- responsive grid of account cards
- each card shows:
  - label
  - email
  - plan
  - auth state
  - freshness state
  - 5H progress bar
  - weekly progress bar
  - credits
  - reset times

#### Right-side or lower activity panel

- last auto-switch event
- last notification event
- last sync event
- background refresh status for logged-out accounts

#### Account actions

Per-card actions:

- `Log in to this account`
- `Log out and delete`
- `Refresh now`

Global actions:

- `Add account`
- `Refresh all`
- `Open data folder`

## 7. Command Model

Keep the current CLI and add:

```powershell
codex-lover run
```

Expected behavior:

- if desktop assets exist, open the desktop app window
- if the desktop runtime is unavailable during development, return a clear error

Potential future variants:

- `codex-lover run --hidden`
- `codex-lover run --minimized`
- `codex-lover run --debug`

But these are not required for the first implementation.

## 8. Architecture

## 8.1 Keep The Current Backend

Keep these packages as the main source of truth:

- `internal/service`
- `internal/store`
- `internal/codex`
- `internal/opencode`
- `internal/model`
- `internal/notify`

The desktop app should call these services, not reimplement them in JS.

## 8.2 Add A Desktop Layer

Add a new package, for example:

- `internal/desktop`

Responsibilities:

- Wails app bootstrapping
- frontend binding methods
- periodic refresh loop for the desktop UI
- app-level event broadcasting to the frontend

## 8.3 Frontend Layer

Add a frontend directory, for example:

- `frontend/`

Responsibilities:

- render dashboard
- manage client-side view state
- call bound Go methods
- subscribe to refresh or event updates
- render responsive cards cleanly

## 9. Backend API Surface For The Desktop App

The desktop app should expose a focused Go API to the frontend.

Recommended first binding surface:

- `GetInitialState()`
- `RefreshAll()`
- `GetProfileStatuses()`
- `AddCodexAccount()`
- `ActivateProfile(profileID string)`
- `LogoutProfile(profileID string)`
- `GetAppStatus()`
- `StartDesktopWatch()`
- `StopDesktopWatch()`

### Suggested return shapes

The frontend should not receive raw internal-only objects unless they are already stable enough.

Prefer a dedicated DTO shape with:

- profile id
- label
- email
- home path
- plan
- auth status
- freshness status
- 5H summary
- weekly summary
- credits
- last error
- flags for active dot / active account / cached auth availability

## 10. Event Model

The desktop app should not poll the Go backend from JS every second.

Better model:

1. frontend calls `StartDesktopWatch()`
2. Go starts or joins one refresh loop
3. Go emits snapshot updates to frontend after each meaningful refresh
4. frontend re-renders from the latest snapshot

This keeps logic centralized in Go.

### Recommended refresh behavior

- active account refresh: every 15 seconds
- logged-out cached background usage refresh: every 15 minutes
- manual refresh button triggers immediate refresh

### Recommended emitted events

- `snapshot.updated`
- `notification.event`
- `switch.event`
- `opencode.sync.event`
- `background.refresh.event`
- `error.event`

## 11. Reuse Rules From Current CLI Watch

The desktop app must reuse current behavior rather than fork it.

These rules should stay identical between CLI watch and desktop mode:

- active account detection
- logged-out account persistence
- reset inference
- OpenCode sync decision logic
- auto-switch decision logic
- notification thresholds
- cached auth activation behavior

If desktop mode and CLI watch start drifting, maintenance cost will go up immediately.

So the implementation rule is:

- move shared watch logic into reusable service methods
- make both CLI watch and desktop watch consume the same behavior

## 12. UI Design Rules

The app should feel like a dashboard, not a form-heavy utility.

### Visual direction

- bold card layout
- strong color-coded status tags
- filled progress bars
- stable spacing
- readable at both small and large window sizes

### Responsive rules

- wide window: 2 to 4 card columns
- medium window: 2 columns
- narrow window: 1 column
- no horizontal overflow
- main body scrolls naturally

### Status semantics

- red dot only for the single active account
- exactly one `ACTIVE` badge at a time for Codex runtime state
- `LOGGED OUT` accounts remain visible
- `CACHED` explains last known usable data
- progress bars always show text values beside visual fill

## 13. Account Management UX

The desktop app should replace the current `Manage accounts` menu with a direct UI.

### Add account

Flow:

1. click `Add account`
2. app starts managed `codex login`
3. user completes login
4. app imports the new account
5. dashboard refreshes

### Log in to a cached account

Flow:

1. user clicks `Log in to this account`
2. app restores cached auth into the runtime Codex home
3. refresh runs
4. selected account becomes `ACTIVE`
5. previous active account becomes `LOGGED OUT`

### Logout and delete

Flow:

1. user clicks `Log out and delete`
2. app asks for confirmation
3. app removes cached auth, profile data, and managed-home remnants
4. dashboard refreshes

## 14. Notification UX

Desktop mode should show notifications in two places:

### In-app event list

- account reached 20%
- account reached 10%
- auto-switch occurred

### OS notification

Reuse existing notification logic where possible so desktop mode does not introduce a second notification policy.

The app should not spam repeated threshold notifications inside the same reset cycle.

## 15. File Layout Proposal

Suggested additions:

```text
cmd/
  codex-lover/
internal/
  desktop/
    app.go
    bindings.go
    events.go
frontend/
  package.json
  src/
    main.tsx
    App.tsx
    components/
      AccountCard.tsx
      StatusBar.tsx
      EventFeed.tsx
      ProgressBar.tsx
    hooks/
      useSnapshot.ts
      useDesktopEvents.ts
    styles/
      app.css
build/
  windows/
```

This exact structure can change, but the separation should stay:

- Go backend bindings
- frontend app
- build output

## 16. Implementation Phases

## Phase 1: Desktop Shell

Goal:

- open a real window from `codex-lover run`

Tasks:

- add Wails project scaffolding
- add frontend build pipeline
- create empty app shell
- wire `codex-lover run`

Definition of done:

- one command opens the window
- frontend can call a simple Go method

Estimated effort:

- 0.5 to 1 day

## Phase 2: Read-Only Dashboard

Goal:

- show current account cards in the window

Tasks:

- expose `GetProfileStatuses` binding
- build account card components
- render tags, progress bars, and reset text
- add manual refresh button

Definition of done:

- desktop app shows the same account list as CLI status

Estimated effort:

- 0.5 to 1 day

## Phase 3: Live Refresh Loop

Goal:

- make the desktop app behave like watch without terminal redraw issues

Tasks:

- add Go-side watch loop
- emit snapshot events
- update frontend reactively
- display sync/switch/notification status lines

Definition of done:

- dashboard updates automatically
- active account changes are reflected without reopening the window

Estimated effort:

- 0.5 to 1 day

## Phase 4: Account Actions

Goal:

- manage accounts directly from the desktop app

Tasks:

- `Add account`
- `Log in to this account`
- `Log out and delete`
- confirmation dialog for destructive action

Definition of done:

- user no longer needs the terminal menu for account management

Estimated effort:

- 0.5 to 1 day

## Phase 5: Polish

Goal:

- make the app feel production-usable

Tasks:

- loading states
- empty states
- action disabling while busy
- better error messages
- desktop icons and window metadata
- stable notification/event panel

Definition of done:

- app feels coherent and safe to use daily

Estimated effort:

- 0.5 to 1.5 days

## 17. Total Effort

Realistic first useful version:

- about 2 to 3 days

Faster technical MVP:

- about 1 day if the goal is only:
  - open window
  - show status
  - refresh

But that would not yet fully replace the current terminal workflow.

## 18. Risks

### Build and packaging risk

- desktop scaffolding adds frontend tooling complexity

Mitigation:

- keep backend logic fully in Go
- keep frontend thin

### State duplication risk

- desktop watch loop and CLI watch loop may diverge

Mitigation:

- move shared watch orchestration into reusable service-level methods

### Login UX risk

- `codex login` is interactive and may open browser/device auth flow outside the app shell

Mitigation:

- first version should launch the existing Codex login flow, not reimplement OAuth

### Windows-only assumptions

- paths and auth locations are currently Windows-first

Mitigation:

- accept Windows-first scope for now
- avoid claiming cross-platform polish in V1 desktop

## 19. Acceptance Criteria

The desktop app plan is successful when the implemented version can do all of the following:

- `codex-lover run` opens a desktop window
- dashboard shows all known Codex accounts
- exactly one account shows `ACTIVE` and the live dot
- logged-out accounts remain visible with cached or inferred usage
- user can add an account without logging out the current one first
- user can activate a cached account from the UI
- user can log out and delete an account from the UI
- OpenCode sync status is visible
- auto-switch status is visible
- threshold and switch notifications are visible in-app

## 20. Final Recommendation

Start with a desktop app, not a new TUI rewrite.

The current codebase already has enough backend behavior to support a good app window. The main work is:

- expose the right bindings
- centralize the watch loop
- build the dashboard and account action UI

That path is much more likely to produce a stable daily tool than continuing to push the terminal UI further.
