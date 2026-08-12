# Codex Card Info Lines Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show two info lines on the main-dashboard Codex account cards — last trigger (time + date + model) and added date — without opening Settings; and keep a simple deletion history in Settings so a removed account can be found later.

**Architecture:** Track last-trigger per account in `ProfileState` (stamped in `RunTrigger`), reuse the existing `Profile.CreatedAt` for the added date, expose both through `ProfileCard` in `buildSnapshot`, and render two compact lines on Codex cards only.

**Tech Stack:** Go (stdlib), Wails v2 bindings, React + TypeScript.

## Global Constraints

- Module path is `codex-lover`; imports use `codex-lover/internal/...`.
- Both lines render only for `provider === "codex"` (OpenAI only). Claude/Kimi cards unchanged.
- Added date = `Profile.CreatedAt` (codex-lover add/login date), NOT a new stored field.
- Trigger line format: time + date + model, e.g. `Trigger  08:00 08/07 · gpt-5.4-mini`. Added line: `Added  08/07/2026`.
- Config/state JSON uses snake_case tags (matches existing `model`); `ProfileCard` JSON uses camelCase (matches existing).
- `gofmt`/`goimports` mandatory; Go tests table-driven; no token/secret exposure.

---

## File Structure

| File | Change |
|---|---|
| `internal/model/types.go` | +2 `ProfileState` fields (`LastTriggeredAt`, `LastTriggeredModel`) |
| `internal/service/trigger.go` | extract pure `applyTriggerStamps`; stamp per-profile in `persistLastTriggerRun` |
| `internal/service/trigger_stamp_test.go` | test `applyTriggerStamps` |
| `desktop-app/app.go` | +3 `ProfileCard` fields + populate in `buildSnapshot` + 2 format helpers |
| `desktop-app/frontend/src/App.tsx` | extend `ProfileCard` type + render 2 lines on Codex cards; deletion-log section |
| `desktop-app/frontend/src/style.css` | `.card-meta` / `.card-meta-row` + `.deletion-log*` styles |
| `internal/service/deletion_history.go` | pure `appendDeletionRecord` + `recordDeletion` + `DeletionHistory` (Task 3) |
| `internal/service/deletion_history_test.go` | test `appendDeletionRecord` (Task 3) |

---

### Task 1: Backend — per-account trigger stamp + snapshot fields

**Files:**
- Modify: `internal/model/types.go`
- Modify: `internal/service/trigger.go`
- Create: `internal/service/trigger_stamp_test.go`
- Modify: `desktop-app/app.go`

**Interfaces:**
- Consumes: `model.ProfileState`, `model.TriggerRun`, `model.TriggerAccountResult`, `model.TriggerStatusOpened`, existing `buildSnapshot`/`ProfileCard`.
- Produces: `ProfileState.LastTriggeredAt *time.Time`, `ProfileState.LastTriggeredModel string`; pure `applyTriggerStamps(state model.State, run model.TriggerRun) model.State`; `ProfileCard.CreatedAtText/LastTriggeredAtText/LastTriggeredModel string`.

- [ ] **Step 1: Add the two ProfileState fields**

In `internal/model/types.go`, add to the `ProfileState` struct (after `LastSeenLoggedOutAt`):

```go
	LastTriggeredAt    *time.Time `json:"last_triggered_at,omitempty"`
	LastTriggeredModel string     `json:"last_triggered_model,omitempty"`
```

- [ ] **Step 2: Write the failing test**

Create `internal/service/trigger_stamp_test.go`:

