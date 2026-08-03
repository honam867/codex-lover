# Codex Account Health & Expiry Display Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a desktop `Check trạng thái` action that probes each Codex account and marks unusable accounts red, plus show each Codex account's end date and remaining days on its card.

**Architecture:** Reuse the existing Codex `/backend-api/codex/responses` minimal trigger/probe path, but persist the result as a health-check state separate from manual `Blocked`. Keep the action non-destructive: no auto-switch, no auto-block, no account removal. Current Codex auth/usage payloads in this repo do not expose a real subscription expiry date, so card expiry is derived from the local Added date as **one calendar month** using `time.AddDate(0, 1, 0)`, not a fixed `30 * 24h` duration.

**Tech Stack:** Go stdlib, Wails v2 bindings, React + TypeScript, CSS.

---

## Product Decisions To Confirm Before Coding

- `Check trạng thái` applies to **Codex accounts only** in the first implementation.
- A health probe sends the same kind of minimal request as trigger, so it can open/use the Codex weekly window for accounts that have not been used yet.
- The app **does not automatically block** unhealthy accounts; it only marks them visually so the user can manually block/remove.
- `End` date is calculated as `Added + 1 calendar month` because the current Codex auth/usage APIs used by the app do not expose a subscription end date.
- If `Added` is missing, hide `End`.

---

## File Structure

| File | Change |
|---|---|
| `internal/model/types.go` | Add persisted health-check fields to `ProfileState` and status constants. |
| `internal/codex/trigger.go` | Add typed trigger error metadata so health checks can classify HTTP status without parsing strings. |
| `internal/codex/trigger_test.go` | Cover typed trigger errors for unauthorized and rejected models. |
| `internal/service/profile_health.go` *(new)* | Batch Codex health-check service, result classification, state persistence. |
| `internal/service/profile_health_test.go` *(new)* | Unit tests for classification and state stamping. |
| `desktop-app/app.go` | Add `CheckProfileHealth` Wails action; extend `ProfileCard` with health + calendar-month expiry fields. |
| `desktop-app/frontend/src/App.tsx` | Add top-nav button, busy handling, card red state, health badge/reason, `End` row. |
| `desktop-app/frontend/src/style.css` | Add health danger/warning card styles and meta row styling if needed. |
| `README.md` | Document the new desktop button and its non-destructive behavior if implementation ships. |

---

## Health Status Model

Use these persisted state values in `internal/model/types.go`:

```go
const (
	HealthStatusUnknown = "unknown"
	HealthStatusOK      = "ok"
	HealthStatusNoAuth  = "no_auth"
	HealthStatusLimited = "limited"
	HealthStatusFailed  = "failed"
)
```

Add these fields to `ProfileState`:

```go
	HealthStatus       string     `json:"health_status,omitempty"`
	HealthMessage      string     `json:"health_message,omitempty"`
	HealthCheckedAt    *time.Time `json:"health_checked_at,omitempty"`
	HealthCheckedModel string     `json:"health_checked_model,omitempty"`
```

UI semantics:

- `ok`: normal card, show optional `CHECK OK` text.
- `limited`: warning/red card because the account cannot currently send the probe successfully.
- `failed`: red card.
- `no_auth`: red or warning card because visible is not switchable/checkable.
- `unknown`: no extra visual treatment.

---

### Task 1: Add Typed Trigger Errors

**Files:**
- Modify: `internal/codex/trigger.go`
- Modify: `internal/codex/trigger_test.go`

- [ ] **Step 1: Add failing tests for status metadata**

Add tests that assert unauthorized and rejected-model failures expose status codes through `errors.As`.

Run: `go test ./internal/codex -run 'TestTriggerWindow.*Error'`

Expected before implementation: FAIL because `TriggerError` does not exist.

- [ ] **Step 2: Add `TriggerError`**

In `internal/codex/trigger.go`, add:

```go
type TriggerError struct {
	StatusCode int
	Model      string
	Message    string
}

func (e *TriggerError) Error() string {
	if e == nil {
		return "trigger error"
	}
	if e.Model != "" {
		return fmt.Sprintf("%s for model %s", e.Message, e.Model)
	}
	return e.Message
}
```

- [ ] **Step 3: Return typed errors from `doTriggerOnce`**

Change the unauthorized branch to return `&TriggerError{StatusCode: resp.StatusCode, Model: modelName, Message: "trigger unauthorized"}`.

