# Codex Account Meta & Weekly-Only Limit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the user edit a Codex account's added-date and purchase price (VNĐ) from its card, and refactor the Codex quota to a single weekly limit (the 5h window is gone from the API).

**Architecture:** Add `Profile.Price` + a `UpdateProfileMeta` service/binding for editing date+price; expose price/created-ISO on `ProfileCard`. Consolidate the Codex-only quota helpers to one weekly window. Frontend: Codex cards show one WEEKLY meter, a click-to-edit modal, a price line, and a skippable price popup for newly-logged-in accounts.

**Tech Stack:** Go (stdlib), Wails v2 bindings, React + TypeScript.

## Global Constraints

- Module path is `codex-lover`; imports use `codex-lover/internal/...`.
- All three features are **Codex-only**: price, edit modal, price popup, and the single-weekly-meter render apply only when `provider === "codex"`. Claude/Kimi cards and logic are unchanged.
- Price is an integer **VNĐ** (`int64`), formatted with `Intl.NumberFormat("vi-VN")` → e.g. `300.000 ₫`. No decimals.
- The 5h concept is removed from Codex naming/logic/display; the shared `model.UsageSnapshot{Primary,Secondary}` stays (Claude/Kimi use it) — for Codex, `Primary` IS the weekly window.
- state/config JSON = snake_case; `ProfileCard` JSON = camelCase (match existing).
- `gofmt`/`goimports` mandatory; Go tests table-driven; no token/secret exposure. Frontend must `tsc && vite build` clean. Wails TS bindings live in a gitignored dir, regenerated with `wails generate module` — never committed.

---

## File Structure

| File | Change |
|---|---|
| `internal/model/types.go` | `Profile.Price int64` |
| `internal/service/profile_meta.go` *(new)* | pure `applyProfileMeta` + `(*Service).UpdateProfileMeta` |
| `internal/service/profile_meta_test.go` *(new)* | test `applyProfileMeta` |
| `internal/service/service.go` | consolidate quota helpers to one weekly window (Task 2) |
| `internal/service/weekly_quota_test.go` *(new)* | test weekly quota helpers |
| `desktop-app/app.go` | `UpdateProfileMeta` binding; `ProfileCard.Price`/`CreatedAtISO`; `formatCreatedAtISO`; populate in `buildSnapshot` |
| `desktop-app/frontend/src/App.tsx` | Codex single weekly meter + trigger text (T3); edit modal + price line (T4); price popup (T5) |
| `desktop-app/frontend/src/style.css` | edit-modal / price / popup styles |

---

### Task 1: Backend — Profile price + edit-meta service/binding

**Files:**
- Modify: `internal/model/types.go`
- Create: `internal/service/profile_meta.go`, `internal/service/profile_meta_test.go`
- Modify: `desktop-app/app.go`

**Interfaces:**
- Produces: `model.Profile.Price int64 \`json:"price,omitempty"\``; `applyProfileMeta(profile model.Profile, createdAt time.Time, price int64, now time.Time) model.Profile`; `(*Service).UpdateProfileMeta(profileID string, createdAt time.Time, price int64) (model.Profile, error)`; `(*App).UpdateProfileMeta(profileID string, createdAtISO string, price int64) ActionResponse`; `ProfileCard.Price int64 \`json:"price"\``, `ProfileCard.CreatedAtISO string \`json:"createdAtISO"\``; `formatCreatedAtISO(time.Time) string`.

- [ ] **Step 1: Add the Price field**

In `internal/model/types.go`, add to the `Profile` struct (after `Plan string ...`):

```go
	Price int64 `json:"price,omitempty"` // purchase price in VNĐ (integer dong)
```

- [ ] **Step 2: Write the failing test**

Create `internal/service/profile_meta_test.go`:

```go
package service

import (
	"testing"
	"time"

	"codex-lover/internal/model"
)

func TestApplyProfileMeta(t *testing.T) {
	created := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	base := model.Profile{ID: "codex-a", Label: "a", Tool: model.ToolCodex, Email: "a@x.com"}

	got := applyProfileMeta(base, created, 300000, now)
	if !got.CreatedAt.Equal(created) {
		t.Fatalf("CreatedAt = %v, want %v", got.CreatedAt, created)
	}
	if got.Price != 300000 {
		t.Fatalf("Price = %d, want 300000", got.Price)
	}
	if !got.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt not set to now")
	}
	if got.Email != "a@x.com" || got.Label != "a" {
		t.Fatalf("other fields must be preserved")
	}

	// zero createdAt keeps the existing date
	withDate := model.Profile{ID: "b", CreatedAt: created}
	got2 := applyProfileMeta(withDate, time.Time{}, 500000, now)
	if !got2.CreatedAt.Equal(created) {
		t.Fatalf("zero createdAt must keep existing date, got %v", got2.CreatedAt)
	}
	if got2.Price != 500000 {
		t.Fatalf("Price = %d, want 500000", got2.Price)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestApplyProfileMeta`
Expected: FAIL — `applyProfileMeta` undefined.

- [ ] **Step 4: Implement the service**

Create `internal/service/profile_meta.go`:

```go
package service

import (
	"fmt"
	"time"

	"codex-lover/internal/model"
)

// applyProfileMeta returns a copy of profile with an updated added-date and
// price. A zero createdAt keeps the existing date. Other fields are preserved.
func applyProfileMeta(profile model.Profile, createdAt time.Time, price int64, now time.Time) model.Profile {
	if !createdAt.IsZero() {
		profile.CreatedAt = createdAt
	}
	profile.Price = price
	profile.UpdatedAt = now
	return profile
}

// UpdateProfileMeta edits a profile's added-date and price (VNĐ) and persists it.
func (s *Service) UpdateProfileMeta(profileID string, createdAt time.Time, price int64) (model.Profile, error) {
	if price < 0 {
		return model.Profile{}, fmt.Errorf("price must be >= 0")
	}
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return model.Profile{}, err
	}
	for _, profile := range cfg.Profiles {
		if profile.ID != profileID {
			continue
		}
		updated := applyProfileMeta(profile, createdAt, price, time.Now().UTC())
		if err := s.store.UpsertProfile(updated); err != nil {
			return model.Profile{}, err
		}
		return updated, nil
	}
	return model.Profile{}, fmt.Errorf("profile %q not found", profileID)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/service/ -run TestApplyProfileMeta`
Expected: PASS

- [ ] **Step 6: Add the binding + ProfileCard fields**

In `desktop-app/app.go`, add the binding (place near `LogoutProfile`; `fmt`/`time` are already imported):

```go
func (a *App) UpdateProfileMeta(profileID string, createdAtISO string, price int64) ActionResponse {
	if err := a.ensureReady(); err != nil {
		return ActionResponse{Message: "Update failed", Error: err.Error(), Snapshot: a.mustSnapshotFallback()}
	}
	var createdAt time.Time
	if s := strings.TrimSpace(createdAtISO); s != "" {
		parsed, err := time.ParseInLocation("2006-01-02", s, time.Local)
		if err != nil {
			return ActionResponse{Message: "Update failed", Error: "invalid date: " + err.Error(), Snapshot: a.mustSnapshotFallback()}
		}
		createdAt = parsed.UTC()
	}
	a.mu.Lock()
	_, err := a.svc.UpdateProfileMeta(profileID, createdAt, price)
	a.mu.Unlock()
	if err != nil {
		return ActionResponse{Message: "Update failed", Error: err.Error(), Snapshot: a.mustSnapshotFallback()}
	}
	snapshot, err := a.snapshot(true)
	if err != nil {
		return ActionResponse{Message: "Updated", Error: err.Error(), Snapshot: a.mustSnapshotFallback()}
	}
	return ActionResponse{Message: "Updated", Snapshot: snapshot}
}
```

(`strings` is already imported in `desktop-app/app.go`.)

Add to the `ProfileCard` struct (after `LastTriggeredModel`):

