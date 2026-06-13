# Interactive Watch — Design Spec

- **Date:** 2026-06-13
- **Branch:** ubuntu
- **Status:** Design approved; pending implementation plan
- **Topic:** Make `codex-lover watch` interactive (switch / remove / add accounts in-place)

## 1. Goal

Turn the read-only `watch` dashboard into an interactive terminal UI so the user
can **switch, remove, and add** Codex accounts directly while watching live
usage — without leaving the dashboard or dropping to a separate command.

## 2. Current state

- `internal/app/watch.go` → `runWatch` is a simple redraw loop: every 15s (or on
  `ctx` cancel) it calls `statusesForDisplay`, clears the screen
  (`\033[H\033[2J`), and prints account cards. **No keyboard input.** `Ctrl+C`
  exits via the default `SIGINT`.
- Bare `codex-lover` (no args) now also runs `runWatch` (changed in this session).
- Existing CLI actions already follow a **daemon-first, service-fallback** pattern
  via `internal/app/remote.go`:
  - `runAccountSwitch` → `ensureDaemonRunning` + `postDaemonSwitch`, fallback
    `svc.ActivateProfile`.
  - `runAccountRemove` → `ensureDaemonRunning` + `postDaemonRemove`, fallback
    `svc.LogoutProfile`.
  - `addCodexAccount(ctx, svc, st)` runs `codex login` (browser-link login, also
    changed this session) inside an isolated managed home, imports, caches, then
    `tryDaemonRefresh`.
- No TUI library is used; rendering is hand-rolled ANSI. `golang.org/x/term` is
  **not available offline** in this environment (not in module cache, network
  fetch fails, Go version mismatch). `/usr/bin/stty` **is** available.

## 3. Requirements

### Functional
- Navigate the account list with `↑/↓` and `j/k`.
- Trigger actions with single keys:
  - `s` or `Enter` → switch to the selected account
  - `a` → add a new account
  - `d` → remove the selected account (requires `y/n` confirmation)
  - `r` → refresh now
  - `q` or `Ctrl+C` → quit
- The selection cursor **persists across the 15s auto-refresh**, tracked by
  profile ID (not by row index), and clamps gracefully when the list changes.
- `add` **pauses** the dashboard, runs the login flow inline in the same terminal
  (so the login link is visible and the browser flow works), then **resumes** the
  dashboard with the new account present.
- `remove` shows an inline `Remove <label>? (y/n)` confirmation; only `y` deletes.
- Actions route **through the daemon when it is running**, otherwise call the
  service directly — reusing the existing helpers in `remote.go`.

### Non-functional / constraints
- **No new Go module dependency.** Raw mode is implemented by shelling out to
  `stty` via `os/exec`.
- If stdin/stdout is **not a TTY** or `stty` fails, **fall back** to the current
  read-only redraw loop so piping / non-interactive use still works.
- The terminal is **always restored** on exit — normal quit, action error, or
  panic — via `defer` (and a `SIGINT` safety net).