Change the non-2xx branch to set `lastErr = &TriggerError{StatusCode: resp.StatusCode, Model: modelName, Message: fmt.Sprintf("trigger failed with %d", resp.StatusCode)}`.

- [ ] **Step 4: Verify existing behavior is preserved**

Run: `go test ./internal/codex`

Expected: PASS. Existing callers still see normal `error` strings, while health check can inspect status codes.

---

### Task 2: Add Service Health Check State

**Files:**
- Modify: `internal/model/types.go`
- Create: `internal/service/profile_health.go`
- Create: `internal/service/profile_health_test.go`

- [ ] **Step 1: Add model constants and state fields**

Add the constants and `ProfileState` fields from the Health Status Model section.

- [ ] **Step 2: Write classification tests**

Create tests for:

- success → `ok`, message `Probe OK`
- missing cached auth → `no_auth`, message `Cached auth missing`
- `TriggerError{StatusCode: 401}` → `failed`, message `Unauthorized or expired auth`
- `TriggerError{StatusCode: 403}` → `failed`, message `Forbidden or blocked`
- `TriggerError{StatusCode: 429}` → `limited`, message `Quota or rate limited`
- generic network error → `failed`, message `Probe request failed`

Run: `go test ./internal/service -run TestClassifyProfileHealth`

Expected before implementation: FAIL because helper does not exist.

- [ ] **Step 3: Implement health classification**

Create `internal/service/profile_health.go` with a small pure helper:

```go
type profileHealthResult struct {
	ProfileID string
	Status    string
	Message   string
	ModelUsed string
}
```

The helper should inspect `*codex.TriggerError` with `errors.As` and map status codes as described in Step 2.

- [ ] **Step 4: Implement `CheckCodexProfileHealth`**

Add service method:

```go
func (s *Service) CheckCodexProfileHealth(statuses []model.ProfileStatus) ([]profileHealthResult, error)
```

Behavior:

- Load current config profiles from `statusesToProfiles(statuses)`.
- Iterate every profile where `Profile.Tool == model.ToolCodex`.
- Resolve source cached auth with `s.cachedAuthSourceProfileID(profile, profiles)`.
- If no cache, stamp `no_auth`.
- If cached auth exists, call `codex.TriggerFromCachedAuth(s.codexAuthCacheRoot(), sourceProfileID, codex.DefaultTriggerModels)`.
- Classify result.
- Save `HealthStatus`, `HealthMessage`, `HealthCheckedAt`, and `HealthCheckedModel` into `state.Profiles[profile.ID]`.
- Do not change `Blocked`, do not call `ActivateProfile`, do not delete auth.

- [ ] **Step 5: Verify service tests**

Run: `go test ./internal/service -run 'TestClassifyProfileHealth|TestApplyProfileHealth'`

Expected: PASS.

---

### Task 3: Expose Desktop Snapshot Fields And Action

**Files:**
- Modify: `desktop-app/app.go`

- [ ] **Step 1: Extend `ProfileCard`**

Add fields:

```go
	HealthStatus        string `json:"healthStatus"`
	HealthMessage       string `json:"healthMessage"`
	HealthCheckedAtText string `json:"healthCheckedAtText"`
	EndAtText           string `json:"endAtText"`
	DaysRemainingText   string `json:"daysRemainingText"`
```

- [ ] **Step 2: Add calendar-month expiry helpers**

Add near `formatCreatedAt`:

```go
const codexAccountDurationMonths = 1
```

Add helper behavior:

- `codexAccountEndAt(createdAt)` returns `createdAt.Local().AddDate(0, codexAccountDurationMonths, 0)`.
- `formatEndAt(createdAt)` returns the computed calendar-month end date formatted `02/01/2006`.
- `formatDaysRemaining(createdAt, now)` returns `còn N ngày` for future dates.
- If end date has passed, return `đã hết hạn`.
- If `createdAt.IsZero()`, return empty strings.
- Count remaining days from local date boundaries, not raw hours, so the display is stable across time-of-day and daylight-saving differences.

- [ ] **Step 3: Populate `ProfileCard` in `buildSnapshot`**

Set health fields from `status.State`.

Set expiry fields only for Codex profiles with non-zero `CreatedAt`.

- [ ] **Step 4: Add Wails action**

Add method:

```go
func (a *App) CheckProfileHealth() ActionResponse
```

Behavior:

- `ensureReady()`.
- Lock `a.mu` like other mutating actions.
- Refresh statuses with `a.svc.RefreshAllWithOptions(service.RefreshOptions{SkipUsageFetch: true})` if that option exists; otherwise use normal `RefreshAllWithOptions` consistently with existing snapshot actions.
- Call `a.svc.CheckCodexProfileHealth(statuses)`.
- Return fresh `snapshot(false)`.
- Message: `Checked N Codex account(s)`.
- If service returns an error, return fallback snapshot and `Error`.

- [ ] **Step 5: Verify Go build/tests for desktop package**

Run: `go test ./desktop-app ./internal/service ./internal/codex`

Expected: PASS.

---

### Task 4: Add UI Button, Card Warning, And End Row

**Files:**
- Modify: `desktop-app/frontend/src/App.tsx`
- Modify: `desktop-app/frontend/src/style.css`

- [ ] **Step 1: Extend frontend type and imports**

In `ProfileCard`, add:

```ts
healthStatus: string;
healthMessage: string;
healthCheckedAtText: string;
endAtText: string;
daysRemainingText: string;
```

Import generated binding `CheckProfileHealth` from `../wailsjs/go/main/App` after Wails generation.

- [ ] **Step 2: Add handler**

Add handler near `refresh`:

```ts
async function checkHealth() {
  setStatusText("Checking account health...");
  const response = await CheckProfileHealth();
  if (response.snapshot) setSnapshot(response.snapshot);
  setStatusText(response.error || response.message || "Health check complete");
}
```

- [ ] **Step 3: Render top-nav button next to `Sync All`**

Add a button before `New Link`:

```tsx
<button onClick={() => void checkHealth()} className="cyber-btn flex items-center gap-2">
  <ShieldCheck size={14} /> Check trạng thái
</button>
```

- [ ] **Step 4: Add card classes and badges**

In card className, add a health danger class for Codex profiles where `healthStatus` is `failed`, `limited`, or `no_auth`.

Render a badge near `BLOCKED`:

```tsx
{profile.provider.toLowerCase() === "codex" && isUnhealthy(profile.healthStatus) && (
  <span className="health-badge">CHECK FAILED</span>
)}
```

- [ ] **Step 5: Add health reason and End row**

In `.card-meta`, include rows:

- `End`: `{profile.endAtText} ({profile.daysRemainingText})`
- `Health`: `{profile.healthMessage}` and optional checked time

- [ ] **Step 6: Add CSS**

Add styles:

```css
.account-card-health-danger {
  border-color: rgba(255, 69, 99, 0.75);
  box-shadow: 0 0 24px rgba(255, 69, 99, 0.16);
}

.health-badge {
  border: 1px solid rgba(255, 69, 99, 0.7);
  color: #ff4563;
  font-size: 9px;
  padding: 2px 6px;
  border-radius: 999px;
}
```

- [ ] **Step 7: Verify frontend build**

Run from `desktop-app/frontend`: `npm run build`

Expected: PASS.

---

### Task 5: Docs And Full Verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Document the feature**

Add to desktop behavior docs:

- `Check trạng thái` sends a minimal Codex probe for cached Codex accounts.
- Failed accounts are visually marked only; the app does not auto-block or auto-remove them.
- The button is Codex-only in this version.
- `End` is derived from the local Added date.

- [ ] **Step 2: Run full Go tests**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 3: Run frontend build**

Run from `desktop-app/frontend`: `npm run build`

Expected: PASS.

- [ ] **Step 4: Run desktop build if Wails is available**

Run from `desktop-app`: `wails build -clean`

Expected: PASS and output `build/bin/codex-lover-desktop.exe`.

---

## Risks

- HIGH: The real Codex subscription/account expiry is not available in the current auth/usage payloads; `End` is a local estimate based on Added + one calendar month.
- HIGH: One failed probe cannot perfectly distinguish dead account, provider outage, network failure, or quota/rate limit.
- MEDIUM: The health check intentionally sends a minimal Codex request and may open/use the weekly window.
- MEDIUM: Wails generated TypeScript bindings are gitignored and must be regenerated locally before frontend import compiles.
- LOW: Health red state and manual blocked state must remain visually distinct.

---

## Confirmation Gate

Do not implement until the user confirms:

1. Codex-only health check is acceptable for the first version.
2. `End = Added + 1 calendar month` is acceptable as the local expiry estimate.
3. Health probe may reuse the existing trigger/probe request.
