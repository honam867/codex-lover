package service

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"codex-lover/internal/codex"
	"codex-lover/internal/model"
)

type ProfileHealthResult struct {
	ProfileID string
	Status    string
	Message   string
	ModelUsed string
}

func (s *Service) CheckCodexProfileHealthByID(statuses []model.ProfileStatus, profileID string) (ProfileHealthResult, error) {
	profiles := statusesToProfiles(statuses)
	result, ok := checkSingleCodexProfileHealthStatus(
		statuses,
		profileID,
		s.cachedSourceFunc(profiles),
		func(sourceProfileID string) (*codex.TriggerResult, error) {
			return codex.TriggerHealthProbeFromCachedAuth(s.codexAuthCacheRoot(), sourceProfileID)
		},
	)
	if !ok {
		return ProfileHealthResult{}, fmt.Errorf("Codex profile %q not found", profileID)
	}

	state, err := s.store.LoadState()
	if err != nil {
		return result, err
	}
	state = applyProfileHealthStamps(state, []ProfileHealthResult{result}, time.Now().UTC())
	if err := s.store.SaveState(state); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Service) CheckCodexProfileHealth(statuses []model.ProfileStatus) ([]ProfileHealthResult, error) {
	profiles := statusesToProfiles(statuses)
	results := checkCodexProfileHealthStatuses(
		statuses,
		s.cachedSourceFunc(profiles),
		func(sourceProfileID string) (*codex.TriggerResult, error) {
			return codex.TriggerHealthProbeFromCachedAuth(s.codexAuthCacheRoot(), sourceProfileID)
		},
	)

	state, err := s.store.LoadState()
	if err != nil {
		return results, err
	}
	state = applyProfileHealthStamps(state, results, time.Now().UTC())
	if err := s.store.SaveState(state); err != nil {
		return results, err
	}
	return results, nil
}

func checkCodexProfileHealthStatuses(
	statuses []model.ProfileStatus,
	cachedSource func(model.Profile) (string, bool),
	trigger func(sourceProfileID string) (*codex.TriggerResult, error),
) []ProfileHealthResult {
	results := make([]ProfileHealthResult, 0, len(statuses))
	probed := map[string]ProfileHealthResult{}
	for _, status := range statuses {
		if status.Profile.Tool != model.ToolCodex {
			continue
		}
		sourceProfileID, ok := cachedSource(status.Profile)
		if !ok {
			results = append(results, profileHealthNoAuth(status.Profile.ID))
			continue
		}

		if cached, ok := probed[sourceProfileID]; ok {
			cached.ProfileID = status.Profile.ID
			results = append(results, cached)
			continue
		}

		result, err := trigger(sourceProfileID)
		health := classifyProfileHealth(status.Profile.ID, result, err)
		probed[sourceProfileID] = health
		results = append(results, health)
	}
	return results
}

func checkSingleCodexProfileHealthStatus(
	statuses []model.ProfileStatus,
	profileID string,
	cachedSource func(model.Profile) (string, bool),
	trigger func(sourceProfileID string) (*codex.TriggerResult, error),
) (ProfileHealthResult, bool) {
	for _, status := range statuses {
		if status.Profile.ID != profileID || status.Profile.Tool != model.ToolCodex {
			continue
		}
		sourceProfileID, ok := cachedSource(status.Profile)
		if !ok {
			return profileHealthNoAuth(status.Profile.ID), true
		}
		result, err := trigger(sourceProfileID)
		return classifyProfileHealth(status.Profile.ID, result, err), true
	}
	return ProfileHealthResult{}, false
}

func classifyProfileHealth(profileID string, result *codex.TriggerResult, err error) ProfileHealthResult {
	if err == nil {
		modelUsed := ""
		if result != nil {
			modelUsed = result.ModelUsed
		}
		return ProfileHealthResult{ProfileID: profileID, Status: model.HealthStatusOK, Message: "Probe OK", ModelUsed: modelUsed}
	}

	var triggerErr *codex.TriggerError
	if errors.As(err, &triggerErr) {
		switch triggerErr.StatusCode {
		case http.StatusUnauthorized:
			return ProfileHealthResult{ProfileID: profileID, Status: model.HealthStatusFailed, Message: "Unauthorized or expired auth"}
		case http.StatusForbidden:
			return ProfileHealthResult{ProfileID: profileID, Status: model.HealthStatusFailed, Message: "Forbidden or blocked"}
		case http.StatusTooManyRequests:
			return ProfileHealthResult{ProfileID: profileID, Status: model.HealthStatusLimited, Message: "Quota or rate limited"}
		}
	}

	return ProfileHealthResult{ProfileID: profileID, Status: model.HealthStatusFailed, Message: "Probe request failed"}
}

func profileHealthNoAuth(profileID string) ProfileHealthResult {
	return ProfileHealthResult{ProfileID: profileID, Status: model.HealthStatusNoAuth, Message: "Cached auth missing"}
}

func applyProfileHealthStamps(state model.State, results []ProfileHealthResult, checkedAt time.Time) model.State {
	if state.Profiles == nil {
		state.Profiles = map[string]model.ProfileState{}
	}
	for _, result := range results {
		profileState := state.Profiles[result.ProfileID]
		profileState.ProfileID = result.ProfileID
		profileState.HealthStatus = result.Status
		profileState.HealthMessage = result.Message
		profileState.HealthCheckedAt = &checkedAt
		profileState.HealthCheckedModel = result.ModelUsed
		state.Profiles[result.ProfileID] = profileState
	}
	return state
}
