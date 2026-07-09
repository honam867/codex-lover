package service

import (
	"time"

	"codex-lover/internal/model"
)

const deletionHistoryMax = 50

// appendDeletionRecord prepends rec (newest first) and trims to max entries.
func appendDeletionRecord(history []model.DeletedAccountRecord, rec model.DeletedAccountRecord, max int) []model.DeletedAccountRecord {
	next := append([]model.DeletedAccountRecord{rec}, history...)
	if max > 0 && len(next) > max {
		next = next[:max]
	}
	return next
}

func (s *Service) recordDeletion(profile model.Profile) error {
	state, err := s.store.LoadState()
	if err != nil {
		return err
	}
	rec := model.DeletedAccountRecord{
		ProfileID: profile.ID,
		Label:     profileLabel(profile),
		Email:     profile.Email,
		Provider:  chooseNonEmpty(profile.Provider, profile.Tool),
		DeletedAt: time.Now().UTC(),
	}
	state.DeletionHistory = appendDeletionRecord(state.DeletionHistory, rec, deletionHistoryMax)
	return s.store.SaveState(state)
}

func (s *Service) DeletionHistory() ([]model.DeletedAccountRecord, error) {
	state, err := s.store.LoadState()
	if err != nil {
		return nil, err
	}
	return state.DeletionHistory, nil
}
