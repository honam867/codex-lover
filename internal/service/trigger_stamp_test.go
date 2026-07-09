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
