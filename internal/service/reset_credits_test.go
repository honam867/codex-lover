package service

import (
	"regexp"
	"testing"

	"codex-lover/internal/codex"
	"codex-lover/internal/model"
)

func TestResolveResetAuthTarget(t *testing.T) {
	active := model.ProfileStatus{
		Profile: model.Profile{ID: "a", Tool: model.ToolCodex, HomePath: `C:\home\.codex`},
		State:   model.ProfileState{AuthStatus: model.AuthStatusActive},
	}
	loggedOut := model.ProfileStatus{
		Profile: model.Profile{ID: "b", Tool: model.ToolCodex},
		State:   model.ProfileState{AuthStatus: model.AuthStatusLoggedOut},
	}
	noCache := model.ProfileStatus{
		Profile: model.Profile{ID: "c", Tool: model.ToolCodex},
		State:   model.ProfileState{AuthStatus: model.AuthStatusLoggedOut},
	}
	claude := model.ProfileStatus{
		Profile: model.Profile{ID: "d", Tool: model.ToolClaude},
		State:   model.ProfileState{AuthStatus: model.AuthStatusActive},
	}
	statuses := []model.ProfileStatus{active, loggedOut, noCache, claude}
	cachedSource := func(p model.Profile) (string, bool) {
		if p.ID == "b" {
			return "b-source", true
		}
		return "", false
	}

	tests := []struct {
		name       string
		profileID  string
		wantErr    bool
		wantHome   string
		wantSource string
	}{
		{name: "active uses runtime home auth", profileID: "a", wantHome: `C:\home\.codex`},
		{name: "logged out uses cached auth", profileID: "b", wantSource: "b-source"},
		{name: "no cached auth errors", profileID: "c", wantErr: true},
		{name: "non codex errors", profileID: "d", wantErr: true},
		{name: "unknown profile errors", profileID: "zzz", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := resolveResetAuthTarget(statuses, tt.profileID, cachedSource)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", target)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if target.HomePath != tt.wantHome || target.SourceProfileID != tt.wantSource {
				t.Fatalf("got %+v", target)
			}
		})
	}
}

func TestApplyResetCreditCount(t *testing.T) {
	state := model.State{Profiles: map[string]model.ProfileState{
		"a": {ProfileID: "a", Usage: &model.UsageSnapshot{PlanType: "plus"}},
		"b": {ProfileID: "b"},
	}}

	updated, changed := applyResetCreditCount(state, "a", 4)
	if got := updated.Profiles["a"].Usage.ResetCreditsAvailable; !changed || got == nil || *got != 4 {
		t.Fatalf("expected count 4, got %v (changed=%v)", got, changed)
	}
	if _, changedAgain := applyResetCreditCount(updated, "a", 4); changedAgain {
		t.Fatalf("same count must not report a change")
	}
	if updated.Profiles["a"].Usage.PlanType != "plus" {
		t.Fatalf("other usage fields must be kept")
	}
	if state.Profiles["a"].Usage.ResetCreditsAvailable != nil {
		t.Fatalf("original state must not be mutated")
	}

	// Without a usage snapshot there is nothing to attach the count to.
	untouched, changed := applyResetCreditCount(state, "b", 2)
	if changed || untouched.Profiles["b"].Usage != nil {
		t.Fatalf("profile without usage must stay untouched")
	}
}

func TestHomeAuthMatchesTarget(t *testing.T) {
	target := resetAuthTarget{AccountID: "acct-1", Email: "a@example.com"}
	tests := []struct {
		name string
		auth codex.ProfileAuth
		want bool
	}{
		{name: "same account", auth: codex.ProfileAuth{AccountID: "ACCT-1", Email: "A@example.com"}, want: true},
		{name: "different account id", auth: codex.ProfileAuth{AccountID: "acct-2", Email: "a@example.com"}, want: false},
		{name: "same team workspace, different user", auth: codex.ProfileAuth{AccountID: "acct-1", Email: "b@example.com"}, want: false},
		{name: "unknown identity is allowed", auth: codex.ProfileAuth{}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := homeAuthMatchesTarget(&tt.auth, target); got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestNewRedeemRequestIDIsUUIDv4(t *testing.T) {
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	first, second := newRedeemRequestID(), newRedeemRequestID()
	if !pattern.MatchString(first) {
		t.Fatalf("not a UUIDv4: %q", first)
	}
	if first == second {
		t.Fatalf("ids must be unique")
	}
}