```go
package service

import (
	"testing"
	"time"

	"codex-lover/internal/model"
)

func TestApplyTriggerStamps(t *testing.T) {
	state := model.State{Profiles: map[string]model.ProfileState{
		"acc-a": {ProfileID: "acc-a", AuthStatus: model.AuthStatusLoggedOut},
	}}
	ranAt := time.Date(2026, 7, 8, 8, 0, 0, 0, time.UTC)
	run := model.TriggerRun{
		RanAt: ranAt,
		Results: []model.TriggerAccountResult{
			{ProfileID: "acc-a", Status: model.TriggerStatusOpened, ModelUsed: "gpt-5.4-mini"},
			{ProfileID: "acc-b", Status: model.TriggerStatusError},
			{ProfileID: "acc-c", Status: model.TriggerStatusSkippedNoAuth},
		},
	}

	out := applyTriggerStamps(state, run)

	a := out.Profiles["acc-a"]
	if a.LastTriggeredAt == nil || !a.LastTriggeredAt.Equal(ranAt) {
		t.Fatalf("acc-a should be stamped at ranAt, got %v", a.LastTriggeredAt)
	}
	if a.LastTriggeredModel != "gpt-5.4-mini" {
		t.Fatalf("acc-a model = %q, want gpt-5.4-mini", a.LastTriggeredModel)
	}
	if a.AuthStatus != model.AuthStatusLoggedOut {
		t.Fatalf("existing ProfileState fields must be preserved")
	}
	if out.Profiles["acc-b"].LastTriggeredAt != nil {
		t.Fatalf("error result must not be stamped")
	}
	if out.Profiles["acc-c"].LastTriggeredAt != nil {
		t.Fatalf("skipped result must not be stamped")
	}
	if out.LastTriggerRun == nil {
		t.Fatalf("LastTriggerRun should be set")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestApplyTriggerStamps`
Expected: FAIL — `applyTriggerStamps` undefined.

- [ ] **Step 4: Implement the pure helper + wire it into persistLastTriggerRun**

In `internal/service/trigger.go`, replace the existing `persistLastTriggerRun` with:

```go
// applyTriggerStamps records the run as the last run and stamps each
// successfully-opened account's per-profile last-trigger time and model.
func applyTriggerStamps(state model.State, run model.TriggerRun) model.State {
	if state.Profiles == nil {
		state.Profiles = map[string]model.ProfileState{}
	}
	saved := run
	state.LastTriggerRun = &saved
	for _, res := range run.Results {
		if res.Status != model.TriggerStatusOpened {
			continue
		}
		ps := state.Profiles[res.ProfileID]
		ps.ProfileID = res.ProfileID
		ranAt := run.RanAt
		ps.LastTriggeredAt = &ranAt
		ps.LastTriggeredModel = res.ModelUsed
		state.Profiles[res.ProfileID] = ps
	}
	return state
}

func (s *Service) persistLastTriggerRun(run model.TriggerRun) error {
	state, err := s.store.LoadState()
	if err != nil {
		return err
	}
	state = applyTriggerStamps(state, run)
	return s.store.SaveState(state)
}
```

(The prior `persistLastTriggerRun` only set `state.LastTriggerRun`; the new one also stamps per-profile via `applyTriggerStamps`. Do not change `RunTrigger`/`RunScheduledTrigger`.)

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/service/ -run TestApplyTriggerStamps`
Expected: PASS

- [ ] **Step 6: Add ProfileCard fields + populate in buildSnapshot**

In `desktop-app/app.go`, add to the `ProfileCard` struct (after `LastRefreshedAtText`):

```go
	CreatedAtText       string `json:"createdAtText"`
	LastTriggeredAtText string `json:"lastTriggeredAtText"`
	LastTriggeredModel  string `json:"lastTriggeredModel"`
```

In `buildSnapshot`, inside the `profiles = append(profiles, ProfileCard{...})` literal, add these three fields (after `LastRefreshedAtText: ...`):

```go
			CreatedAtText:       formatCreatedAt(status.Profile.CreatedAt),
			LastTriggeredAtText: formatLastTriggeredAt(status.State.LastTriggeredAt),
			LastTriggeredModel:  status.State.LastTriggeredModel,
```

Add these two helpers to `desktop-app/app.go` (near `formatTimePointer`):

```go
func formatCreatedAt(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Local().Format("02/01/2006")
}

