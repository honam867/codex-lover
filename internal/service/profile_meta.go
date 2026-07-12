package service

import (
	"fmt"
	"time"

	"codex-lover/internal/model"
)

// applyProfileMeta returns a copy of profile with an updated added-date and
// price. A zero createdAt keeps the existing date. Other fields are preserved.
func applyProfileMeta(profile model.Profile, createdAt time.Time, price int64, now time.Time) model.Profile {
	if !createdAt.IsZero() {
		profile.CreatedAt = createdAt
	}
	profile.Price = price
	profile.UpdatedAt = now
	return profile
}

// UpdateProfileMeta edits a profile's added-date and price (VNĐ) and persists it.
func (s *Service) UpdateProfileMeta(profileID string, createdAt time.Time, price int64) (model.Profile, error) {
	if price < 0 {
		return model.Profile{}, fmt.Errorf("price must be >= 0")
	}
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return model.Profile{}, err
	}
	for _, profile := range cfg.Profiles {
		if profile.ID != profileID {
			continue
		}
		updated := applyProfileMeta(profile, createdAt, price, time.Now().UTC())
		if err := s.store.UpsertProfile(updated); err != nil {
			return model.Profile{}, err
		}
		return updated, nil
	}
	return model.Profile{}, fmt.Errorf("profile %q not found", profileID)
}
