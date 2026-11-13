package service

import (
	"strings"

	"codex-lover/internal/codex"
	"codex-lover/internal/model"
)

// isFreeCodexPlan reports a Codex account whose live plan (from the usage API)
// is "free". Such an account can no longer use the Codex models ("model is not
// supported when using Codex with a ChatGPT account").
func isFreeCodexPlan(status model.ProfileStatus) bool {
	return status.Profile.Tool == model.ToolCodex &&
		status.State.Usage != nil &&
		strings.EqualFold(strings.TrimSpace(status.State.Usage.PlanType), "free")
}

// ActiveCodexIsFree reports whether the active Codex account dropped to free.
func ActiveCodexIsFree(statuses []model.ProfileStatus) bool {
	active, ok := activeCodexStatus(statuses)
	return ok && isFreeCodexPlan(active)
}

// FreeCodexCleanup is what RemoveFreeCodexAccounts did.
type FreeCodexCleanup struct {
	Removed  []model.Profile
	Switched SwitchResult
}

// RemoveFreeCodexAccounts deletes Codex accounts whose plan dropped to free.
// When the active one is removed, the account with the most quota left is
// logged in instead.
func (s *Service) RemoveFreeCodexAccounts(statuses []model.ProfileStatus) (FreeCodexCleanup, error) {
	var out FreeCodexCleanup
	for _, status := range statuses {
		if !isFreeCodexPlan(status) {
			continue
		}
		if _, err := s.LogoutProfile(status.Profile.ID); err != nil {
			return out, err
		}
		out.Removed = append(out.Removed, status.Profile)
		if status.State.AuthStatus == model.AuthStatusActive {
			switched, err := s.ActivateBestCodexAfter(status.Profile)
			if err != nil {
				return out, err
			}
			out.Switched = switched
		}
	}
	return out, nil
}

// ActivateBestCodexAfter logs the Codex account with the most quota left into
// the home of a just-removed active account, so Codex keeps working.
func (s *Service) ActivateBestCodexAfter(removed model.Profile) (SwitchResult, error) {
	statuses, err := s.store.ProfileStatuses()
	if err != nil {
		return SwitchResult{}, err
	}
	candidate, ok := s.bestSwitchCandidate(statuses, model.ProfileStatus{Profile: removed})
	if !ok {
		return SwitchResult{Checked: true, From: removed, Reason: "no cached ready account"}, nil
	}
	if err := codex.RestoreCachedHomeAuth(s.codexAuthCacheRoot(), candidate.Profile.ID, removed.HomePath); err != nil {
		return SwitchResult{Checked: true, From: removed, To: candidate.Profile}, err
	}
	return SwitchResult{Checked: true, Changed: true, From: removed, To: candidate.Profile, Reason: "previous active account removed"}, nil
}