- Respect `NO_COLOR` and `TERM=dumb` exactly as today.
- Codex-only (this branch's scope).

## 4. Design

### 4.1 Entry dispatch
`runWatch` becomes a thin dispatcher:
- If `isInteractiveTerminal()` (stdin is a char device and `stty -g` succeeds) →
  `runWatchInteractive(ctx, svc, st)`.
- Else → `runWatchReadOnly(ctx, svc, st)` (the current loop, renamed; behavior
  unchanged).

### 4.2 Raw mode (stty wrapper)
- `enableRawMode() (restore func() , error)`:
  - Capture original settings: `stty -g` (stdin = `os.Stdin`); store the string.
  - Apply: `stty -echo -icanon min 1 time 0` (no echo, byte-at-a-time reads).
  - `restore` runs `stty <original>` (fallback `stty sane`).
- Called once at start of `runWatchInteractive`; `restore` is `defer`-ed.
- Also installed: a `SIGINT`/`SIGTERM` handler that calls `restore` then exits, so
  an unexpected signal cannot leave the terminal in raw mode. (In raw mode,
  `Ctrl+C` arrives as byte `0x03` and is handled as "quit"; the signal handler is
  a safety net.)

### 4.3 Input reader
- A goroutine reads from `os.Stdin` into a small buffer and emits decoded **key
  events** on a channel. It must decode:
  - single bytes: `j k s a d r q`, `\r` (Enter), `0x03` (Ctrl+C), `y n`
  - arrow escape sequences: `\x1b [ A` (up), `\x1b [ B` (down) — and ignore other
    escape sequences.
- The goroutine exits when `ctx` is done / stdin returns EOF.

### 4.4 Interactive loop (`runWatchInteractive`)
State: `statuses []model.ProfileStatus`, `selectedID string`, `pendingRemoveID
string`, `statusMsg string`.

Loop:
1. Fetch `statuses` (`statusesForDisplay`, codex-filtered); reconcile `selectedID`
   (keep if still present, else clamp to nearest valid row).
2. Render: account cards with the **selected row highlighted** (cursor `>` +
   inverse/bold), a transient `statusMsg` line, and a **footer** of keybindings
   (or the remove confirmation prompt when `pendingRemoveID != ""`).
3. `select`:
   - key event → `handleKey(...)` (mutates state / performs action), then loop
   - `ticker.C` (15s) → loop (re-fetch + re-render)
   - `ctx.Done()` → return

### 4.5 Key → action mapping (pure, testable)
A pure function maps a key event + current state to an action enum
(`moveUp, moveDown, switchSel, addAcct, removeSel, refresh, quit, confirmYes,
confirmNo, none`). When `pendingRemoveID != ""`, only `y`/`n` (and quit) are
meaningful.

### 4.6 Action handlers
- **switch:** `ensureDaemonRunning` → `postDaemonSwitch(addr, id)`; on
  `ErrDaemonUnavailable` fall back to `svc.ActivateProfile(id)`. Set `statusMsg`
  to the result/error. Re-render.
- **remove:** set `pendingRemoveID = selectedID`; footer becomes
  `Remove <label>? (y/n)`. Next key: `y` → perform delete
  (`postDaemonRemove` / fallback `svc.LogoutProfile`), clear `pendingRemoveID`,
  set `statusMsg`; any other key → cancel (clear `pendingRemoveID`).
- **add:**
  1. Stop input goroutine; `restore()` terminal; clear screen.
  2. Run `addCodexAccount(ctx, svc, st)` inline (prints login link; user completes
     browser login).
  3. Print a one-line result (added / error); prompt "Press any key to continue".
  4. Re-enable raw mode, restart input goroutine, re-fetch, re-render; select the
     newly added account if known.
- **refresh:** `svc.RefreshAll()` (+ `tryDaemonRefresh`); set `statusMsg`;
  re-render.

### 4.7 Rendering changes
- Selected row: leading `>` marker + inverse video (`\033[7m`) / bold, gated by
  `watchUsesANSI()`.
- Footer (interactive): `[↑/↓ j/k] move  [s] switch  [a] add  [d] remove  [r] refresh  [q] quit`.
- Transient `statusMsg` line above the footer for action results/errors.
- Read-only mode keeps the existing `Ctrl+C to stop.` footer.

### 4.8 Error handling
- `stty` failure at startup → silent fallback to read-only loop.
- Action errors → shown in `statusMsg`; never crash the loop.
- Terminal restore guaranteed by `defer` + signal handler.

## 5. Files touched
- `internal/app/watch.go` — split `runWatch` into dispatcher + `runWatchReadOnly`;
  add selected-row highlight + interactive footer helpers.
- **New** `internal/app/watch_interactive.go` — interactive loop + action handlers.
- **New** `internal/app/watch_term.go` — `stty` raw-mode wrapper + TTY detection.
- **New** `internal/app/watch_keys.go` — pure key decoding + selection logic.
- **New** `internal/app/watch_keys_test.go` — unit tests.
- Reuse (no change): `ensureDaemonRunning`, `postDaemonSwitch`, `postDaemonRemove`,
  `tryDaemonRefresh` (`remote.go`); `addCodexAccount`, `statusesForDisplay`,
  `svc.ActivateProfile`, `svc.LogoutProfile`, `svc.RefreshAll`.

## 6. Testing
- **Unit (no terminal):**
  - key/escape-sequence decoding (incl. arrows, Enter, Ctrl+C, y/n);
  - selection movement + clamping;
  - `selectedID` reconciliation when the list adds/removes/reorders accounts;
  - action mapping in normal vs. pending-confirm state.
- **Manual:** real terminal — navigation, switch, remove-with-confirm, add
  pause/resume, refresh, quit; verify terminal restored after quit and after an
  error; verify non-TTY fallback via `codex-lover | cat`.

## 7. Out of scope (v1 — YAGNI)
Scrolling/pagination, search/filter, editing labels, multi-select, mouse support,
Claude / non-Codex providers.

## 8. Assumptions
- `addCodexAccount(ctx, svc, st)` is callable from the watch package (it is — same
  package, package-level function).
- Reusing `postDaemonRemove`/`postDaemonSwitch` keeps the daemon as the single
  control loop when it is running; when it is not, direct service calls are safe
  (the file-locked store already supports this, and `account add` already does it).
