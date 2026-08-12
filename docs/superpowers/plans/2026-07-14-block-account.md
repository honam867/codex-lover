# Block Account (Codex-only) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users block a Codex account so manual activation, automatic limit switching, and automatic rotation cannot select it, while preserving the invariant that the active Codex account is never blocked.

**Architecture:** Persist `Blocked` on `model.Profile`, enforce it in the service selection and activation paths, and expose one mutex-protected Wails action to the existing React card UI. Blocking an active account first restores the best eligible cached replacement and only then persists the block; this ordering keeps the invariant intact if auth restore fails.

**Tech Stack:** Go 1.x, JSON-backed local store, Wails v2.12, React 18, TypeScript 5.6, Vite 5, lucide-react, plain CSS.

## Global Constraints

- Scope is Codex-only; Claude and Kimi behavior and cards remain unchanged.
- A blocked account cannot be activated manually or selected by `AutoSwitchLimitedCodex` or `AutoRotateCodex`.
- The active Codex account must never remain blocked: switch to a usable non-blocked cached account first, or refuse the block without changing `Profile.Blocked`.
- Refusing an active-account block must return exactly `cannot block the active account: no other usable account to switch to`.
- Activating a blocked account must return exactly `account is blocked; unblock it first`.
- Block and unblock act immediately without a confirmation dialog; errors use the existing `applyAction` status bar rather than introducing a new notification system.
- `Profile.UpdatedAt` is not changed by block/unblock because the approved data-model behavior only changes `Blocked`.
- `desktop-app/frontend/wailsjs/` is generated and gitignored; run `wails build -clean` to regenerate bindings instead of editing those files manually.

---

## File Structure

- Modify `internal/model/types.go`: persist the `Profile.Blocked` flag.
- Create `internal/service/block.go`: own `BlockResult` and `Service.SetProfileBlocked`.
- Create `internal/service/block_test.go`: service fixtures and block/selection/activation tests.
- Modify `internal/service/service.go`: activation guard, automatic-selection exclusions, and merge preservation.
- Modify `internal/service/profile_merge_test.go`: regression coverage for deduplication.
- Modify `desktop-app/app.go`: Wails action and snapshot DTO mapping.
- Create `desktop-app/app_snapshot_test.go`: verify the backend-to-frontend blocked contract.
- Modify `desktop-app/frontend/src/App.tsx`: Codex-only block/unblock controls and rendering.
- Modify `desktop-app/frontend/src/style.css`: blocked card and badge styles.

### Task 1: Persist and Preserve the Blocked Flag

**Files:**
- Modify: `internal/model/types.go:63-77`
- Modify: `internal/service/service.go:1810-1832`
- Modify: `internal/service/profile_merge_test.go`

**Interfaces:**
- Produces: `model.Profile.Blocked bool` serialized as optional JSON field `blocked`.
- Produces: `mergeCanonicalProfile` returns `Blocked == true` when either merged profile is blocked.

- [ ] **Step 1: Write the failing merge regression test**

Append to `internal/service/profile_merge_test.go`:

```go
func TestMergeCanonicalProfilePreservesBlocked(t *testing.T) {
	cases := []struct {
		name      string
		canonical bool
		duplicate bool
		want      bool
	}{
		{name: "neither blocked", want: false},
		{name: "canonical blocked", canonical: true, want: true},
		{name: "duplicate blocked", duplicate: true, want: true},
		{name: "both blocked", canonical: true, duplicate: true, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeCanonicalProfile(
				model.Profile{ID: "canonical", Blocked: tc.canonical},
				model.Profile{ID: "duplicate", Blocked: tc.duplicate},
				time.Now().UTC(),
			)
			if got.Blocked != tc.want {
				t.Fatalf("Blocked = %v, want %v", got.Blocked, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```powershell
go test ./internal/service -run TestMergeCanonicalProfilePreservesBlocked -count=1
```

Expected: build failure because `model.Profile` has no `Blocked` field.

- [ ] **Step 3: Add the persisted field and merge rule**

Add after `Price` in `internal/model/types.go`:

```go
	Blocked        bool      `json:"blocked,omitempty"`