func formatLastTriggeredAt(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Local().Format("15:04 02/01")
}
```

(`time` is already imported in `desktop-app/app.go`.)

- [ ] **Step 7: Build + full service tests**

Run: `go build ./...` then `go test ./internal/service/`
Expected: build OK; all service tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/model/types.go internal/service/trigger.go internal/service/trigger_stamp_test.go desktop-app/app.go
git commit -m "feat: track per-account last trigger and expose created/trigger on card"
```

---

### Task 2: Frontend — render the two lines on Codex cards

**Files:**
- Modify: `desktop-app/frontend/src/App.tsx`
- Modify: `desktop-app/frontend/src/style.css`

**Interfaces:**
- Consumes: `ProfileCard.createdAtText/lastTriggeredAtText/lastTriggeredModel` (from Task 1), the existing `ProfileCard` TS type and card `<article>` JSX.

- [ ] **Step 1: Extend the ProfileCard TS type**

In `desktop-app/frontend/src/App.tsx`, add to the `type ProfileCard = {...}` (after `lastRefreshedAtText: string;`):

```tsx
  createdAtText: string;
  lastTriggeredAtText: string;
  lastTriggeredModel: string;
```

- [ ] **Step 2: Render the two lines on Codex cards**

In `App.tsx`, inside the card `<article>`, immediately AFTER the quota block (the `<div className="space-y-5"> ... </div>` that contains the two `meter-block`s) and BEFORE the footer `<div className="flex justify-between items-center mt-6 pt-4 border-t ...">`, insert:

```tsx
              {profile.provider.toLowerCase() === "codex" &&
                (profile.lastTriggeredAtText || profile.createdAtText) && (
                  <div className="card-meta">
                    {profile.lastTriggeredAtText && (
                      <div className="card-meta-row">
                        <span className="text-dim">Trigger</span>
                        <span>
                          {profile.lastTriggeredAtText}
                          {profile.lastTriggeredModel ? ` · ${profile.lastTriggeredModel}` : ""}
                        </span>
                      </div>
                    )}
                    {profile.createdAtText && (
                      <div className="card-meta-row">
                        <span className="text-dim">Added</span>
                        <span>{profile.createdAtText}</span>
                      </div>
                    )}
                  </div>
                )}
```

- [ ] **Step 3: Add CSS**

Append to `desktop-app/frontend/src/style.css`:

```css
.card-meta {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.card-meta-row {
  display: flex;
  justify-content: space-between;
  font-size: 10px;
  color: #7fe9ff;
}
```

- [ ] **Step 4: Build the frontend**

Run: `cd desktop-app/frontend && npm run build`
Expected: `tsc && vite build` completes with zero TypeScript errors.

- [ ] **Step 5: Commit**

```bash
git add desktop-app/frontend/src/App.tsx desktop-app/frontend/src/style.css
git commit -m "feat: show last-trigger and added-date lines on Codex cards"
```

---

### Task 3: Deletion history (all providers)

**Files:**
- Modify: `internal/model/types.go`
- Create: `internal/service/deletion_history.go`
- Create: `internal/service/deletion_history_test.go`
- Modify: `internal/service/service.go` (`LogoutProfile`)
- Modify: `desktop-app/app.go`
- Modify: `desktop-app/frontend/src/App.tsx`, `desktop-app/frontend/src/style.css`

**Interfaces:**
- Consumes: `model.State`, `model.Profile`, existing `s.store`, `profileLabel`, `chooseNonEmpty`, the Settings modal JSX.
- Produces: `model.DeletedAccountRecord`, `State.DeletionHistory`, pure `appendDeletionRecord(history []model.DeletedAccountRecord, rec model.DeletedAccountRecord, max int) []model.DeletedAccountRecord`, `(s *Service) DeletionHistory() ([]model.DeletedAccountRecord, error)`, `App.GetDeletionHistory() []model.DeletedAccountRecord`.

- [ ] **Step 1: Add the model + State field**

In `internal/model/types.go` add:

```go
type DeletedAccountRecord struct {
	ProfileID string    `json:"profile_id"`
	Label     string    `json:"label"`
	Email     string    `json:"email,omitempty"`
	Provider  string    `json:"provider"`
	DeletedAt time.Time `json:"deleted_at"`
}
```
Add to `State` (after `LastTriggerDate`): `DeletionHistory []DeletedAccountRecord \`json:"deletion_history,omitempty"\``.

- [ ] **Step 2: Write the failing test**

Create `internal/service/deletion_history_test.go`:

```go
package service

import (
	"testing"
	"time"

	"codex-lover/internal/model"
)

func TestAppendDeletionRecordPrependsAndCaps(t *testing.T) {
	var history []model.DeletedAccountRecord
	for i := 0; i < 3; i++ {
		rec := model.DeletedAccountRecord{ProfileID: string(rune('a' + i)), DeletedAt: time.Unix(int64(i), 0)}
		history = appendDeletionRecord(history, rec, 2)
	}
	if len(history) != 2 {
		t.Fatalf("history should be capped at 2, got %d", len(history))
	}
	if history[0].ProfileID != "c" {
		t.Fatalf("newest ('c') must be first, got %q", history[0].ProfileID)
	}
	if history[1].ProfileID != "b" {
		t.Fatalf("second-newest ('b') must be second, got %q", history[1].ProfileID)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestAppendDeletionRecordPrependsAndCaps`
Expected: FAIL — `appendDeletionRecord` undefined.

- [ ] **Step 4: Implement the helpers**

Create `internal/service/deletion_history.go`:

```go
package service

import (
	"time"

	"codex-lover/internal/model"
)

const deletionHistoryMax = 50

// appendDeletionRecord prepends rec (newest first) and trims to max entries.
func appendDeletionRecord(history []model.DeletedAccountRecord, rec model.DeletedAccountRecord, max int) []model.DeletedAccountRecord {
	next := append([]model.DeletedAccountRecord{rec}, history...)
	if max > 0 && len(next) > max {
		next = next[:max]
	}
	return next
}

func (s *Service) recordDeletion(profile model.Profile) error {
	state, err := s.store.LoadState()
	if err != nil {
		return err
	}
	rec := model.DeletedAccountRecord{
		ProfileID: profile.ID,
		Label:     profileLabel(profile),
		Email:     profile.Email,
		Provider:  chooseNonEmpty(profile.Provider, profile.Tool),
		DeletedAt: time.Now().UTC(),
	}
	state.DeletionHistory = appendDeletionRecord(state.DeletionHistory, rec, deletionHistoryMax)
	return s.store.SaveState(state)
}

func (s *Service) DeletionHistory() ([]model.DeletedAccountRecord, error) {
	state, err := s.store.LoadState()
	if err != nil {
		return nil, err
	}
	return state.DeletionHistory, nil
}
```

- [ ] **Step 5: Record on delete in LogoutProfile**

In `internal/service/service.go`, `LogoutProfile`, the function ends with:

```go
	if err := s.store.RemoveProfile(selected.Profile.ID); err != nil {
		return result, err
	}

	return result, nil
```

Replace the final `return result, nil` so a deletion record is written after the profile is removed:

```go
	if err := s.store.RemoveProfile(selected.Profile.ID); err != nil {
		return result, err
	}

	if err := s.recordDeletion(selected.Profile); err != nil {
		return result, err
	}

	return result, nil
```

- [ ] **Step 6: Run test + full package**

Run: `go test ./internal/service/ -run TestAppendDeletionRecordPrependsAndCaps` then `go test ./internal/service/`
Expected: new test PASS; whole service package PASS.

- [ ] **Step 7: Add the binding**

In `desktop-app/app.go` add:

```go
func (a *App) GetDeletionHistory() []model.DeletedAccountRecord {
	if err := a.ensureReady(); err != nil {
		return []model.DeletedAccountRecord{}
	}
	history, err := a.svc.DeletionHistory()
	if err != nil || history == nil {
		return []model.DeletedAccountRecord{}
	}
	return history
}
```

- [ ] **Step 8: Build backend + commit**

