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