```

Add beside the existing boolean merges in `mergeCanonicalProfile`:

```go
	canonical.Enabled = canonical.Enabled || duplicate.Enabled
	canonical.AutoDiscovered = canonical.AutoDiscovered || duplicate.AutoDiscovered
	canonical.Blocked = canonical.Blocked || duplicate.Blocked
```

- [ ] **Step 4: Run the focused and package tests**

Run:

```powershell
go test ./internal/service -run TestMergeCanonicalProfilePreservesBlocked -count=1
go test ./internal/model ./internal/service
```

Expected: both commands pass.

- [ ] **Step 5: Commit**

```powershell
git add internal/model/types.go internal/service/service.go internal/service/profile_merge_test.go
git commit -m "feat: persist Codex account block state"
```

### Task 2: Exclude Blocked Accounts From Activation and Automatic Selection

**Files:**
- Create: `internal/service/block_test.go`
- Modify: `internal/service/service.go:637-700,817-865,914-940`

**Interfaces:**
- Consumes: `model.Profile.Blocked` from Task 1.
- Produces: `ActivateProfile(profileID string)` refuses blocked profiles before the already-active and cached-auth branches.
- Produces: `bestSwitchCandidate` and `AutoRotateCodex` ignore blocked Codex profiles.

- [ ] **Step 1: Add reusable service fixtures and failing guard/selection tests**

Create `internal/service/block_test.go`:

```go
package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codex-lover/internal/model"
	"codex-lover/internal/store"
)

func newBlockTestService(t *testing.T) (*Service, *store.Store, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	st, err := store.New()
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := st.Ensure(); err != nil {
		t.Fatalf("Store.Ensure: %v", err)
	}
	return New(st), st, filepath.Join(home, ".codex")
}

func writeBlockTestCache(t *testing.T, st *store.Store, profileID, contents string) {
	t.Helper()
	root := filepath.Join(st.Root(), "codex-auth")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("MkdirAll cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, profileID+".json"), []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile cache: %v", err)
	}
}

func blockTestStatus(id, home, authStatus string, remaining float64, blocked bool) model.ProfileStatus {
	return model.ProfileStatus{
		Profile: model.Profile{
			ID:       id,
			Label:    id,
			Tool:     model.ToolCodex,
			Provider: model.ToolCodex,
			HomePath: home,
			Enabled:  true,
			Blocked:  blocked,
		},
		State: model.ProfileState{
			ProfileID:  id,
			AuthStatus: authStatus,
			Usage: &model.UsageSnapshot{
				Primary: &model.UsageWindow{RemainingPercent: remaining},
			},
		},
	}
}