```go
	Price        int64  `json:"price"`
	CreatedAtISO string `json:"createdAtISO"`
```

In `buildSnapshot`, inside the `ProfileCard{...}` literal (after `LastTriggeredModel: ...`), add:

```go
			Price:        status.Profile.Price,
			CreatedAtISO: formatCreatedAtISO(status.Profile.CreatedAt),
```

Add this helper near `formatCreatedAt` in `desktop-app/app.go`:

```go
func formatCreatedAtISO(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Local().Format("2006-01-02")
}
```

- [ ] **Step 7: Build + test + commit**

Run: `go build ./...` then `go test ./internal/service/`
Expected: build OK; tests PASS.

```bash
git add internal/model/types.go internal/service/profile_meta.go internal/service/profile_meta_test.go desktop-app/app.go
git commit -m "feat: add codex account price and edit-meta service/binding"
```

---

### Task 2: Backend — consolidate Codex quota to one weekly window

**Files:**
- Modify: `internal/service/service.go`
- Create: `internal/service/weekly_quota_test.go`

**Interfaces:**
- Produces: `weeklyWindow(status model.ProfileStatus) *model.UsageWindow`; simplified `weeklyRemaining`, `usageLimitReached`, `quotaScore` (all single-weekly-window). Removed: `fiveHourRemaining`, `windowLimitReached`, `minFloat`.

- [ ] **Step 1: Write the failing test**

Create `internal/service/weekly_quota_test.go`:

```go
package service

import (
	"testing"
	"time"

	"codex-lover/internal/model"
)

func weeklyStatus(remaining float64) model.ProfileStatus {
	return model.ProfileStatus{
		Profile: model.Profile{ID: "a", Tool: model.ToolCodex},
		State: model.ProfileState{
			AuthStatus: model.AuthStatusActive,
			Usage:      &model.UsageSnapshot{Primary: &model.UsageWindow{RemainingPercent: remaining}},
		},
	}
}

func TestWeeklyRemainingReadsSingleWindow(t *testing.T) {
	if got := weeklyRemaining(weeklyStatus(73)); got != 73 {
		t.Fatalf("weeklyRemaining = %v, want 73", got)
	}
	// falls back to Secondary if Primary is nil
	st := model.ProfileStatus{State: model.ProfileState{Usage: &model.UsageSnapshot{Secondary: &model.UsageWindow{RemainingPercent: 40}}}}
	if got := weeklyRemaining(st); got != 40 {
		t.Fatalf("weeklyRemaining fallback = %v, want 40", got)
	}
	if got := weeklyRemaining(model.ProfileStatus{}); got != 0 {
		t.Fatalf("weeklyRemaining nil-usage = %v, want 0", got)
	}
}

func TestUsageLimitReachedWeekly(t *testing.T) {
	if !usageLimitReached(weeklyStatus(0.4)) {
		t.Fatalf("0.4%% remaining should be limit-reached")
	}
	if usageLimitReached(weeklyStatus(20)) {
		t.Fatalf("20%% remaining should NOT be limit-reached")
	}
}

func TestQuotaScoreWeekly(t *testing.T) {
	score, ok := quotaScore(weeklyStatus(55), time.Now())
	if !ok || score != 55 {
		t.Fatalf("quotaScore = (%v,%v), want (55,true)", score, ok)
	}
	if _, ok := quotaScore(weeklyStatus(0.3), time.Now()); ok {
		t.Fatalf("exhausted window should be ok=false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run "TestWeeklyRemaining|TestUsageLimitReachedWeekly|TestQuotaScoreWeekly"`
Expected: FAIL — `weeklyWindow` undefined / signature mismatch.

- [ ] **Step 3: Consolidate the helpers**

In `internal/service/service.go`:

Replace `fiveHourRemaining` and `weeklyRemaining` (the two functions) with a single window accessor + weekly-remaining:

