package service

import (
	"fmt"
	"strings"
	"time"

	"codex-lover/internal/model"
)

// applyProfileMeta returns a copy of profile with updated local metadata. A zero
// createdAt keeps the existing date. Other fields are preserved.
func applyProfileMeta(profile model.Profile, createdAt time.Time, price int64, audience string, shopName string, customerName string, note string, now time.Time) model.Profile {
	if !createdAt.IsZero() {
		profile.CreatedAt = createdAt
	}
	profile.Price = price
	profile.Audience = normalizeProfileAudience(audience)
	profile.ShopName = strings.TrimSpace(shopName)
	profile.CustomerName = strings.TrimSpace(customerName)
	profile.Note = strings.TrimSpace(note)
	profile.UpdatedAt = now
	return profile
}

// UpdateProfileMeta edits a profile's local metadata and persists it.
func (s *Service) UpdateProfileMeta(profileID string, createdAt time.Time, price int64, audience string, shopName string, customerName string, note string) (model.Profile, error) {
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
		updated := applyProfileMeta(profile, createdAt, price, audience, shopName, customerName, note, time.Now().UTC())
		if err := s.store.UpsertProfile(updated); err != nil {
			return model.Profile{}, err
		}
		return updated, nil
	}
	return model.Profile{}, fmt.Errorf("profile %q not found", profileID)
}

func normalizeProfileAudience(audience string) string {
	switch strings.ToLower(strings.TrimSpace(audience)) {
	case model.ProfileAudienceCustomer:
		return model.ProfileAudienceCustomer
	default:
		return model.ProfileAudiencePersonal
	}
}
