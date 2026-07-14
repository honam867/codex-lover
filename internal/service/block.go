package service

import (
	"fmt"

	"codex-lover/internal/codex"
	"codex-lover/internal/model"
)

type BlockResult struct {
	Profile  model.Profile
	Blocked  bool
	Switched bool
	To       model.Profile
}

func (s *Service) SetProfileBlocked(profileID string, blocked bool) (BlockResult, error) {
	statuses, err := s.ProfileStatuses()
	if err != nil {
		return BlockResult{}, err
	}

	var target model.ProfileStatus
	found := false
	for _, status := range statuses {
		if status.Profile.ID == profileID {
			target = status
			found = true
			break
		}
	}
	if !found {
		return BlockResult{}, fmt.Errorf("profile %q not found", profileID)
	}
	if target.Profile.Tool != model.ToolCodex {
		return BlockResult{}, fmt.Errorf("profile %q is not a Codex account", profileID)
	}

	if !blocked {
		target.Profile.Blocked = false
		if err := s.store.UpsertProfile(target.Profile); err != nil {
			return BlockResult{}, err
		}
		return BlockResult{Profile: target.Profile, Blocked: false}, nil
	}

	result := BlockResult{Profile: target.Profile, Blocked: true}
	if target.State.AuthStatus == model.AuthStatusActive {
		candidate, ok := s.bestSwitchCandidate(statuses, target)
		if !ok {
			return BlockResult{}, fmt.Errorf("cannot block the active account: no other usable account to switch to")
		}
		if err := codex.RestoreCachedHomeAuth(s.codexAuthCacheRoot(), candidate.Profile.ID, target.Profile.HomePath); err != nil {
			return BlockResult{}, err
		}
		result.Switched = true
		result.To = candidate.Profile
	}

	target.Profile.Blocked = true
	result.Profile = target.Profile
	if err := s.store.UpsertProfile(target.Profile); err != nil {
		return result, err
	}
	return result, nil
}