```go
// weeklyWindow returns the account's sole rate-limit window. Codex dropped the
// 5h window; the API now returns the weekly window as Primary (Secondary is a
// legacy fallback).
func weeklyWindow(status model.ProfileStatus) *model.UsageWindow {
	if status.State.Usage == nil {
		return nil
	}
	if status.State.Usage.Primary != nil {
		return status.State.Usage.Primary
	}
	return status.State.Usage.Secondary
}

func weeklyRemaining(status model.ProfileStatus) float64 {
	window := weeklyWindow(status)
	if window == nil {
		return 0
	}
	return window.RemainingPercent
}
```

Replace `usageLimitReached` + `windowLimitReached` with:

```go
func usageLimitReached(status model.ProfileStatus) bool {
	window := weeklyWindow(status)
	return window != nil && window.RemainingPercent <= 0.5
}
```

Replace `quotaScore` (and remove `minFloat` if now unused) with:

```go
func quotaScore(status model.ProfileStatus, now time.Time) (float64, bool) {
	window := EffectiveWindowForDisplay(weeklyWindow(status), status.State.AuthStatus, now)
	if window == nil || window.RemainingPercent <= 0.5 {
		return 0, false
	}
	return window.RemainingPercent, true
}
```

In `AutoRotateCodex`, replace the two `fiveHourRemaining(...)` calls (candidate sort and the `diff` computation) with `weeklyRemaining(...)`. The existing `weeklyRemaining(status) <= 0.5` candidate filter stays as-is.

- [ ] **Step 4: Fix any remaining callers + build**

Run: `grep -rn "fiveHourRemaining\|windowLimitReached\|minFloat" internal/`
Expected: no matches (all removed/repointed). If any remain, repoint `fiveHourRemaining`→`weeklyRemaining` and delete unused `windowLimitReached`/`minFloat`.

Run: `go build ./...`
Expected: build OK (no "declared and not used" / undefined errors).

- [ ] **Step 5: Run tests**

Run: `go test ./internal/service/`
Expected: all PASS (new weekly tests + existing suite).

- [ ] **Step 6: Commit**

```bash
git add internal/service/service.go internal/service/weekly_quota_test.go
git commit -m "refactor: consolidate codex quota to single weekly window (drop 5h)"
```

---

### Task 3: Frontend — Codex single weekly meter + trigger text

**Files:**
- Modify: `desktop-app/frontend/src/App.tsx`

**Interfaces:**
- Consumes: existing `ProfileCard` fields `primaryPercent`/`primarySummary`/`secondaryPercent`/`secondarySummary`, `provider`.

- [ ] **Step 1: Render one WEEKLY meter for Codex, keep two for others**

In `desktop-app/frontend/src/App.tsx`, replace the entire quota block — the `<div className="space-y-5"> ... </div>` currently containing the two `meter-block`s ("Quota: 5H" and "Quota: WEEKLY") — with a provider-conditional version:

```tsx
              <div className="space-y-5">
                {profile.provider.toLowerCase() === "codex" ? (
                  <div className="meter-block">
                    <div className="meter-label">
                      <span>Quota: WEEKLY</span>
                      <span className="text-neon">{profile.primarySummary}</span>
                    </div>
                    <div className="meter-track">
                      <div
                        className={clsx("meter-fill", meterTone(profile.primaryPercent))}
                        style={{ width: `${profile.primaryPercent}%` }}
                      />
                    </div>
                  </div>
                ) : (
                  <>
                    <div className="meter-block">
                      <div className="meter-label">
                        <span>Quota: 5H</span>
                        <span className="text-neon">{profile.primarySummary}</span>
                      </div>
                      <div className="meter-track">
                        <div
                          className={clsx("meter-fill", meterTone(profile.primaryPercent))}
                          style={{ width: `${profile.primaryPercent}%` }}
                        />
                      </div>
                    </div>
                    <div className="meter-block">
                      <div className="meter-label">
                        <span>Quota: WEEKLY</span>
                        <span className="text-neon">{profile.secondarySummary}</span>
                      </div>
                      <div className="meter-track">
                        <div
                          className={clsx("meter-fill", meterTone(profile.secondaryPercent))}
                          style={{ width: `${profile.secondaryPercent}%` }}
                        />
                      </div>
                    </div>
                  </>
                )}
              </div>
```

