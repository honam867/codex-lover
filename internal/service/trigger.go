package service

import (
	"time"

	"codex-lover/internal/codex"
	"codex-lover/internal/model"
)

func (s *Service) cachedSourceFunc(profiles []model.Profile) func(model.Profile) (string, bool) {
	return func(p model.Profile) (string, bool) {
		return s.cachedAuthSourceProfileID(p, profiles)
	}
}

// RunTrigger selects and triggers the configured Codex accounts, persists the
// run to state, and returns the run summary. It never switches the active account.
// onProgress (may be nil) receives the run so far: once with every selected
// account pending, then before and after each account is triggered.
func (s *Service) RunTrigger(statuses []model.ProfileStatus, cfg model.TriggerConfig, manual bool, onProgress func(model.TriggerRun)) (model.TriggerRun, error) {
	profiles := statusesToProfiles(statuses)
	selected, skipped := selectTriggerTargets(statuses, cfg, s.cachedSourceFunc(profiles))

	run := model.TriggerRun{RanAt: time.Now().UTC(), Manual: manual}
	for _, t := range selected {
		run.Results = append(run.Results, model.TriggerAccountResult{
			ProfileID: t.Status.Profile.ID,
			Label:     profileLabel(t.Status.Profile),
			Status:    model.TriggerStatusPending,
		})
	}
	for _, sk := range skipped {
		run.Results = append(run.Results, model.TriggerAccountResult{
			ProfileID: sk.Status.Profile.ID,
			Label:     profileLabel(sk.Status.Profile),
			Status:    sk.Reason,
		})
	}
	report := func() {
		if onProgress != nil {
			snapshot := run
			snapshot.Results = append([]model.TriggerAccountResult(nil), run.Results...)
			onProgress(snapshot)
		}
	}
	report()

	for i, t := range selected {
		res := &run.Results[i]
		res.Status = model.TriggerStatusRunning
		report()
		result, err := codex.TriggerFromCachedAuth(s.codexAuthCacheRoot(), t.SourceProfileID, nil)
		if err != nil {
			res.Status = model.TriggerStatusError
			res.Error = err.Error()
		} else {
			res.Status = model.TriggerStatusOpened
			res.ModelUsed = result.ModelUsed
			res.Verified = s.verifyTriggerOpened(t.SourceProfileID)
		}
		report()
	}

	if err := s.persistLastTriggerRun(run); err != nil {
		return run, err
	}
	return run, nil
}

func (s *Service) verifyTriggerOpened(sourceProfileID string) bool {
	usage, _, err := codex.FetchUsageFromCachedAuth(s.codexAuthCacheRoot(), sourceProfileID)
	if err != nil || usage == nil || usage.Primary == nil || usage.Primary.ResetsAt == nil {
		return false
	}
	return usage.Primary.ResetsAt.After(time.Now())
}

// applyTriggerStamps records the run as the last run and stamps each
// successfully-opened account's per-profile last-trigger time and model.
func applyTriggerStamps(state model.State, run model.TriggerRun) model.State {
	if state.Profiles == nil {
		state.Profiles = map[string]model.ProfileState{}
	}
	saved := run
	state.LastTriggerRun = &saved
	for _, res := range run.Results {
		if res.Status != model.TriggerStatusOpened {
			continue
		}
		ps := state.Profiles[res.ProfileID]
		ps.ProfileID = res.ProfileID
		ranAt := run.RanAt
		ps.LastTriggeredAt = &ranAt
		ps.LastTriggeredModel = res.ModelUsed
		state.Profiles[res.ProfileID] = ps
	}
	return state
}

func (s *Service) persistLastTriggerRun(run model.TriggerRun) error {
	state, err := s.store.LoadState()
	if err != nil {
		return err
	}
	state = applyTriggerStamps(state, run)
	return s.store.SaveState(state)
}

// RunScheduledTrigger runs the trigger set only if it is due. Returns ran=false
// when not due. On a successful run it stamps LastTriggerDate so it fires once/day.
func (s *Service) RunScheduledTrigger(now time.Time, statuses []model.ProfileStatus) (bool, model.TriggerRun, error) {
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return false, model.TriggerRun{}, err
	}
	state, err := s.store.LoadState()
	if err != nil {
		return false, model.TriggerRun{}, err
	}
	fire, _ := shouldFireTrigger(now, cfg.Trigger, state.LastTriggerDate)
	if !fire {
		return false, model.TriggerRun{}, nil
	}
	// Claim today's slot BEFORE running so a persist failure or a crash mid-run
	// cannot cause the 15s scheduler to re-fire (and re-hit every account) within
	// the grace window. Fail-safe: if we cannot record the claim, skip rather than hammer.
	state.LastTriggerDate = now.Format("2006-01-02")
	if err := s.store.SaveState(state); err != nil {
		return false, model.TriggerRun{}, err
	}
	run, err := s.RunTrigger(statuses, cfg.Trigger, false, nil)
	return true, run, err
}

func (s *Service) LastTriggerRun() (*model.TriggerRun, error) {
	state, err := s.store.LoadState()
	if err != nil {
		return nil, err
	}
	return state.LastTriggerRun, nil
}

// TriggerSelectionItem is a UI-facing preview row.
type TriggerSelectionItem struct {
	ProfileID string `json:"profileId"`
	Label     string `json:"label"`
	Reason    string `json:"reason"`
}

func (s *Service) PreviewTriggerSelection(statuses []model.ProfileStatus, cfg model.TriggerConfig) ([]string, []TriggerSelectionItem) {
	profiles := statusesToProfiles(statuses)
	selected, skipped := selectTriggerTargets(statuses, cfg, s.cachedSourceFunc(profiles))
	selectedIDs := make([]string, 0, len(selected))
	for _, t := range selected {
		selectedIDs = append(selectedIDs, t.Status.Profile.ID)
	}
	skips := make([]TriggerSelectionItem, 0, len(skipped))
	for _, sk := range skipped {
		skips = append(skips, TriggerSelectionItem{
			ProfileID: sk.Status.Profile.ID,
			Label:     profileLabel(sk.Status.Profile),
			Reason:    sk.Reason,
		})
	}
	return selectedIDs, skips
}
