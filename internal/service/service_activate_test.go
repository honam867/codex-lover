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

// A manual switch to an account that is at its usage limit must be blocked
// up-front: the daemon's auto-switch would only bounce it straight back, so we
// refuse the switch and leave the runtime auth untouched.
func TestActivateProfileBlocksWhenTargetAtLimit(t *testing.T) {
	svc, st, runtimeHome := newTestService(t)
	now := time.Now().UTC()

	active := model.Profile{
		ID: "codex-active", Label: "active", Tool: model.ToolCodex, Provider: model.ToolCodex,
		HomePath: runtimeHome, Email: "active@example.com", AccountID: "acct-active",
		Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	limited := model.Profile{
		ID: "codex-limited", Label: "limited", Tool: model.ToolCodex, Provider: model.ToolCodex,
		HomePath: runtimeHome, Email: "limited@example.com", AccountID: "acct-limited",
		Enabled: true, CreatedAt: now, UpdatedAt: now,
	}

	cfg := store.DefaultConfig()
	cfg.Profiles = []model.Profile{active, limited}
	if err := st.SaveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	state := store.DefaultState()
	state.Profiles[active.ID] = model.ProfileState{
		ProfileID:  active.ID,
		AuthStatus: model.AuthStatusActive,
		Usage:      &model.UsageSnapshot{Primary: &model.UsageWindow{RemainingPercent: 90}},
	}
	state.Profiles[limited.ID] = model.ProfileState{
		ProfileID:  limited.ID,
		AuthStatus: model.AuthStatusLoggedOut,
		Usage:      &model.UsageSnapshot{Primary: &model.UsageWindow{RemainingPercent: 0}},
	}
	if err := st.SaveState(state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	if err := os.MkdirAll(runtimeHome, 0o700); err != nil {
		t.Fatalf("create runtime home: %v", err)
	}
	original := []byte(`{"active":"current"}`)
	if err := os.WriteFile(filepath.Join(runtimeHome, "auth.json"), original, 0o600); err != nil {
		t.Fatalf("write runtime auth: %v", err)
	}

	// The limited account has cached auth, so the ONLY reason to block is the limit.
	cachedHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(cachedHome, "auth.json"), []byte(`{"cached":"limited"}`), 0o600); err != nil {
		t.Fatalf("write cached auth: %v", err)
	}
	if err := codex.CacheHomeAuth(svc.codexAuthCacheRoot(), limited.ID, cachedHome); err != nil {
		t.Fatalf("cache limited auth: %v", err)
	}

	if _, err := svc.ActivateProfile(limited.ID); err == nil {
		t.Fatal("expected ActivateProfile to be blocked for an at-limit account, got nil error")
	}

	got, err := os.ReadFile(filepath.Join(runtimeHome, "auth.json"))
	if err != nil {
		t.Fatalf("read runtime auth: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("runtime auth.json must be unchanged when switch is blocked, got %q", string(got))
	}
}

// Switching to a healthy account must still work (guard must not over-block).
func TestActivateProfileAllowsHealthyTarget(t *testing.T) {
	svc, st, runtimeHome := newTestService(t)
	now := time.Now().UTC()

	active := model.Profile{
		ID: "codex-active", Label: "active", Tool: model.ToolCodex, Provider: model.ToolCodex,
		HomePath: runtimeHome, Email: "active@example.com", AccountID: "acct-active",
		Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	healthy := model.Profile{
		ID: "codex-healthy", Label: "healthy", Tool: model.ToolCodex, Provider: model.ToolCodex,
		HomePath: runtimeHome, Email: "healthy@example.com", AccountID: "acct-healthy",
		Enabled: true, CreatedAt: now, UpdatedAt: now,
	}

	cfg := store.DefaultConfig()
	cfg.Profiles = []model.Profile{active, healthy}
	if err := st.SaveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	state := store.DefaultState()
	state.Profiles[active.ID] = model.ProfileState{
		ProfileID:  active.ID,
		AuthStatus: model.AuthStatusActive,
		Usage:      &model.UsageSnapshot{Primary: &model.UsageWindow{RemainingPercent: 10}},
	}
	state.Profiles[healthy.ID] = model.ProfileState{
		ProfileID:  healthy.ID,
		AuthStatus: model.AuthStatusLoggedOut,
		Usage:      &model.UsageSnapshot{Primary: &model.UsageWindow{RemainingPercent: 80}},
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
	cachedAuth := []byte(`{"cached":"healthy"}`)
	if err := os.WriteFile(filepath.Join(cachedHome, "auth.json"), cachedAuth, 0o600); err != nil {
		t.Fatalf("write cached auth: %v", err)
	}
	if err := codex.CacheHomeAuth(svc.codexAuthCacheRoot(), healthy.ID, cachedHome); err != nil {
		t.Fatalf("cache healthy auth: %v", err)
	}

	if _, err := svc.ActivateProfile(healthy.ID); err != nil {
		t.Fatalf("expected healthy switch to succeed, got: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(runtimeHome, "auth.json"))
	if err != nil {
		t.Fatalf("read runtime auth: %v", err)
	}
	if string(got) != string(cachedAuth) {
		t.Fatalf("runtime auth.json should be replaced by healthy cache, got %q", string(got))
	}
}
