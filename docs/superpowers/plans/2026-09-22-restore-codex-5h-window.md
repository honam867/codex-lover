# Plan: Restore Codex 5h quota window (alongside weekly)

## Context

Commits `9b4a115` (backend) and `878f418` (frontend), plus cleanups `5d3dfbe` and `f2c870a`, removed the Codex 5-hour quota window from logic and UI because at that time the Codex usage API returned only one window (`primary_window` = 7 days, `secondary_window: null`). Codex now returns both windows again: `primary_window` = 5h, `secondary_window` = weekly.

The backend still parses both windows (`internal/codex/usage.go:252-253`, `model.UsageSnapshot{Primary, Secondary}`), and `desktop-app/app.go` already exposes `PrimaryPercent/PrimarySummary/SecondaryPercent/SecondarySummary` on `ProfileCard`. Only the labels and decision logic assume `Primary == weekly`. This plan restores the pre-`9b4a115` semantics: **Primary = 5H, Secondary = WEEKLY**, for Codex the same as every other provider.

## Global Constraints

- Semantics everywhere: `Primary` window = **5H**, `Secondary` window = **WEEKLY**. No provider-specific branch that relabels Primary as weekly.
- Keep null-safe fallbacks: if only one window exists, helpers fall back to the other window (as pre-`9b4a115` code did) so the app still works if the API returns a single window again.
- Limit threshold stays `RemainingPercent <= 0.5`.
- Do NOT touch the block-account feature (`status.Profile.Blocked` filters in `AutoRotateCodex` / `bestSwitchCandidate`), price/edit-meta, health/audience, export features.
- Go: `go build ./... && go test ./...` must pass. Frontend: `cd desktop-app/frontend && npx tsc --noEmit` must pass.
- Match existing code style; no new abstractions.
- Commit messages: conventional commits, one commit per task (or a small number of focused commits).

## Task 1: Restore 5h window in service decision logic (Go)

Files: `internal/service/service.go`, `internal/service/trigger_select.go`, `internal/service/weekly_quota_test.go`, `internal/service/trigger_select_test.go`, `internal/codex/trigger.go` (comment only).

Reference for the exact prior code: `git show 9b4a115 -- internal/service/service.go internal/service/trigger_select.go` (the `-` side is the target) and `git show 9b4a115^:internal/service/service_usage_test.go` for the two old tests.

Requirements:
1. Replace `weeklyWindow()` with two helpers:
   - `fiveHourRemaining(status model.ProfileStatus) float64` → `Usage.Primary.RemainingPercent`, fallback `Secondary`, else `0`.
   - `weeklyRemaining(status model.ProfileStatus) float64` → `Usage.Secondary.RemainingPercent`, fallback `Primary`, else `0`.
   Remove the stale comment "Codex dropped the 5h window…".
2. `AutoRotateCodex`: keep the existing `Blocked` filtering untouched. Sort candidates and compute `diff` using `fiveHourRemaining` (pre-`9b4a115` behaviour). Reason string: `"auto-rotate: top account has %.2f%% more 5h remaining"`.
3. `usageLimitReached(status)` → `true` if **either** Primary or Secondary window has `RemainingPercent <= 0.5`. Reintroduce `windowLimitReached(window *model.UsageWindow) bool` (`window != nil && window.RemainingPercent <= 0.5`).
4. `quotaScore(status, now)` → apply `EffectiveWindowForDisplay` to both windows; return `(0, false)` if both nil or if any present window has `RemainingPercent <= 0.5`; otherwise return `min(remaining of present windows)`, `true`. Reintroduce `minFloat`.
5. `selectTriggerTargets` in `trigger_select.go`: after the `weeklyRemaining` comparison, add tie-break on `fiveHourRemaining` (descending) before the label comparison (as in the pre-`9b4a115` code).
6. `internal/codex/trigger.go:59` comment: "weekly quota window" → "5h quota window". `internal/service/trigger_select_test.go:128` comment: fix stale wording about weekly-only.
7. Tests (TDD — write/adjust first, watch fail, then implement):
   - Rewrite `internal/service/weekly_quota_test.go` so it asserts the new semantics (weekly = Secondary with Primary fallback; 5h = Primary with Secondary fallback). Rename the file to `quota_window_test.go` if the name becomes misleading.
   - Restore the two tests from `git show 9b4a115^:internal/service/service_usage_test.go` (limit reached when either window is exhausted; quotaScore uses min of both windows) — adapt to current helper names/signatures.
   - Add a test that `selectTriggerTargets` tie-breaks on 5h when weekly is equal.
8. `go build ./... && go test ./...` green.

## Task 2: Restore 5H / WEEKLY labels in Go presentation layers

Files: `desktop-app/notifications.go`, `desktop-app/tray_windows.go`, `internal/app/app.go`.

Reference: `git show f2c870a -- desktop-app/notifications.go desktop-app/tray_windows.go internal/app/app.go` (the `-` side is the target).

Requirements:
1. `desktop-app/notifications.go` (~line 43-48): remove the Codex-specific branch that labels Primary as `"WEEKLY"`. Primary label = `"5H"`, Secondary label = `"WEEKLY"` for every tool.
2. `desktop-app/tray_windows.go` (~line 183-194): remove the Codex-only "Weekly <reset>" branch; use the generic `"5H <primary> | W <secondary>"` tooltip and `"5H - | W -"` default for all tools.
3. `internal/app/app.go` (~line 592-597, CLI status output): remove the `if item.Profile.Tool == model.ToolCodex` branch; print `5h:` from Primary and `weekly:` from Secondary like other tools.
4. If any of these have unit tests, update them; otherwise add no new tests (presentation only). `go build ./... && go test ./...` green.

## Task 3: Restore two meters (5H + WEEKLY) on Codex cards in the desktop UI

Files: `desktop-app/frontend/src/App.tsx`.

Reference: `git show 878f418 -- desktop-app/frontend/src/App.tsx` (the `-` side is the target), but the file has changed a lot since — do this by hand, do not revert.

Requirements:
1. Card quota block (~line 806-846): remove the `provider === codex` ternary that renders a single "Quota: WEEKLY" meter from `primaryPercent/primarySummary`. Use the existing non-Codex branch for all providers: meter "5H" from `primaryPercent`/`primarySummary` and meter "WEEKLY" from `secondaryPercent`/`secondarySummary`, keeping the existing `renderQuotaSummary()` wrapping.
2. Hide the WEEKLY meter (do not render an empty block) when `secondarySummary` is empty/undefined, so the card still looks right if the API returns only one window.
3. Auto-trigger texts: `"Open weekly quota window"` → `"Open 5H quota window"` (~line 1083). Trigger picker preview rows (~line 1146, 1165): `WK {p.primaryPercent}%` → `5H {p.primaryPercent}% · WK {p.secondaryPercent}%`. Leave `"best weekly quota"` (~line 1126) as is — top_n selection still sorts by weekly first.
4. `cd desktop-app/frontend && npx tsc --noEmit` passes. Do not run `wails build`.

## Task 4: Docs

Files: `README.md`, `PLAN.md`, `PLAN.desktop-app.md`.

Reference: `git show f2c870a -- README.md PLAN.md PLAN.desktop-app.md`.

Requirements:
1. Restore mentions of the 5H window where `f2c870a` removed them (e.g. README "What You Get": "5H and weekly quota bars per Codex account" instead of "A weekly quota bar"). Keep wording consistent with the surrounding text; do not add new sections.
2. No code changes in this task.
