package service

import (
	"testing"
	"time"

	"codex-lover/internal/model"
)

func codexStatus(id string, weekly float64, enabled bool) model.ProfileStatus {
	return model.ProfileStatus{
		Profile: model.Profile{ID: id, Label: id, Tool: model.ToolCodex, Enabled: enabled},
		State: model.ProfileState{
			AuthStatus: model.AuthStatusLoggedOut,
			Usage:      &model.UsageSnapshot{Secondary: &model.UsageWindow{RemainingPercent: weekly}},
		},
	}
}

func allCached(model.Profile) (string, bool) { return "src", true }

func TestSelectTriggerTargetsTopN(t *testing.T) {
	statuses := []model.ProfileStatus{
		codexStatus("a", 30, true),
		codexStatus("b", 90, true),
		codexStatus("c", 60, true),
	}
	cfg := model.TriggerConfig{Mode: model.TriggerModeTopN, Count: 2}
	selected, _ := selectTriggerTargets(statuses, cfg, func(p model.Profile) (string, bool) { return p.ID, true })
	if len(selected) != 2 {
		t.Fatalf("selected %d, want 2", len(selected))
	}
	if selected[0].Status.Profile.ID != "b" || selected[1].Status.Profile.ID != "c" {
		t.Fatalf("wrong order: %s,%s", selected[0].Status.Profile.ID, selected[1].Status.Profile.ID)
	}
}

func TestSelectTriggerTargetsAllSkipsNoAuth(t *testing.T) {
	statuses := []model.ProfileStatus{codexStatus("a", 10, true), codexStatus("b", 20, true)}
	cfg := model.TriggerConfig{Mode: model.TriggerModeAll}
	cached := func(p model.Profile) (string, bool) { return p.ID, p.ID == "a" }
	selected, skipped := selectTriggerTargets(statuses, cfg, cached)
	if len(selected) != 1 || selected[0].Status.Profile.ID != "a" {
		t.Fatalf("selected wrong set")
	}
	if len(skipped) != 1 || skipped[0].Reason != model.TriggerStatusSkippedNoAuth {
		t.Fatalf("expected b skipped no-auth")
	}
}

func TestSelectTriggerTargetsCustom(t *testing.T) {
	statuses := []model.ProfileStatus{codexStatus("a", 10, true), codexStatus("b", 20, true)}
	cfg := model.TriggerConfig{Mode: model.TriggerModeCustom, ProfileIDs: []string{"b"}}
	selected, _ := selectTriggerTargets(statuses, cfg, allCached)
	if len(selected) != 1 || selected[0].Status.Profile.ID != "b" {
		t.Fatalf("custom did not pick b")
	}
}

func TestSelectTriggerTargetsSkipsNonCodex(t *testing.T) {
	claude := model.ProfileStatus{Profile: model.Profile{ID: "cl", Tool: model.ToolClaude, Enabled: true}}
	statuses := []model.ProfileStatus{codexStatus("a", 10, true), claude}
	selected, _ := selectTriggerTargets(statuses, model.TriggerConfig{Mode: model.TriggerModeAll}, allCached)
	if len(selected) != 1 || selected[0].Status.Profile.ID != "a" {
		t.Fatalf("should only pick codex")
	}
}

func TestShouldFireTrigger(t *testing.T) {
	loc := time.Local
	base := time.Date(2026, 7, 8, 8, 0, 0, 0, loc)
	cfg := model.TriggerConfig{Enabled: true, TimeOfDay: "08:00", GraceMins: 60}
	cases := []struct {
		name    string
		now     time.Time
		lastDay string
		cfg     model.TriggerConfig
		want    bool
	}{
		{"disabled", base, "", model.TriggerConfig{Enabled: false, TimeOfDay: "08:00", GraceMins: 60}, false},
		{"at time", base, "", cfg, true},
		{"within grace", base.Add(30 * time.Minute), "", cfg, true},
		{"before time", base.Add(-time.Minute), "", cfg, false},
		{"past grace", base.Add(61 * time.Minute), "", cfg, false},
		{"already ran", base, "2026-07-08", cfg, false},
	}
	for _, tc := range cases {
		got, _ := shouldFireTrigger(tc.now, tc.cfg, tc.lastDay)
		if got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
