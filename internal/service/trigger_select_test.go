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
	selected, skipped := selectTriggerTargets(statuses, model.TriggerConfig{Mode: model.TriggerModeAll}, allCached)
	if len(selected) != 1 || selected[0].Status.Profile.ID != "a" {
		t.Fatalf("should only pick codex")
	}
	if len(skipped) != 0 {
		t.Fatalf("non-codex profile must not appear in skipped, got %+v", skipped)
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

func TestSelectTriggerTargetsCustomNoAuthSkip(t *testing.T) {
	statuses := []model.ProfileStatus{codexStatus("a", 10, true), codexStatus("b", 20, true)}
	cached := func(p model.Profile) (string, bool) { return p.ID, p.ID == "a" } // only "a" has cached auth
	// custom wants both; b has no auth and IS wanted -> skip
	cfg := model.TriggerConfig{Mode: model.TriggerModeCustom, ProfileIDs: []string{"a", "b"}}
	selected, skipped := selectTriggerTargets(statuses, cfg, cached)
	if len(selected) != 1 || selected[0].Status.Profile.ID != "a" {
		t.Fatalf("expected only a selected, got %v", selected)
	}
	if len(skipped) != 1 || skipped[0].Status.Profile.ID != "b" || skipped[0].Reason != model.TriggerStatusSkippedNoAuth {
		t.Fatalf("expected b skipped no-auth, got %+v", skipped)
	}
	// custom does NOT want the no-auth account -> no skip
	cfg2 := model.TriggerConfig{Mode: model.TriggerModeCustom, ProfileIDs: []string{"a"}}
	if _, skipped2 := selectTriggerTargets(statuses, cfg2, cached); len(skipped2) != 0 {
		t.Fatalf("expected no skips when no-auth account not wanted, got %+v", skipped2)
	}
}

func TestSelectTriggerTargetsTopNTieBreakAndClamp(t *testing.T) {
	mk := func(id string, weekly, primary float64) model.ProfileStatus {
		return model.ProfileStatus{
			Profile: model.Profile{ID: id, Label: id, Tool: model.ToolCodex, Enabled: true},
			State: model.ProfileState{Usage: &model.UsageSnapshot{
				Secondary: &model.UsageWindow{RemainingPercent: weekly},
				Primary:   &model.UsageWindow{RemainingPercent: primary},
			}},
		}
	}
	all := func(p model.Profile) (string, bool) { return p.ID, true }
	// equal weekly-secondary (50) -> ordered by primary (weekly) remaining desc: b(80) > c(40) > a(10)
	statuses := []model.ProfileStatus{mk("a", 50, 10), mk("b", 50, 80), mk("c", 50, 40)}
	selected, _ := selectTriggerTargets(statuses, model.TriggerConfig{Mode: model.TriggerModeTopN, Count: 2}, all)
	if len(selected) != 2 || selected[0].Status.Profile.ID != "b" || selected[1].Status.Profile.ID != "c" {
		t.Fatalf("tie-break by primary desc failed: %v", selected)
	}
	// Count > len -> clamp to len
	if sel, _ := selectTriggerTargets(statuses, model.TriggerConfig{Mode: model.TriggerModeTopN, Count: 99}, all); len(sel) != 3 {
		t.Fatalf("Count>len should clamp to len, got %d", len(sel))
	}
	// Count < 1 -> clamp to 1
	if sel, _ := selectTriggerTargets(statuses, model.TriggerConfig{Mode: model.TriggerModeTopN, Count: 0}, all); len(sel) != 1 {
		t.Fatalf("Count<1 should clamp to 1, got %d", len(sel))
	}
}

func TestSelectTriggerTargetsExcludesDisabled(t *testing.T) {
	statuses := []model.ProfileStatus{codexStatus("a", 10, true), codexStatus("b", 20, false)}
	selected, skipped := selectTriggerTargets(statuses, model.TriggerConfig{Mode: model.TriggerModeAll}, allCached)
	if len(selected) != 1 || selected[0].Status.Profile.ID != "a" {
		t.Fatalf("disabled profile should be excluded from selection, got %v", selected)
	}
	if len(skipped) != 0 {
		t.Fatalf("disabled profile should be silently excluded, not skipped, got %+v", skipped)
	}
}

func TestShouldFireTriggerGraceDefault(t *testing.T) {
	loc := time.Local
	sched := time.Date(2026, 7, 8, 8, 0, 0, 0, loc)
	cfg := model.TriggerConfig{Enabled: true, TimeOfDay: "08:00", GraceMins: 0} // 0 -> default 60
	if fire, _ := shouldFireTrigger(sched.Add(30*time.Minute), cfg, ""); !fire {
		t.Fatalf("within default grace (60) should fire")
	}
	if fire, _ := shouldFireTrigger(sched.Add(61*time.Minute), cfg, ""); fire {
		t.Fatalf("past default grace (60) should not fire")
	}
}