- [ ] **Step 2: Update trigger UI text from 5H to weekly**

In the AUTO_TRIGGER settings block, change the description line:

```tsx
                    <div className="text-[10px] text-dim">Open 5H quota window on a schedule</div>
```
to:
```tsx
                    <div className="text-[10px] text-dim">Open weekly quota window on a schedule</div>
```

In the Top-N preview row, change:
```tsx
                                  <span className="trigger-pick-quota">WK {p.secondaryPercent}%</span>
```
to:
```tsx
                                  <span className="trigger-pick-quota">WK {p.primaryPercent}%</span>
```

In the custom-pick row, change:
```tsx
                            <span className="trigger-pick-quota">5H {p.primaryPercent}% · WK {p.secondaryPercent}%</span>
```
to:
```tsx
                            <span className="trigger-pick-quota">WK {p.primaryPercent}%</span>
```

- [ ] **Step 3: Build**

Run: `cd desktop-app/frontend && npm run build`
Expected: `tsc && vite build` clean, zero TS errors.

- [ ] **Step 4: Commit**

```bash
git add desktop-app/frontend/src/App.tsx
git commit -m "feat: show single weekly meter for codex cards and update trigger text"
```

---

### Task 4: Frontend — click-to-edit modal + price line

**Files:**
- Modify: `desktop-app/frontend/src/App.tsx`, `desktop-app/frontend/src/style.css`

**Interfaces:**
- Consumes: `UpdateProfileMeta` binding + `ProfileCard.price`/`createdAtISO` (Task 1). Requires regenerated Wails bindings.

- [ ] **Step 1: Regenerate Wails bindings**

Run: `cd desktop-app && wails generate module` (long timeout up to 600000 ms). If it errors, run `wails build -clean`.
Confirm `desktop-app/frontend/wailsjs/go/main/App.d.ts` now declares `UpdateProfileMeta`. Do NOT commit the gitignored `wailsjs` dir.

- [ ] **Step 2: Extend the ProfileCard TS type + import + state + helper**

In `App.tsx`, add to `type ProfileCard = {...}` (after `lastTriggeredModel: string;`):

```tsx
  price: number;
  createdAtISO: string;
```

Add `UpdateProfileMeta` to the `../wailsjs/go/main/App` import.

Add a VND formatter near the other module-level helpers (e.g. next to `getProviderLogo`):

```tsx
const formatVND = (value: number): string =>
  `${new Intl.NumberFormat("vi-VN").format(value)} ₫`;
```

Add edit-modal state inside `App()` (near the other `useState`s):

```tsx
  const [editProfile, setEditProfile] = useState<ProfileCard | null>(null);
  const [editDate, setEditDate] = useState<string>("");
  const [editPrice, setEditPrice] = useState<number>(0);
```

Add handlers (near `onDelete`):

```tsx
  function openEdit(profile: ProfileCard) {
    setEditProfile(profile);
    setEditDate(profile.createdAtISO || "");
    setEditPrice(profile.price || 0);
  }

  async function saveEdit() {
    if (!editProfile) return;
    const result = await UpdateProfileMeta(editProfile.id, editDate, editPrice);
    setEditProfile(null);
    applyAction(result);
  }
```

- [ ] **Step 3: Open the modal on card click + stopPropagation on action buttons + add price line**

On the card `<article>` (Codex only opens the editor), add an `onClick`:

```tsx
            <article
              key={profile.id}
              onClick={() => { if (profile.provider.toLowerCase() === "codex") openEdit(profile); }}
              className={clsx(
                "account-card",
                profile.provider.toLowerCase() === "codex" && "account-card-clickable",
                profile.isActive && `active active-${profile.provider.toLowerCase()}`
              )}
            >
```

Change the two footer buttons so a click doesn't also open the editor — wrap their handlers to stop propagation:

```tsx
                    <button
                      onClick={(e) => { e.stopPropagation(); void onActivate(profile.id); }}
                      className="cyber-btn p-1.5"
                      disabled={busyProfile === profile.id}
                      title="RE-AUTHENTICATE"
                    >
                      <ShieldCheck size={14} />
                    </button>
```
```tsx
                  <button
                    onClick={(e) => { e.stopPropagation(); void onDelete(profile.id, profile.label); }}
                    className="cyber-btn cyber-btn-danger p-1.5"
                    disabled={busyProfile === profile.id}
                    title="TERMINATE"
                  >
                    <Trash2 size={14} />
                  </button>
```

In the codex `card-meta` block, add a price row (before the `Trigger` row inside the `<div className="card-meta">`):

```tsx
                    {profile.price > 0 && (
                      <div className="card-meta-row">
                        <span className="text-dim">Giá</span>
                        <span>{formatVND(profile.price)}</span>
                      </div>
                    )}
```

Also update the codex-meta wrapper condition so the block shows when there is a price even without trigger/added text — change:
```tsx
              {profile.provider.toLowerCase() === "codex" &&
                (profile.lastTriggeredAtText || profile.createdAtText) && (
```
to:
```tsx
              {profile.provider.toLowerCase() === "codex" &&
                (profile.price > 0 || profile.lastTriggeredAtText || profile.createdAtText) && (
```

- [ ] **Step 4: Add the edit modal JSX**

Near the other modals (e.g. after the settings modal block, before the prerequisite modal), add:

```tsx
      {editProfile && (
        <div className="modal-overlay" onClick={() => setEditProfile(null)}>
          <div className="modal-content" onClick={(e) => e.stopPropagation()}>
            <div className="flex justify-between items-center mb-6">
              <h2 className="text-neon text-lg font-bold">EDIT_ACCOUNT</h2>
              <button onClick={() => setEditProfile(null)}><X size={20} /></button>
            </div>
            <div className="space-y-5">
              <div>
                <div className="text-[11px] text-dim mb-1">NGÀY ADD</div>
                <input
                  type="date"
                  className="trigger-select w-full"
                  value={editDate}
                  onChange={(e) => setEditDate(e.target.value)}
                />
              </div>
              <div>
                <div className="text-[11px] text-dim mb-1">GIÁ TIỀN (VNĐ)</div>
                <input
                  type="number"
                  min={0}
                  className="trigger-select w-full"
                  value={editPrice}
                  onChange={(e) => setEditPrice(Math.max(0, Number(e.target.value)))}
                />
                {editPrice > 0 && <div className="text-[10px] text-dim mt-1">{formatVND(editPrice)}</div>}
              </div>
              <button onClick={() => void saveEdit()} className="cyber-btn cyber-btn-solid w-full">
                LƯU
              </button>
            </div>
          </div>
        </div>
      )}
```

- [ ] **Step 5: CSS**

Append to `desktop-app/frontend/src/style.css`:

```css
.account-card-clickable {
  cursor: pointer;
}
```

- [ ] **Step 6: Build + commit**

Run: `cd desktop-app/frontend && npm run build`
Expected: clean, zero TS errors.

```bash
git add desktop-app/frontend/src/App.tsx desktop-app/frontend/src/style.css
git commit -m "feat: click-to-edit codex card (date + price) and show price"
```

---

### Task 5: Frontend — skippable price popup on new Codex login

**Files:**
- Modify: `desktop-app/frontend/src/App.tsx`

**Interfaces:**
- Consumes: `UpdateProfileMeta`, `formatVND`, edit-price state pattern (Task 4), `snapshot.profiles`.

- [ ] **Step 1: Add popup state + detection effect**

In `App.tsx`, add state + refs (near the edit state):

```tsx
  const [pricePrompt, setPricePrompt] = useState<ProfileCard | null>(null);
  const [promptPrice, setPromptPrice] = useState<number>(0);
  const seenCodexIds = useRef<Set<string>>(new Set());
  const seededCodexIds = useRef<boolean>(false);
```

