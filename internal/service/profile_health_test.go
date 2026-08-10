package service

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"codex-lover/internal/codex"
	"codex-lover/internal/model"
)

func TestClassifyProfileHealth(t *testing.T) {
	tests := []struct {
		name       string
		result     *codex.TriggerResult
		err        error
		wantStatus string
		wantMsg    string
		wantModel  string
	}{
		{
			name:       "success",
			result:     &codex.TriggerResult{ModelUsed: "gpt-5.4-mini", Status: http.StatusOK},
			wantStatus: model.HealthStatusOK,
			wantMsg:    "Probe OK",
			wantModel:  "gpt-5.4-mini",
		},
		{
			name:       "unauthorized",
			err:        &codex.TriggerError{StatusCode: http.StatusUnauthorized},
			wantStatus: model.HealthStatusFailed,
			wantMsg:    "Unauthorized or expired auth",
		},
		{
			name:       "forbidden",
			err:        &codex.TriggerError{StatusCode: http.StatusForbidden},
			wantStatus: model.HealthStatusFailed,
			wantMsg:    "Forbidden or blocked",
		},
		{
			name:       "rate limited is skipped instead of failed",
			err:        &codex.TriggerError{StatusCode: http.StatusTooManyRequests},
			wantStatus: model.HealthStatusLimited,
			wantMsg:    "Quota or rate limited",
		},
		{
			name:       "generic failure",
			err:        errors.New("dial tcp failed"),
			wantStatus: model.HealthStatusFailed,
			wantMsg:    "Probe request failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyProfileHealth("acc-a", tt.result, tt.err)
			if got.ProfileID != "acc-a" {
				t.Fatalf("ProfileID = %q, want acc-a", got.ProfileID)
			}
			if got.Status != tt.wantStatus {
				t.Fatalf("Status = %q, want %q", got.Status, tt.wantStatus)
			}
			if got.Message != tt.wantMsg {
				t.Fatalf("Message = %q, want %q", got.Message, tt.wantMsg)
			}
			if got.ModelUsed != tt.wantModel {
				t.Fatalf("ModelUsed = %q, want %q", got.ModelUsed, tt.wantModel)
			}
		})
	}
}

func TestProfileHealthNoAuth(t *testing.T) {
	got := profileHealthNoAuth("acc-a")
	if got.Status != model.HealthStatusNoAuth {
		t.Fatalf("Status = %q, want %q", got.Status, model.HealthStatusNoAuth)
	}
	if got.Message != "Cached auth missing" {
		t.Fatalf("Message = %q, want Cached auth missing", got.Message)
	}
}

func TestApplyProfileHealthStamps(t *testing.T) {
	checkedAt := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	state := model.State{Profiles: map[string]model.ProfileState{
		"acc-a": {ProfileID: "acc-a", AuthStatus: model.AuthStatusLoggedOut},
	}}
	results := []ProfileHealthResult{
		{ProfileID: "acc-a", Status: model.HealthStatusOK, Message: "Probe OK", ModelUsed: "gpt-5.4-mini"},
		{ProfileID: "acc-b", Status: model.HealthStatusFailed, Message: "Forbidden or blocked"},
	}

	out := applyProfileHealthStamps(state, results, checkedAt)

	a := out.Profiles["acc-a"]
	if a.HealthStatus != model.HealthStatusOK {
		t.Fatalf("acc-a HealthStatus = %q, want %q", a.HealthStatus, model.HealthStatusOK)
	}
	if a.HealthMessage != "Probe OK" {
		t.Fatalf("acc-a HealthMessage = %q, want Probe OK", a.HealthMessage)
	}
	if a.HealthCheckedAt == nil || !a.HealthCheckedAt.Equal(checkedAt) {
		t.Fatalf("acc-a HealthCheckedAt = %v, want %v", a.HealthCheckedAt, checkedAt)
	}
	if a.HealthCheckedModel != "gpt-5.4-mini" {
		t.Fatalf("acc-a HealthCheckedModel = %q, want gpt-5.4-mini", a.HealthCheckedModel)
	}
	if a.AuthStatus != model.AuthStatusLoggedOut {
		t.Fatalf("existing ProfileState fields must be preserved")
	}

	b := out.Profiles["acc-b"]
	if b.ProfileID != "acc-b" || b.HealthStatus != model.HealthStatusFailed {
		t.Fatalf("acc-b state not created with health result: %+v", b)
	}
}

func TestCheckCodexProfileHealthStatusesReusesSourceProbe(t *testing.T) {
	statuses := []model.ProfileStatus{
		{Profile: model.Profile{ID: "visible-a", Tool: model.ToolCodex}},
		{Profile: model.Profile{ID: "visible-b", Tool: model.ToolCodex}},
	}
	probeCount := 0

	results := checkCodexProfileHealthStatuses(
		statuses,
		func(model.Profile) (string, bool) { return "source-a", true },
		func(sourceProfileID string) (*codex.TriggerResult, error) {
			probeCount++
			if sourceProfileID != "source-a" {
				t.Fatalf("sourceProfileID = %q, want source-a", sourceProfileID)
			}
			return &codex.TriggerResult{ModelUsed: "gpt-5.4-mini", Status: http.StatusOK}, nil
		},
	)

	if probeCount != 1 {
		t.Fatalf("probeCount = %d, want 1", probeCount)
	}
	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results))
	}
	for _, result := range results {
		if result.Status != model.HealthStatusOK || result.ModelUsed != "gpt-5.4-mini" {
			t.Fatalf("unexpected reused result: %+v", result)
		}
	}
	if results[0].ProfileID != "visible-a" || results[1].ProfileID != "visible-b" {
		t.Fatalf("visible profile IDs must be preserved: %+v", results)
	}
}

func TestCheckSingleCodexProfileHealthStatusOnlyProbesSelectedProfile(t *testing.T) {
	statuses := []model.ProfileStatus{
		{Profile: model.Profile{ID: "visible-a", Tool: model.ToolCodex}},
		{Profile: model.Profile{ID: "visible-b", Tool: model.ToolCodex}},
	}
	var probed []string

	result, ok := checkSingleCodexProfileHealthStatus(
		statuses,
		"visible-b",
		func(p model.Profile) (string, bool) { return "source-" + p.ID, true },
		func(sourceProfileID string) (*codex.TriggerResult, error) {
			probed = append(probed, sourceProfileID)
			return &codex.TriggerResult{ModelUsed: "gpt-5.4-mini", Status: http.StatusOK}, nil
		},
	)

	if !ok {
		t.Fatalf("expected selected Codex profile to be found")
	}
	if len(probed) != 1 || probed[0] != "source-visible-b" {
		t.Fatalf("probed = %v, want only source-visible-b", probed)
	}
	if result.ProfileID != "visible-b" || result.Status != model.HealthStatusOK {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestCheckSingleCodexProfileHealthStatusIgnoresNonCodexProfile(t *testing.T) {
	statuses := []model.ProfileStatus{
		{Profile: model.Profile{ID: "claude-a", Tool: model.ToolClaude}},
	}
	called := false

	_, ok := checkSingleCodexProfileHealthStatus(
		statuses,
		"claude-a",
		func(model.Profile) (string, bool) { return "source", true },
		func(string) (*codex.TriggerResult, error) {
			called = true
			return &codex.TriggerResult{}, nil
		},
	)

	if ok {
		t.Fatalf("non-Codex profile must not be selectable for Codex health check")
	}
	if called {
		t.Fatalf("trigger must not be called for non-Codex profile")
	}
}
