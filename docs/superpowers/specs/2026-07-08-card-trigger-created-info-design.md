# Codex Card Info Lines + Deletion History — Design Spec

- **Date:** 2026-07-08
- **Status:** Approved design, pending spec review
- **Scope:** codex-lover desktop app — (A) surface two extra info lines on the main-dashboard account cards for Codex/OpenAI accounts, and (B) keep a simple deletion history in Settings so the user can find a removed account later.

---

## 1. Problem & Goal

The auto-trigger feature records trigger results, but the user must open Settings to see them. On the main dashboard, for each **Codex** account, show:

1. **Last trigger** — when this account was last triggered and which model was used, so the user sees it without opening Settings.
2. **Added date** — when the account was added to codex-lover (its login/add date).

Both lines appear only on Codex/OpenAI cards. Claude/Kimi cards are unchanged.

**Non-goals:** the real OpenAI account creation date (not available via the API — we use the codex-lover add date); any change to non-Codex cards; any new trigger behavior.

---

## 2. Decisions (locked)

- **Added date** reuses the existing `Profile.CreatedAt` (already set at login/add, preserved across refresh in `RefreshAllWithOptions`, and merged to the earliest value in `mergeCanonicalProfile`). No new stored field.
- **Last-trigger** is tracked **per account** in `model.ProfileState` (the current `State.LastTriggerRun` only holds the single most-recent run, not per-account history).
- **Trigger line format:** time + date + model, e.g. `Trigger: 08:00 08/07 · gpt-5.4-mini`.
- **Added line format:** `Added: 08/07/2026`.
- Both lines render only when `provider === "codex"`.

---

## 3. Data Model (`internal/model/types.go`)

Add two fields to `ProfileState`:

```go
LastTriggeredAt    *time.Time `json:"last_triggered_at,omitempty"`
LastTriggeredModel string     `json:"last_triggered_model,omitempty"`
```

`Profile.CreatedAt` already exists — no change.

---

## 4. Stamping per-account trigger (`internal/service/trigger.go`)

In `RunTrigger`, after a target account is triggered successfully (`Status == TriggerStatusOpened`), persist that account's per-profile stamp:

- Load state, set `state.Profiles[profileID].LastTriggeredAt = run.RanAt` and `.LastTriggeredModel = result.ModelUsed`, save.
- This must not disturb the existing `LastTriggerRun` persistence or other `ProfileState` fields.
- Applies to BOTH scheduled runs and manual `TriggerNow` (both call `RunTrigger`).

The stamp uses the profile's own ID (`t.Status.Profile.ID`), even when the cached auth is resolved from a different source profile ID — the card the user sees is keyed by `Profile.ID`.

---

## 5. Snapshot fields (`desktop-app/app.go`, `buildSnapshot` + `ProfileCard`)

Add to `ProfileCard`:

```go
CreatedAtText       string `json:"createdAtText"`
LastTriggeredAtText string `json:"lastTriggeredAtText"`
LastTriggeredModel  string `json:"lastTriggeredModel"`
```

Populate in `buildSnapshot`:
- `CreatedAtText`: `status.Profile.CreatedAt.Local().Format("02/01/2006")`, or `""` if `CreatedAt.IsZero()`.
- `LastTriggeredAtText`: `status.State.LastTriggeredAt.Local().Format("15:04 02/01")`, or `""` if nil.
- `LastTriggeredModel`: `status.State.LastTriggeredModel`.

(Populated for all profiles; the frontend gates display by provider.)

---

## 6. UI (`desktop-app/frontend/src/App.tsx`, `style.css`)

On each account card, for `provider === "codex"` only, render:
- If `createdAtText`: a small line `Added: {createdAtText}`.
- If `lastTriggeredAtText`: a small line `Trigger: {lastTriggeredAtText} · {lastTriggeredModel}` (model omitted if empty).

Place these in the card footer/meta area, styled compact and dim (reuse existing `text-dim`/small-text conventions). Extend the `ProfileCard` TS type with the three new fields.

---

## 7. Error Handling & Edges

- Never-triggered Codex account → no trigger line (empty text).
- Non-Codex card → neither new line.
- `CreatedAt` zero (shouldn't happen) → no added line.
- No token/secret exposure (only timestamps + model name).

---

## 8. Testing

- `internal/service`: unit test that `RunTrigger` stamps `LastTriggeredAt` + `LastTriggeredModel` on the triggered profile's `ProfileState` for an opened result, and leaves a skipped/non-selected profile un-stamped. (Uses a temp store; asserts persisted state.)
- Frontend: `tsc && vite build` clean.
- Manual: rebuild app, confirm the two lines show on a Codex card after a trigger.

---

## 9. Files

| File | Change |
|---|---|
| `internal/model/types.go` | +2 `ProfileState` fields |
| `internal/service/trigger.go` | stamp per-profile state in `RunTrigger` |
| `internal/service/trigger_test.go` (or new) | test the stamp |
| `desktop-app/app.go` | +3 `ProfileCard` fields + populate in `buildSnapshot` |
| `desktop-app/frontend/src/App.tsx` | render two lines on Codex cards + extend type |
| `desktop-app/frontend/src/style.css` | small styles if needed |

---

## 10. Deletion History (Part B)

**Goal:** when the user deletes an account, keep a simple record so they can find it again later. Shown as a read-only list in Settings. Applies to **all providers** (not Codex-only) — the user asked to log "any account" deletion.

**Data model (`internal/model/types.go`):**

```go
type DeletedAccountRecord struct {
	ProfileID string    `json:"profile_id"`
	Label     string    `json:"label"`
	Email     string    `json:"email,omitempty"`
	Provider  string    `json:"provider"`
	DeletedAt time.Time `json:"deleted_at"`
}
```
Add to `State`: `DeletionHistory []DeletedAccountRecord \`json:"deletion_history,omitempty"\``.

**Recording (`internal/service/service.go`, `LogoutProfile`):** after the profile is removed (`RemoveProfile` succeeds), append a record built from the deleted profile (`Label` via `profileLabel`, `Email`, `Provider` via provider-or-tool, `DeletedAt = now`). Newest-first, capped at the last **50** entries. `RemoveProfile` deletes `state.Profiles[id]` but not `DeletionHistory`, so the history survives.

Use a pure helper for the list operation so it is testable without a store:
```go
func appendDeletionRecord(history []DeletedAccountRecord, rec DeletedAccountRecord, max int) []DeletedAccountRecord
```
Prepends `rec`, trims to `max`.

**Binding (`desktop-app/app.go`):** `GetDeletionHistory() []model.DeletedAccountRecord` (returns `[]` not nil; `ensureReady`-guarded).

**UI (`App.tsx`):** a new "DELETION LOG" block inside the existing Settings modal, listing each record as `label/email · PROVIDER · <deletedAt>` (`deletedAt` formatted `DD/MM/YYYY HH:MM`). Empty state: "No deletions yet." Load via `GetDeletionHistory` when the settings modal opens (or on mount).

**Testing:** pure `appendDeletionRecord` (prepend + cap-at-max, oldest dropped); frontend build.

**Edges:** deleting the same account twice → two records (fine, simple). No token/secret in records (only label/email/provider/time).