Add an effect that prompts for price only on **newly-appeared** Codex accounts with no price (existing accounts are seeded silently on first load):

```tsx
  useEffect(() => {
    const codex = snapshot.profiles.filter((p) => p.provider.toLowerCase() === "codex");
    if (!seededCodexIds.current) {
      codex.forEach((p) => seenCodexIds.current.add(p.id));
      seededCodexIds.current = true;
      return;
    }
    const fresh = codex.find((p) => !seenCodexIds.current.has(p.id));
    if (fresh) {
      codex.forEach((p) => seenCodexIds.current.add(p.id));
      if (fresh.price === 0 && !pricePrompt) {
        setPricePrompt(fresh);
        setPromptPrice(0);
      }
    }
  }, [snapshot.profiles, pricePrompt]);
```

- [ ] **Step 2: Add popup save/skip handlers + JSX**

Add handlers (near `saveEdit`):

```tsx
  async function savePrice() {
    if (!pricePrompt) return;
    const result = await UpdateProfileMeta(pricePrompt.id, pricePrompt.createdAtISO || "", promptPrice);
    setPricePrompt(null);
    applyAction(result);
  }
```

Add the popup JSX (near the edit modal):

```tsx
      {pricePrompt && (
        <div className="modal-overlay" onClick={() => setPricePrompt(null)}>
          <div className="modal-content" onClick={(e) => e.stopPropagation()}>
            <div className="flex justify-between items-center mb-4">
              <h2 className="text-neon text-lg font-bold">GIÁ TÀI KHOẢN</h2>
              <button onClick={() => setPricePrompt(null)}><X size={20} /></button>
            </div>
            <div className="text-[11px] text-dim mb-3 truncate" title={pricePrompt.email}>
              {pricePrompt.label || pricePrompt.email}
            </div>
            <input
              type="number"
              min={0}
              autoFocus
              placeholder="Nhập giá đã mua (VNĐ)"
              className="trigger-select w-full"
              value={promptPrice || ""}
              onChange={(e) => setPromptPrice(Math.max(0, Number(e.target.value)))}
            />
            {promptPrice > 0 && <div className="text-[10px] text-dim mt-1">{formatVND(promptPrice)}</div>}
            <div className="flex gap-3 mt-5">
              <button onClick={() => setPricePrompt(null)} className="cyber-btn flex-1">SKIP</button>
              <button onClick={() => void savePrice()} className="cyber-btn cyber-btn-solid flex-1">LƯU</button>
            </div>
          </div>
        </div>
      )}
```

- [ ] **Step 3: Build + commit**

Run: `cd desktop-app/frontend && npm run build`
Expected: clean, zero TS errors.

```bash
git add desktop-app/frontend/src/App.tsx
git commit -m "feat: prompt for price when a new codex account appears"
```

---

## Self-Review Notes

- **Spec coverage:** Profile.Price + UpdateProfileMeta (T1) · edit modal date+price (T4) · price on card (T4) · price popup on new login (T5) · single weekly meter for Codex (T3) · quota logic consolidation to weekly (T2) · trigger text 5H→weekly (T3) · all Codex-only (provider gate in T3/T4/T5, price fields Codex-populated). Covered.
- **Codex-only:** the edit modal, price line, and popup are all gated by `provider.toLowerCase() === "codex"`; the single-weekly meter is the Codex branch, Claude/Kimi keep two meters.
- **Model untouched:** `UsageSnapshot{Primary,Secondary}` unchanged; Task 2 only touches Codex-only service helpers. `buildSnapshot` still sends both windows; the frontend chooses per provider.
- **Types consistent:** `Price int64` (Go) ↔ `price: number` (TS); `createdAtISO` string `YYYY-MM-DD` used by `UpdateProfileMeta` binding and the date input; `weeklyWindow`/`weeklyRemaining` names consistent across T2 and its test.
- **Bindings:** regenerated in T4 (first frontend task needing `UpdateProfileMeta`); T5 reuses them.
