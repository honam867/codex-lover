package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"codex-lover/internal/codex"
	"codex-lover/internal/model"
	"codex-lover/internal/store"
)

func TestUsageLimitReachedOnWeeklyWindow(t *testing.T) {
	status := model.ProfileStatus{
		Profile: model.Profile{Tool: model.ToolCodex},
		State: model.ProfileState{
			Usage: &model.UsageSnapshot{
				Primary: &model.UsageWindow{
					RemainingPercent: 80,
				},
				Secondary: &model.UsageWindow{
					RemainingPercent: 0.5,
				},
			},
		},
	}

	if !usageLimitReached(status) {
		t.Fatal("expected weekly window to trigger limit reached")
	}
}

func TestQuotaScoreRejectsWeeklyLimitedCandidate(t *testing.T) {
	now := time.Now()
	status := model.ProfileStatus{
		Profile: model.Profile{Tool: model.ToolCodex},
		State: model.ProfileState{
			AuthStatus: model.AuthStatusLoggedOut,
			Usage: &model.UsageSnapshot{
				Primary: &model.UsageWindow{
					RemainingPercent: 80,
				},
				Secondary: &model.UsageWindow{
					RemainingPercent: 0.4,
				},
			},
		},
	}

	if score, ok := quotaScore(status, now); ok || score != 0 {
		t.Fatalf("expected weekly-limited candidate to be rejected, got score=%v ok=%v", score, ok)
	}
}

func TestActivateProfileRestoresCachedAuth(t *testing.T) {
	svc, st, runtimeHome := newTestService(t)
	now := time.Now().UTC()
	profile := model.Profile{
		ID:        "codex-target",
		Label:     "target",
		Tool:      model.ToolCodex,
		Provider:  model.ToolCodex,
		HomePath:  runtimeHome,
		Email:     "target@example.com",
		AccountID: "acct-target",
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	cfg := store.DefaultConfig()
	cfg.Profiles = []model.Profile{profile}
	if err := st.SaveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	state := store.DefaultState()
	state.Profiles[profile.ID] = model.ProfileState{
		ProfileID:  profile.ID,
		AuthStatus: model.AuthStatusLoggedOut,
		LastError:  "",
		Usage:      nil,
	}
	if err := st.SaveState(state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	if err := os.MkdirAll(runtimeHome, 0o700); err != nil {
		t.Fatalf("create runtime home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runtimeHome, "auth.json"), []byte(`{"current":"old"}`), 0o600); err != nil {
		t.Fatalf("write runtime auth: %v", err)
	}

	cachedHome := t.TempDir()
	cachedAuth := []byte(`{"cached":"target"}`)
	if err := os.WriteFile(filepath.Join(cachedHome, "auth.json"), cachedAuth, 0o600); err != nil {
		t.Fatalf("write cached auth source: %v", err)
	}
	if err := codex.CacheHomeAuth(svc.codexAuthCacheRoot(), profile.ID, cachedHome); err != nil {
		t.Fatalf("cache auth: %v", err)
	}

	result, err := svc.ActivateProfile(profile.ID)
	if err != nil {
		t.Fatalf("activate profile: %v", err)
	}
	if result.Profile.ID != profile.ID {
		t.Fatalf("expected activated profile %q, got %q", profile.ID, result.Profile.ID)
	}

	restored, err := os.ReadFile(filepath.Join(runtimeHome, "auth.json"))
	if err != nil {
		t.Fatalf("read restored auth: %v", err)
	}
	if string(restored) != string(cachedAuth) {
		t.Fatalf("expected runtime auth to be restored from cache, got %q", string(restored))
	}
}

func TestAutoSwitchLimitedCodexRestoresBestCachedCandidate(t *testing.T) {
	svc, st, runtimeHome := newTestService(t)
	now := time.Now().UTC()
	active := model.Profile{
		ID:        "codex-active",
		Label:     "active",
		Tool:      model.ToolCodex,
		Provider:  model.ToolCodex,
		HomePath:  runtimeHome,
		Email:     "active@example.com",
		AccountID: "acct-active",
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	candidate := model.Profile{
		ID:        "codex-candidate",
		Label:     "candidate",
		Tool:      model.ToolCodex,
		Provider:  model.ToolCodex,
		HomePath:  runtimeHome,
		Email:     "candidate@example.com",
		AccountID: "acct-candidate",
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	cfg := store.DefaultConfig()
	cfg.Profiles = []model.Profile{active, candidate}
	if err := st.SaveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	state := store.DefaultState()
	state.Profiles[active.ID] = model.ProfileState{
		ProfileID:  active.ID,
		AuthStatus: model.AuthStatusActive,
		Usage: &model.UsageSnapshot{
			Primary:   &model.UsageWindow{RemainingPercent: 0},
			Secondary: &model.UsageWindow{RemainingPercent: 80},
		},
	}
	state.Profiles[candidate.ID] = model.ProfileState{
		ProfileID:  candidate.ID,
		AuthStatus: model.AuthStatusLoggedOut,
		Usage: &model.UsageSnapshot{
			Primary:   &model.UsageWindow{RemainingPercent: 80},
			Secondary: &model.UsageWindow{RemainingPercent: 90},
		},
	}
	if err := st.SaveState(state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	if err := os.MkdirAll(runtimeHome, 0o700); err != nil {
		t.Fatalf("create runtime home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runtimeHome, "auth.json"), []byte(`{"active":"current"}`), 0o600); err != nil {
		t.Fatalf("write runtime auth: %v", err)
	}

	cachedHome := t.TempDir()
	candidateAuth := []byte(`{"cached":"candidate"}`)
	if err := os.WriteFile(filepath.Join(cachedHome, "auth.json"), candidateAuth, 0o600); err != nil {
		t.Fatalf("write candidate auth source: %v", err)
	}
	if err := codex.CacheHomeAuth(svc.codexAuthCacheRoot(), candidate.ID, cachedHome); err != nil {
		t.Fatalf("cache candidate auth: %v", err)
	}

	statuses, err := st.ProfileStatuses()
	if err != nil {
		t.Fatalf("load statuses: %v", err)
	}

	result, err := svc.AutoSwitchLimitedCodex(statuses)
	if err != nil {
		t.Fatalf("auto switch: %v", err)
	}
	if !result.Changed {
		t.Fatal("expected auto switch to change active account")
	}
	if result.To.ID != candidate.ID {
		t.Fatalf("expected candidate %q, got %q", candidate.ID, result.To.ID)
	}

	restored, err := os.ReadFile(filepath.Join(runtimeHome, "auth.json"))
	if err != nil {
		t.Fatalf("read restored auth: %v", err)
	}
	if string(restored) != string(candidateAuth) {
		t.Fatalf("expected active runtime auth to be replaced by candidate cache, got %q", string(restored))
	}
}

func newTestService(t *testing.T) (*Service, *store.Store, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	st, err := store.New()
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	if err := st.Ensure(); err != nil {
		t.Fatalf("ensure store: %v", err)
	}

	return New(st), st, filepath.Join(home, ".codex")
}
