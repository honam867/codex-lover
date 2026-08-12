# Codex Account Meta (Editable Date + Price) & Weekly-Only Limit — Design Spec

- **Date:** 2026-07-10
- **Status:** Approved design, pending spec review
- **Scope:** codex-lover desktop app. All three changes are **Codex-only** (Claude/Kimi cards and logic unchanged).

Three features:
- **A. Editable account info** — click a Codex card to edit its *added date*.
- **B. Account price (VNĐ)** — enter a purchase price per Codex account, shown on its card; prompted (skippable) after login, editable anytime via the edit modal.
- **C. Weekly-only limit refactor** — Codex dropped the 5-hour window; the API now returns only a weekly window. Remove the 5h concept from Codex display + logic and show a single weekly limit.

A and B share one edit modal, so they are designed together.

---

## Data finding (drives Part C)

Live `GET https://chatgpt.com/backend-api/wham/usage` on an active Codex account now returns:

```json
"rate_limit": {
  "primary_window":  { "used_percent": 0, "limit_window_seconds": 604800, "reset_at": ... },
  "secondary_window": null
}
```

`limit_window_seconds: 604800` = 7 days = **weekly**, carried in `primary_window`; `secondary_window` is `null`; `additional_rate_limits` is `null`. There is **no 5h window anymore**.

Consequence: the current app mislabels this — the card's "Quota: 5H" meter is actually showing weekly data, and the "Quota: WEEKLY" meter (from `secondary_window`) is empty. The switch/rotate *logic* already reads the single window correctly (because `Primary` is the only non-nil window); only the naming and display are wrong.

---

## Part A + B — Editable date & price (Codex-only)

### Data model (`internal/model/types.go`)
Add to `Profile`:
```go
Price int64 `json:"price,omitempty"` // purchase price in VNĐ (integer dong)
```
`CreatedAt` already exists and is the editable "added date".

### Service (`internal/service/`)
```go
func (s *Service) UpdateProfileMeta(profileID string, createdAt time.Time, price int64) (model.Profile, error)
```
Loads the profile by ID, sets `CreatedAt` (only if `createdAt` is non-zero), sets `Price` (>= 0), bumps `UpdatedAt`, `UpsertProfile`, returns the updated profile. Errors if the profile isn't found or `price < 0`.

### Bindings (`desktop-app/app.go`)
- `UpdateProfileMeta(profileID string, createdAtISO string, price int64) ActionResponse` — parses `createdAtISO` (`YYYY-MM-DD`, interpreted at local midnight; empty → keep existing), calls the service, returns a refreshed snapshot.
- `ProfileCard` gains: `Price int64 \`json:"price"\`` and `CreatedAtISO string \`json:"createdAtISO"\`` (= `CreatedAt.Format("2006-01-02")`, for prefilling the date input). `CreatedAtText` (added-date display) already exists.

### UI (`App.tsx`, `style.css`)
- **Price on card (Codex only):** show a line `Giá  <formatted> ₫` when `price > 0`, formatted with `Intl.NumberFormat("vi-VN")` (thousand separators, e.g. `300.000 ₫`). Reuses the existing `.card-meta` area next to the Added/Trigger lines.
- **Edit modal (Codex only):** clicking the Codex card **body** (not the activate/delete buttons — those `stopPropagation`) opens an **Edit** modal with:
  - a **date** input (`<input type="date">`, default = `createdAtISO`),
  - a **price** input (numeric VNĐ, default = `price`),
  - Save → `UpdateProfileMeta(id, date, price)` → refresh; Cancel closes.
- **Login price popup (Codex only, skippable):** login runs in a separate console, so the app cannot hook the exact login moment. Instead: when a snapshot reveals a **Codex account that is new (its ID was not in the previous snapshot) and has `price == 0`**, open the price popup once for it. It has **Save** and **Skip**. Skipping (or closing) records the ID as "prompted" in frontend state so it doesn't nag again. The edit modal remains the reliable way to set/change price anytime.

### Notes
- Price is Codex-only: the edit modal, price line, and login popup appear only for `provider === "codex"`. Claude/Kimi cards are unchanged.
- Price stored as integer VNĐ; no decimals.

---

## Part C — Weekly-only limit (Codex-only)

### Display (`App.tsx`)
- **Codex cards:** render a **single meter labeled "WEEKLY"**, sourced from the account's one real window (the existing `primaryPercent`/`primarySummary`, which now carries the weekly data). Remove the "5H" meter and the empty second meter for Codex.
- **Claude/Kimi cards:** unchanged (keep their existing two meters).
- **Trigger settings text:** replace "5H" wording with "weekly" (the AUTO_TRIGGER section description and the custom-pick row `5H .. · WK ..` → weekly only).

### Service logic (`internal/service/service.go`) — Codex-only paths
The 5h/weekly helpers are only used by `AutoSwitchLimitedCodex` / `AutoRotateCodex` / `bestSwitchCandidate` (all Codex-only). Consolidate to the single weekly window:
- Remove `fiveHourRemaining`.
- Keep one remaining-quota helper (weekly) that reads the account's single window (`Primary`, falling back to `Secondary` for safety).
- Simplify `quotaScore`, `usageLimitReached`, `bestSwitchCandidate` to reason about the one window instead of "min/any of primary+secondary".
- `verifyTriggerOpened` already reads the single window's reset; keep, adjust comments/naming.

### Model
Leave the shared `UsageSnapshot{Primary, Secondary, ...}` as-is (Claude/Kimi still use both). For Codex, `Primary` **is** the weekly window. Do not rename the shared fields; remove only Codex-facing "5h" naming (helpers, UI labels, comments).

### Auto-trigger
Kept and repurposed to weekly (no mechanism change — a trigger request opens whatever window exists, now weekly). The daily schedule still fires, but within a 7-day weekly window only the first successful trigger of the week actually opens/anchors it; later same-week triggers are effectively no-ops. This is acceptable and needs no scheduler redesign.

---

## Testing
- `UpdateProfileMeta`: unit test — updates CreatedAt + Price, preserves other fields, rejects `price < 0` and unknown profile.
- Weekly quota consolidation: unit test the single-window remaining/`usageLimitReached` behavior (limit reached when the one window is exhausted; candidate selection by weekly remaining).
- Price formatting is frontend (`Intl.NumberFormat`), covered by the frontend build + manual check.
- Frontend `tsc && vite build` clean; manual: edit a Codex card's date+price, see it on the card; confirm Codex card shows one WEEKLY meter.

## Files (indicative)
| File | Change |
|---|---|
| `internal/model/types.go` | `Profile.Price` |
| `internal/service/service.go` | `UpdateProfileMeta`; consolidate 5h→weekly helpers |
| `internal/service/*_test.go` | tests for UpdateProfileMeta + weekly quota |
| `desktop-app/app.go` | `UpdateProfileMeta` binding; `ProfileCard.Price`/`CreatedAtISO`; Codex weekly card fields |
| `desktop-app/frontend/src/App.tsx` | edit modal, price line, login price popup, single weekly meter for Codex, trigger text 5H→weekly |
| `desktop-app/frontend/src/style.css` | edit modal / price / popup styles |
