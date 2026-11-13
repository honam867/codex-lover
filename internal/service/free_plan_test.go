package service

import (
	"os"
	"path/filepath"
	"testing"

	"codex-lover/internal/model"
)

// The active account dropped to free: it is deleted and the best plus account
// with quota is logged in. A free logged-out account is deleted too and is
// never picked as the replacement.
func TestRemoveFreeCodexAccountsSwitchesToPlus(t *testing.T) {
	svc, st, home := newBlockTestService(t)
	active := blockTestStatus("active", home, model.AuthStatusActive, 80, false)
	active.State.Usage.PlanType = "free"
	freeSpare := blockTestStatus("free-spare", home, model.AuthStatusLoggedOut, 100, false)
	freeSpare.State.Usage.PlanType = "free"
	plus := blockTestStatus("plus", home, model.AuthStatusLoggedOut, 60, false)
	plus.State.Usage.PlanType = "plus"
	statuses := []model.ProfileStatus{active, freeSpare, plus}
	saveBlockTestProfiles(t, st, statuses)
	for _, s := range statuses {
		writeBlockTestCache(t, st, s.Profile.ID, s.Profile.ID+"-auth")
	}

	out, err := svc.RemoveFreeCodexAccounts(statuses)
	if err != nil {
		t.Fatalf("RemoveFreeCodexAccounts: %v", err)
	}
	if len(out.Removed) != 2 {
		t.Fatalf("removed %d accounts, want 2 (active + free-spare)", len(out.Removed))
	}
	if !out.Switched.Changed || out.Switched.To.ID != "plus" {
		t.Fatalf("switched = %+v, want switch to plus", out.Switched)
	}
	cfg := loadBlockTestConfig(t, st)
	if len(cfg.Profiles) != 1 || cfg.Profiles[0].ID != "plus" {
		t.Fatalf("profiles left = %+v, want only plus", cfg.Profiles)
	}
	got, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if err != nil || string(got) != "plus-auth" {
		t.Fatalf("home auth = %q, %v; want plus-auth", got, err)
	}
}