Run: `go build ./...` then `go test ./internal/service/`
Expected: build OK; tests PASS.

```bash
git add internal/model/types.go internal/service/deletion_history.go internal/service/deletion_history_test.go internal/service/service.go desktop-app/app.go
git commit -m "feat: record deleted accounts to a deletion history"
```

- [ ] **Step 9: Frontend — type + import + load + render**

In `desktop-app/frontend/src/App.tsx`:

Add `GetDeletionHistory` to the `../wailsjs/go/main/App` import.

Add the type (near the other trigger types):

```tsx
type DeletedAccountRecord = {
  profile_id: string;
  label: string;
  email?: string;
  provider: string;
  deleted_at: string;
};
```

Add state (near `deletionHistory` peers):

```tsx
  const [deletionHistory, setDeletionHistory] = useState<DeletedAccountRecord[]>([]);
```

Add an effect that loads history when the settings modal opens:

```tsx
  useEffect(() => {
    if (!showSettingsModal) return;
    void (async () => {
      try {
        const h = (await GetDeletionHistory()) as unknown as DeletedAccountRecord[];
        setDeletionHistory(h ?? []);
      } catch {
        setDeletionHistory([]);
      }
    })();
  }, [showSettingsModal]);
```

Inside the settings modal's `<div className="space-y-8">`, after the AUTO_TRIGGER block, add:

```tsx
              <div className="bg-[rgba(0,243,255,0.05)] p-4 border border-[rgba(0,243,255,0.1)]">
                <div className="font-bold text-sm mb-3">DELETION_LOG</div>
                {deletionHistory.length === 0 ? (
                  <div className="text-[10px] text-dim">No deletions yet.</div>
                ) : (
                  <div className="deletion-log">
                    {deletionHistory.map((d, i) => (
                      <div key={`${d.profile_id}-${i}`} className="deletion-log-row">
                        <span className="deletion-log-name" title={d.email || d.label}>
                          {d.label || d.email || d.profile_id}
                        </span>
                        <span className="deletion-log-meta">
                          {(d.provider || "").toUpperCase()} · {new Date(d.deleted_at).toLocaleString()}
                        </span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
```

- [ ] **Step 10: CSS + build + commit**

Append to `desktop-app/frontend/src/style.css`:

```css
.deletion-log {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 180px;
  overflow-y: auto;
}
.deletion-log-row {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  font-size: 10px;
}
.deletion-log-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.deletion-log-meta {
  color: #7fe9ff;
  white-space: nowrap;
}
```

Run: `cd desktop-app/frontend && npm run build`
Expected: `tsc && vite build` clean.

```bash
git add desktop-app/frontend/src/App.tsx desktop-app/frontend/src/style.css
git commit -m "feat: show deletion history in settings"
```

---

## Self-Review Notes

- **Spec coverage:** ProfileState fields (T1) · per-account stamp in RunTrigger path via persistLastTriggerRun (T1) · reuse CreatedAt (T1 buildSnapshot) · ProfileCard fields + formats (T1) · Codex-only render of both lines (T2) · test the stamp (T1 Step 2) · frontend build (T2). All covered.
- **No new field for added-date** — reuses `Profile.CreatedAt` per the locked decision.
- **Stamp keys by `res.ProfileID`** (the profile's own ID set in `RunTrigger`), so the card the user sees (keyed by `Profile.ID`) matches.
- Types consistent: `LastTriggeredAt *time.Time` / `LastTriggeredModel string` used identically in model, service, and app-layer formatting.
- **Task 3 coverage:** `DeletedAccountRecord` + `State.DeletionHistory` (T3 Step 1) · record on delete in `LogoutProfile` after `RemoveProfile` (T3 Step 5) · pure `appendDeletionRecord` prepend+cap with test (T3 Steps 2/4) · `GetDeletionHistory` binding (T3 Step 7) · DELETION_LOG UI + load-on-open (T3 Step 9). All-providers per the spec. Snake_case JSON on the model, camelCase not needed (record is read as-is by the frontend via snake_case fields).