func saveBlockTestProfiles(t *testing.T, st *store.Store, statuses []model.ProfileStatus) {
	t.Helper()
	cfg := store.DefaultConfig()
	state := store.DefaultState()
	for _, status := range statuses {
		cfg.Profiles = append(cfg.Profiles, status.Profile)
		state.Profiles[status.Profile.ID] = status.State
	}
	if err := st.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if err := st.SaveState(state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
}

func loadBlockTestProfile(t *testing.T, st *store.Store, profileID string) model.Profile {
	t.Helper()
	cfg, err := st.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	for _, profile := range cfg.Profiles {
		if profile.ID == profileID {
			return profile
		}
	}
	t.Fatalf("profile %q not found", profileID)
	return model.Profile{}
}

func TestBestSwitchCandidateSkipsBlocked(t *testing.T) {
	svc, st, home := newBlockTestService(t)
	active := blockTestStatus("active", home, model.AuthStatusActive, 0, false)
	blocked := blockTestStatus("blocked", home, model.AuthStatusLoggedOut, 90, true)
	fallback := blockTestStatus("fallback", home, model.AuthStatusLoggedOut, 60, false)
	writeBlockTestCache(t, st, blocked.Profile.ID, "blocked-auth")
	writeBlockTestCache(t, st, fallback.Profile.ID, "fallback-auth")

	got, ok := svc.bestSwitchCandidate([]model.ProfileStatus{active, blocked, fallback}, active)
	if !ok || got.Profile.ID != fallback.Profile.ID {
		t.Fatalf("candidate = %q, ok = %v; want fallback", got.Profile.ID, ok)
	}
}

func TestAutoRotateCodexSkipsBlocked(t *testing.T) {
	svc, st, home := newBlockTestService(t)
	cfg := store.DefaultConfig()
	cfg.AutoRotateCodex = true
	cfg.AutoRotateThreshold = 5
	if err := st.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	active := blockTestStatus("active", home, model.AuthStatusActive, 30, false)
	blocked := blockTestStatus("blocked", home, model.AuthStatusLoggedOut, 90, true)
	writeBlockTestCache(t, st, active.Profile.ID, "active-auth")
	writeBlockTestCache(t, st, blocked.Profile.ID, "blocked-auth")

	result, err := svc.AutoRotateCodex([]model.ProfileStatus{active, blocked})
	if err != nil {
		t.Fatalf("AutoRotateCodex: %v", err)
	}
	if result.Changed || result.To.ID != "" {
		t.Fatalf("blocked candidate selected: %+v", result)
	}
}

func TestActivateProfileRejectsBlocked(t *testing.T) {
	svc, st, home := newBlockTestService(t)
	blocked := blockTestStatus("blocked", home, model.AuthStatusLoggedOut, 80, true)
	saveBlockTestProfiles(t, st, []model.ProfileStatus{blocked})

	_, err := svc.ActivateProfile(blocked.Profile.ID)
	if err == nil || !strings.Contains(err.Error(), "account is blocked; unblock it first") {
		t.Fatalf("ActivateProfile error = %v", err)
	}
}
```

- [ ] **Step 2: Run the focused tests to verify they fail**

Run:

```powershell
go test ./internal/service -run 'Test(BestSwitchCandidateSkipsBlocked|AutoRotateCodexSkipsBlocked|ActivateProfileRejectsBlocked)' -count=1
```

Expected: candidate/rotation assertions fail and activation returns the cached-credentials error instead of the blocked error.

- [ ] **Step 3: Add the three blocked guards**

In `ActivateProfile`, immediately after the not-found branch and before the active-status branch, add:

```go
	if selected.Profile.Blocked {
		return ActivateResult{}, fmt.Errorf("account is blocked; unblock it first")
	}
```

In the `AutoRotateCodex` candidate loop, make the first filter:

```go
		if status.Profile.Tool != model.ToolCodex || status.Profile.Blocked {
			continue
		}
```

In `bestSwitchCandidate`, make the first filter:

```go
		if status.Profile.Tool != model.ToolCodex || status.Profile.ID == active.Profile.ID || status.Profile.Blocked {
			continue
		}
```

- [ ] **Step 4: Run focused and service tests**

Run:

```powershell
go test ./internal/service -run 'Test(BestSwitchCandidateSkipsBlocked|AutoRotateCodexSkipsBlocked|ActivateProfileRejectsBlocked)' -count=1
go test ./internal/service
```

Expected: both commands pass.

- [ ] **Step 5: Commit**

```powershell
git add internal/service/service.go internal/service/block_test.go
git commit -m "feat: exclude blocked Codex accounts from switching"
```

### Task 3: Implement Transaction-Safe Block and Unblock

**Files:**
- Create: `internal/service/block.go`
- Modify: `internal/service/block_test.go`

**Interfaces:**
- Consumes: `Service.ProfileStatuses()` and `Service.bestSwitchCandidate(...)`.
- Produces: `BlockResult{Profile model.Profile, Blocked bool, Switched bool, To model.Profile}`.
- Produces: `SetProfileBlocked(profileID string, blocked bool) (BlockResult, error)`.

- [ ] **Step 1: Add failing service behavior tests**

Append to `internal/service/block_test.go`:

```go
func TestSetProfileBlockedBlocksInactiveAccount(t *testing.T) {
	svc, st, home := newBlockTestService(t)
	active := blockTestStatus("active", home, model.AuthStatusActive, 40, false)
	target := blockTestStatus("target", home, model.AuthStatusLoggedOut, 80, false)
	saveBlockTestProfiles(t, st, []model.ProfileStatus{active, target})

	result, err := svc.SetProfileBlocked(target.Profile.ID, true)
	if err != nil {
		t.Fatalf("SetProfileBlocked: %v", err)
	}
	if !result.Blocked || result.Switched || result.Profile.ID != target.Profile.ID {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !loadBlockTestProfile(t, st, target.Profile.ID).Blocked {
		t.Fatal("target block was not persisted")
	}
}

func TestSetProfileBlockedUnblocksAccount(t *testing.T) {
	svc, st, home := newBlockTestService(t)
	target := blockTestStatus("target", home, model.AuthStatusLoggedOut, 80, true)
	saveBlockTestProfiles(t, st, []model.ProfileStatus{target})

	result, err := svc.SetProfileBlocked(target.Profile.ID, false)
	if err != nil {
		t.Fatalf("SetProfileBlocked: %v", err)
	}
	if result.Blocked || result.Switched {
		t.Fatalf("unexpected result: %+v", result)
	}
	if loadBlockTestProfile(t, st, target.Profile.ID).Blocked {
		t.Fatal("target unblock was not persisted")
	}
}

func TestSetProfileBlockedSwitchesBeforeBlockingActiveAccount(t *testing.T) {
	svc, st, home := newBlockTestService(t)
	active := blockTestStatus("active", home, model.AuthStatusActive, 0, false)
	replacement := blockTestStatus("replacement", home, model.AuthStatusLoggedOut, 75, false)
	saveBlockTestProfiles(t, st, []model.ProfileStatus{active, replacement})
	writeBlockTestCache(t, st, replacement.Profile.ID, "replacement-auth")

	result, err := svc.SetProfileBlocked(active.Profile.ID, true)
	if err != nil {
		t.Fatalf("SetProfileBlocked: %v", err)
	}
	if !result.Blocked || !result.Switched || result.To.ID != replacement.Profile.ID {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !loadBlockTestProfile(t, st, active.Profile.ID).Blocked {
		t.Fatal("active profile block was not persisted after switch")
	}
	data, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if err != nil {
		t.Fatalf("ReadFile restored auth: %v", err)
	}
	if string(data) != "replacement-auth" {
		t.Fatalf("restored auth = %q, want replacement-auth", data)
	}
}

func TestSetProfileBlockedRefusesActiveAccountWithoutReplacement(t *testing.T) {
	svc, st, home := newBlockTestService(t)
	active := blockTestStatus("active", home, model.AuthStatusActive, 0, false)
	saveBlockTestProfiles(t, st, []model.ProfileStatus{active})

	result, err := svc.SetProfileBlocked(active.Profile.ID, true)
	if err == nil || err.Error() != "cannot block the active account: no other usable account to switch to" {
		t.Fatalf("SetProfileBlocked error = %v", err)
	}
	if result.Switched || loadBlockTestProfile(t, st, active.Profile.ID).Blocked {
		t.Fatalf("refused block changed state: %+v", result)
	}
}

func TestSetProfileBlockedRejectsNonCodexProfile(t *testing.T) {
	svc, st, home := newBlockTestService(t)
	claude := blockTestStatus("claude", home, model.AuthStatusLoggedOut, 80, false)
	claude.Profile.Tool = model.ToolClaude
	claude.Profile.Provider = model.ToolClaude
	saveBlockTestProfiles(t, st, []model.ProfileStatus{claude})

	_, err := svc.SetProfileBlocked(claude.Profile.ID, true)
	if err == nil || !strings.Contains(err.Error(), "is not a Codex account") {
		t.Fatalf("SetProfileBlocked error = %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify the API is missing**

Run:

```powershell
go test ./internal/service -run TestSetProfileBlocked -count=1
```

Expected: build failure because `Service.SetProfileBlocked` does not exist.

- [ ] **Step 3: Implement the service action with switch-before-persist ordering**

Create `internal/service/block.go`:

```go
package service

import (
	"fmt"

	"codex-lover/internal/codex"
	"codex-lover/internal/model"
)

type BlockResult struct {
	Profile  model.Profile
	Blocked  bool
	Switched bool
	To       model.Profile
}

func (s *Service) SetProfileBlocked(profileID string, blocked bool) (BlockResult, error) {
	statuses, err := s.ProfileStatuses()
	if err != nil {
		return BlockResult{}, err
	}

	var target model.ProfileStatus
	found := false
	for _, status := range statuses {
		if status.Profile.ID == profileID {
			target = status
			found = true
			break
		}
	}
	if !found {
		return BlockResult{}, fmt.Errorf("profile %q not found", profileID)
	}
	if target.Profile.Tool != model.ToolCodex {
		return BlockResult{}, fmt.Errorf("profile %q is not a Codex account", profileID)
	}

	if !blocked {
		target.Profile.Blocked = false
		if err := s.store.UpsertProfile(target.Profile); err != nil {
			return BlockResult{}, err
		}
		return BlockResult{Profile: target.Profile, Blocked: false}, nil
	}

	result := BlockResult{Profile: target.Profile, Blocked: true}
	if target.State.AuthStatus == model.AuthStatusActive {
		candidate, ok := s.bestSwitchCandidate(statuses, target)
		if !ok {
			return BlockResult{}, fmt.Errorf("cannot block the active account: no other usable account to switch to")
		}
		if err := codex.RestoreCachedHomeAuth(s.codexAuthCacheRoot(), candidate.Profile.ID, target.Profile.HomePath); err != nil {
			return BlockResult{}, err
		}
		result.Switched = true
		result.To = candidate.Profile
	}

	target.Profile.Blocked = true
	result.Profile = target.Profile
	if err := s.store.UpsertProfile(target.Profile); err != nil {
		return result, err
	}
	return result, nil
}
```

The restore intentionally precedes `UpsertProfile` for active accounts. If restore fails, the old account remains active and unblocked; if persistence fails after restore, the replacement is active and the original remains unblocked. Both failure paths preserve the invariant.

- [ ] **Step 4: Run service tests**

Run:

```powershell
go test ./internal/service -run 'Test(SetProfileBlocked|BestSwitchCandidate|AutoRotateCodex|ActivateProfile|MergeCanonicalProfilePreservesBlocked)' -count=1
go test ./internal/service
```

Expected: both commands pass.

- [ ] **Step 5: Commit**

```powershell
git add internal/service/block.go internal/service/block_test.go
git commit -m "feat: block and unblock Codex accounts"
```

### Task 4: Expose Blocking Through the Desktop Snapshot and Wails Action

**Files:**
- Modify: `desktop-app/app.go:27-54,153-181,387-418`
- Create: `desktop-app/app_snapshot_test.go`

**Interfaces:**
- Consumes: `Service.SetProfileBlocked(profileID string, blocked bool) (service.BlockResult, error)`.
- Produces: `App.SetProfileBlocked(profileID string, blocked bool) ActionResponse`.
- Produces: `ProfileCard.Blocked bool` serialized as `blocked`.

- [ ] **Step 1: Write the failing snapshot contract test**

Create `desktop-app/app_snapshot_test.go`:

```go
package main

import (
	"testing"

	"codex-lover/internal/model"
)

func TestBuildSnapshotIncludesBlocked(t *testing.T) {
	snapshot := buildSnapshot([]model.ProfileStatus{{
		Profile: model.Profile{
			ID:       "codex-a",
			Label:    "codex-a",
			Tool:     model.ToolCodex,
			Provider: model.ToolCodex,
			Blocked:  true,
		},
	}}, nil)

	if len(snapshot.Profiles) != 1 || !snapshot.Profiles[0].Blocked {
		t.Fatalf("snapshot profiles = %+v, want one blocked card", snapshot.Profiles)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```powershell
go test ./desktop-app -run TestBuildSnapshotIncludesBlocked -count=1
```

Expected: build failure because `ProfileCard.Blocked` does not exist.

- [ ] **Step 3: Add the DTO field and snapshot mapping**

Add to `ProfileCard` after `IsActive`:

```go
	Blocked             bool   `json:"blocked"`
```

Add to the `ProfileCard` literal in `buildSnapshot` after `IsActive`:

```go
			Blocked:             status.Profile.Blocked,
```

- [ ] **Step 4: Add the mutex-protected Wails action**

Add beside the existing profile actions in `desktop-app/app.go`:

```go
func (a *App) SetProfileBlocked(profileID string, blocked bool) ActionResponse {
	if err := a.ensureReady(); err != nil {
		return ActionResponse{Message: "Update failed", Error: err.Error(), Snapshot: a.mustSnapshotFallback()}
	}

	a.mu.Lock()
	result, err := a.svc.SetProfileBlocked(profileID, blocked)
	a.mu.Unlock()
	if err != nil {
		return ActionResponse{Message: "Update failed", Error: err.Error(), Snapshot: a.mustSnapshotFallback()}
	}

	snapshot, err := a.snapshot(result.Switched)
	message := "Blocked " + profileLabel(result.Profile)
	if !result.Blocked {
		message = "Unblocked " + profileLabel(result.Profile)
	}
	if err != nil {
		return ActionResponse{Message: message, Error: err.Error(), Snapshot: a.mustSnapshotFallback()}
	}
	return ActionResponse{Message: message, Snapshot: snapshot}
}
```

Do not defer the unlock: `snapshot` locks `a.mu` internally, so the service call must be unlocked first.

- [ ] **Step 5: Run desktop and full Go tests**

Run:

```powershell
go test ./desktop-app -run TestBuildSnapshotIncludesBlocked -count=1
go test ./...
```

Expected: both commands pass.

- [ ] **Step 6: Commit**

```powershell
git add desktop-app/app.go desktop-app/app_snapshot_test.go
git commit -m "feat: expose account blocking to desktop app"
```

### Task 5: Add the Codex-only Block/Unblock Card Control

**Files:**
- Modify: `desktop-app/frontend/src/App.tsx:1-59,297-340,423-543`
- Modify: `desktop-app/frontend/src/style.css:166-199,267-300`
- Generated, ignored: `desktop-app/frontend/wailsjs/go/main/App.js`
- Generated, ignored: `desktop-app/frontend/wailsjs/go/main/App.d.ts`
- Generated, ignored: `desktop-app/frontend/wailsjs/go/models.ts`

**Interfaces:**
- Consumes: generated `SetProfileBlocked(profileID: string, blocked: boolean): Promise<ActionResponse>`.
- Consumes: `ProfileCard.blocked: boolean`.
- Produces: one immediate Codex-only block/unblock button; no confirmation dialog.

- [ ] **Step 1: Add the TypeScript contract and handler before regenerating bindings**

In `desktop-app/frontend/src/App.tsx`:

```tsx
import {
  Activity,
  Ban,
  Check,
  Cpu,
  LayoutDashboard,
  Plus,
  RefreshCw,
  Settings,
  ShieldCheck,
  Trash2,
  X,
} from "lucide-react";
```

Add `SetProfileBlocked` to the existing Wails imports, add `blocked` after `isActive` in the local card type, and add the handler after `onActivate`:

```tsx
  SetProfileBlocked,
```

```tsx
  blocked: boolean;
```

```tsx
  async function onSetBlocked(profileId: string, blocked: boolean) {
    setBusyProfile(profileId);
    const result = await SetProfileBlocked(profileId, blocked);
    applyAction(result);
  }
```

- [ ] **Step 2: Add blocked card state, badge, and footer controls**

Extend the account-card `clsx` expression:

```tsx
                profile.isActive && `active active-${profile.provider.toLowerCase()}`,
                profile.provider.toLowerCase() === "codex" && profile.blocked && "account-card-blocked"
```

Replace the footer content with:

```tsx
              <div className="flex justify-between items-center mt-6 pt-4 border-t border-dashed border-[rgba(0,243,255,0.1)]">
                <div className="flex items-center gap-2">
                  <span className={clsx("text-[9px] px-2 py-0.5 rounded", badgeClass(profile.authStatus))}>
                    {profile.authStatus.replace('_', ' ')}
                  </span>
                  {profile.provider.toLowerCase() === "codex" && profile.blocked && (
                    <span className="blocked-badge">BLOCKED</span>
                  )}
                </div>
                <div className="flex gap-2">
                  {profile.canLoginFromCache && !profile.blocked && (
                    <button
                      onClick={(e) => { e.stopPropagation(); void onActivate(profile.id); }}
                      className="cyber-btn p-1.5"
                      disabled={busyProfile === profile.id}
                      title="RE-AUTHENTICATE"
                    >
                      <ShieldCheck size={14} />
                    </button>
                  )}
                  {profile.provider.toLowerCase() === "codex" && (
                    <button
                      onClick={(e) => { e.stopPropagation(); void onSetBlocked(profile.id, !profile.blocked); }}
                      className={clsx("cyber-btn p-1.5", !profile.blocked && "cyber-btn-danger")}
                      disabled={busyProfile === profile.id}
                      title={profile.blocked ? "UNBLOCK ACCOUNT" : "BLOCK ACCOUNT"}
                    >
                      {profile.blocked ? <Check size={14} /> : <Ban size={14} />}
                    </button>
                  )}
                  <button
                    onClick={(e) => { e.stopPropagation(); void onDelete(profile.id, profile.label); }}
                    className="cyber-btn cyber-btn-danger p-1.5"
                    disabled={busyProfile === profile.id}
                    title="TERMINATE"
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>
```

All action buttons retain `stopPropagation()` so toggling block does not open the existing Codex metadata editor.

- [ ] **Step 3: Add restrained blocked styling**

Add to `desktop-app/frontend/src/style.css` after the active card rules:

```css
.account-card.account-card-blocked {
  opacity: 0.58;
  border-color: rgba(128, 128, 144, 0.45);
  box-shadow: none;
}

.account-card.account-card-blocked:hover {
  opacity: 0.72;
  border-color: rgba(128, 128, 144, 0.7);
  box-shadow: none;
}

.blocked-badge {
  border: 1px solid rgba(255, 0, 85, 0.35);
  border-radius: 4px;
  background: rgba(255, 0, 85, 0.1);
  color: var(--neon-pink);
  padding: 2px 8px;
  font-size: 9px;
}
```

- [ ] **Step 4: Regenerate Wails bindings and build the app**

Run from the repository root:

```powershell
Push-Location desktop-app
wails build -clean
Pop-Location
```

Expected: Wails regenerates the ignored bindings with `SetProfileBlocked` and `blocked`, then `tsc`, Vite, Go compilation, and Windows packaging complete successfully.

If validating only the frontend after bindings already exist, use the Windows executable to avoid PowerShell's `npm.ps1` execution-policy wrapper:

```powershell
Push-Location desktop-app/frontend
npm.cmd run build
Pop-Location
```

Expected: `tsc && vite build` passes.

- [ ] **Step 5: Run the complete automated regression suite**

Run:

```powershell
go test ./...
go build ./...
```

Expected: both commands pass.

- [ ] **Step 6: Perform the desktop behavior checks**

Run the rebuilt desktop app and verify:

1. A non-active Codex account blocks immediately, dims, shows `BLOCKED`, and loses its re-authenticate button.
2. The same account unblocks immediately and becomes eligible for re-authentication again.
3. Claude and Kimi cards have no block button and no blocked styling.
4. A blocked Codex account is skipped when limit-based switching and auto-rotation select an account.
5. Blocking the active Codex account switches to the highest-quota eligible cached replacement, refreshes usage, and syncs OpenCode through the existing runtime flow.
6. Blocking the active Codex account with no eligible cached replacement leaves it active and unblocked and shows `ERROR: cannot block the active account: no other usable account to switch to` in the existing status bar.

- [ ] **Step 7: Commit**

```powershell
git add desktop-app/frontend/src/App.tsx desktop-app/frontend/src/style.css
git commit -m "feat: add Codex account block controls"
```

Do not add `desktop-app/frontend/wailsjs/`; it is generated and ignored.
