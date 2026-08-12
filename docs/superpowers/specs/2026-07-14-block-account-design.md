# Block Account (Codex-only) — Design Spec

- **Date:** 2026-07-14
- **Status:** Approved design, pending spec review
- **Scope:** codex-lover desktop app — mark a Codex account as **blocked** so it cannot be switched into (manually or by auto-switch/auto-rotate). Codex-only; Claude/Kimi unchanged.

## Goal
Let the user block a Codex account. A blocked account:
1. cannot be activated manually (the activate/re-auth action refuses),
2. is skipped by auto-switch-on-limit and auto-rotate,
3. if blocked while it is the **active** account, the app auto-switches to the best non-blocked account; if there is no usable replacement, the block is **refused**.

**Invariant:** the active Codex account is never in a blocked state (block-active either switches away first, or is refused).

## Non-goals
- Blocking Claude/Kimi (Codex-only; those cards get no block button).
- Any confirm dialog (block acts immediately; a refused block shows an error message).

---

## Data model (`internal/model/types.go`)
Add to `Profile`:
```go
Blocked bool `json:"blocked,omitempty"`
```

## Service (`internal/service/`)

### `SetProfileBlocked` (new — `internal/service/block.go`)
```go
type BlockResult struct {
    Profile  model.Profile
    Blocked  bool
    Switched bool
    To       model.Profile // set when Switched
}

func (s *Service) SetProfileBlocked(profileID string, blocked bool) (BlockResult, error)
```
Behavior:
- Resolve the target profile from `ProfileStatuses()`; error if not found or `Tool != codex`.
- **Unblock** (`blocked == false`): set `Blocked=false`, `UpsertProfile`, return `{Blocked:false}`. Always allowed.
- **Block** (`blocked == true`):
  - If the target is the **active** Codex account (`State.AuthStatus == active`):
    - Find the best non-blocked cached candidate via `bestSwitchCandidate` (which now excludes blocked).
    - If **none** → return an error (`"cannot block the active account: no other usable account to switch to"`). Nothing is persisted.
    - If **found** → set `Blocked=true` + `UpsertProfile`, then `codex.RestoreCachedHomeAuth(codexAuthCacheRoot, candidate.ID, active.HomePath)` to switch. Return `{Blocked:true, Switched:true, To:candidate}`.
  - Else (not active) → set `Blocked=true` + `UpsertProfile`, return `{Blocked:true}`.

### Guards / skips (edit `internal/service/service.go`)
- `ActivateProfile`: after resolving the selected profile, if `selected.Profile.Blocked` → return an error (`"account is blocked; unblock it first"`). (Defense-in-depth; the UI also hides the activate button.)
- `bestSwitchCandidate`: skip a candidate when `status.Profile.Blocked` (add to the existing filter alongside the cached-auth/home checks).
- `AutoRotateCodex`: in the candidate loop, skip when `status.Profile.Blocked`.
- (`AutoSwitchLimitedCodex` calls `bestSwitchCandidate`, so it inherits the blocked skip — no direct change.)
- `mergeCanonicalProfile`: carry the flag — `canonical.Blocked = canonical.Blocked || duplicate.Blocked` (blocked if either side is).

## Binding (`desktop-app/app.go`)
- `SetProfileBlocked(profileID string, blocked bool) ActionResponse`:
  - `ensureReady`; lock `a.mu` around `a.svc.SetProfileBlocked(...)`, unlock before any snapshot call.
  - On error → `Update failed` with the error message (so a refused block surfaces).
  - On success → `a.snapshot(true)` **only if a switch happened** (need a network refresh to reflect the new active account's usage); otherwise `a.snapshot(false)` (fast, block/unblock is local config).
- `ProfileCard`: add `Blocked bool \`json:"blocked"\``, populated in `buildSnapshot` from `status.Profile.Blocked`.

## UI (`desktop-app/frontend/src/App.tsx`, `style.css`)
Codex cards only:
- Add a **Block/Unblock toggle button** in the card footer next to activate/delete. Not blocked → a "block" action (lucide `Ban`, danger tone); blocked → an "unblock" action (lucide `ShieldCheck`/`Check`). Clicking calls `SetProfileBlocked(id, !blocked)` and `applyAction(result)`.
- **Blocked visual:** dim the card (reduced opacity / muted) and show a small `BLOCKED` badge near the auth-status badge.
- **Hide the activate (re-auth) button** when `blocked` (a blocked account can't be switched into).
- Non-Codex cards: no block button, no blocked styling.

## Testing
- `internal/service`: 
  - `bestSwitchCandidate`/candidate selection skips blocked accounts (table-driven via the existing selection paths, or a focused test).
  - `ActivateProfile` refuses a blocked profile.
  - `SetProfileBlocked` block-active with a replacement switches; block-active with no replacement returns an error and persists nothing; block-non-active just sets the flag; unblock clears it. (Where store access makes direct testing hard, extract a pure decision helper and test that, mirroring `applyProfileMeta`/`applyTriggerStamps`.)
  - `mergeCanonicalProfile` preserves `Blocked`.
- Frontend `tsc && vite build` clean; manual: block an account → dimmed + badge + can't activate; auto-switch skips it; block the active one → switches to another; block the active one with no replacement → error toast.

## Files
| File | Change |
|---|---|
| `internal/model/types.go` | `Profile.Blocked` |
| `internal/service/block.go` (new) | `SetProfileBlocked` (+ pure decision helper) |
| `internal/service/service.go` | `ActivateProfile` guard; `bestSwitchCandidate` + `AutoRotateCodex` skip blocked; `mergeCanonicalProfile` carry Blocked |
| `internal/service/*_test.go` | block logic + skip + guard + merge tests |
| `desktop-app/app.go` | `SetProfileBlocked` binding; `ProfileCard.Blocked`; buildSnapshot |
| `desktop-app/frontend/src/App.tsx` | block/unblock button, blocked visual, hide activate when blocked |
| `desktop-app/frontend/src/style.css` | blocked card + badge styles |
