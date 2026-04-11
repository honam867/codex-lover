package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codex-lover/internal/codex"
	"codex-lover/internal/model"
	"codex-lover/internal/opencode"
	"codex-lover/internal/store"
)

type Service struct {
	store *store.Store
}

type SwitchResult struct {
	Checked bool
	Changed bool
	Reason  string
	From    model.Profile
	To      model.Profile
}

func New(store *store.Store) *Service {
	return &Service{store: store}
}

func (s *Service) ImportCodexProfile(label string, homePath string) (model.Profile, error) {
	auth, err := codex.LoadProfileAuth(homePath)
	if err != nil {
		return model.Profile{}, err
	}
	profile := codex.ProfileFromAuth(label, homePath, auth)
	if profile.Label == "" {
		profile.Label = profile.ID
	}
	if err := s.store.UpsertProfile(profile); err != nil {
		return model.Profile{}, err
	}
	return profile, nil
}

func (s *Service) RefreshAll() ([]model.ProfileStatus, error) {
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return nil, err
	}

	currentState, err := s.store.LoadState()
	if err != nil {
		return nil, err
	}

	profilesByHome := map[string][]model.Profile{}
	for _, profile := range cfg.Profiles {
		if profile.Tool != model.ToolCodex {
			continue
		}
		profilesByHome[profile.HomePath] = append(profilesByHome[profile.HomePath], profile)
	}
	authFingerprints := map[string]string{}

	for homePath, profiles := range profilesByHome {
		now := time.Now().UTC()
		auth, err := codex.LoadProfileAuth(homePath)
		if err != nil {
			authStatus := authStatusFromLoadError(err)
			for _, profile := range profiles {
				state := hydrateProfileState(currentState.Profiles[profile.ID], profile.ID, now)
				state.AuthStatus = authStatus
				if authStatus == model.AuthStatusLoggedOut {
					state.LastSeenLoggedOutAt = &now
					state.LastError = ""
				} else {
					state.LastError = err.Error()
				}
				if err := s.store.UpdateProfileState(profile.ID, state); err != nil {
					return nil, err
				}
				currentState.Profiles[profile.ID] = state
			}
			continue
		}

		activeProfile, existed := findMatchingProfile(profiles, auth)
		if !existed {
			activeProfile = codex.ObservedProfileFromAuth(homePath, auth)
		} else {
			activeProfile.Email = chooseNonEmpty(auth.Email, activeProfile.Email)
			activeProfile.AccountID = chooseNonEmpty(auth.AccountID, activeProfile.AccountID)
			activeProfile.Plan = chooseNonEmpty(auth.Plan, activeProfile.Plan)
			activeProfile.UpdatedAt = now
		}
		if err := s.store.UpsertProfile(activeProfile); err != nil {
			return nil, err
		}
		if !containsProfile(profiles, activeProfile.ID) {
			profiles = append(profiles, activeProfile)
		}

		usage, usageErr := codex.FetchUsage(auth)

		for _, profile := range profiles {
			state := hydrateProfileState(currentState.Profiles[profile.ID], profile.ID, now)
			if profile.ID == activeProfile.ID {
				state.AuthStatus = model.AuthStatusActive
				state.AuthFingerprint = codex.AuthFingerprint(auth)
				authFingerprints[profile.ID] = state.AuthFingerprint
				state.LastSeenActiveAt = &now
				state.LastSeenLoggedOutAt = nil
				if usageErr != nil {
					state.LastError = usageErr.Error()
				} else {
					state.Usage = usage
					state.LastError = ""
				}
			} else {
				state.AuthStatus = model.AuthStatusLoggedOut
				state.LastSeenLoggedOutAt = &now
				state.LastError = ""
			}
			if err := s.store.UpdateProfileState(profile.ID, state); err != nil {
				return nil, err
			}
			currentState.Profiles[profile.ID] = state
		}
		if usageErr == nil {
			_ = codex.CacheHomeAuth(s.codexAuthCacheRoot(), activeProfile.ID, activeProfile.HomePath)
		}
	}
	statuses, err := s.store.ProfileStatuses()
	if err != nil {
		return nil, err
	}
	for i := range statuses {
		if fingerprint := authFingerprints[statuses[i].Profile.ID]; fingerprint != "" {
			statuses[i].State.AuthFingerprint = fingerprint
		}
	}
	return statuses, nil
}

func (s *Service) ProfileStatuses() ([]model.ProfileStatus, error) {
	return s.store.ProfileStatuses()
}

func (s *Service) SyncOpenCodeFromActiveCodex(statuses []model.ProfileStatus) (opencode.SyncResult, error) {
	for _, status := range statuses {
		if status.Profile.Tool != model.ToolCodex || status.State.AuthStatus != model.AuthStatusActive {
			continue
		}
		auth, err := codex.LoadProfileAuth(status.Profile.HomePath)
		if err != nil {
			return opencode.SyncResult{}, err
		}
		return opencode.SyncOpenAIFromCodex(auth)
	}
	return opencode.SyncResult{}, fmt.Errorf("no active Codex account to sync")
}

func (s *Service) AutoSwitchLimitedCodex(statuses []model.ProfileStatus) (SwitchResult, error) {
	active, ok := activeCodexStatus(statuses)
	if !ok {
		return SwitchResult{Checked: true, Reason: "no active Codex account"}, nil
	}
	if !usageLimitReached(active) {
		return SwitchResult{Checked: true, From: active.Profile, Reason: "active account still has quota"}, nil
	}

	candidate, ok := s.bestSwitchCandidate(statuses, active)
	if !ok {
		return SwitchResult{Checked: true, From: active.Profile, Reason: "no cached ready account"}, nil
	}
	if err := codex.RestoreCachedHomeAuth(s.codexAuthCacheRoot(), candidate.Profile.ID, active.Profile.HomePath); err != nil {
		return SwitchResult{Checked: true, From: active.Profile, To: candidate.Profile}, err
	}
	return SwitchResult{
		Checked: true,
		Changed: true,
		From:    active.Profile,
		To:      candidate.Profile,
		Reason:  "active account reached limit",
	}, nil
}

func (s *Service) bestSwitchCandidate(statuses []model.ProfileStatus, active model.ProfileStatus) (model.ProfileStatus, bool) {
	now := time.Now()
	var best model.ProfileStatus
	var bestScore float64
	found := false
	for _, status := range statuses {
		if status.Profile.Tool != model.ToolCodex || status.Profile.ID == active.Profile.ID {
			continue
		}
		if status.Profile.HomePath != active.Profile.HomePath {
			continue
		}
		if !codex.HasCachedHomeAuth(s.codexAuthCacheRoot(), status.Profile.ID) {
			continue
		}
		score, ok := quotaScore(status, now)
		if !ok {
			continue
		}
		if !found || score > bestScore {
			best = status
			bestScore = score
			found = true
		}
	}
	return best, found
}

func (s *Service) codexAuthCacheRoot() string {
	return filepath.Join(s.store.Root(), "codex-auth")
}

func activeCodexStatus(statuses []model.ProfileStatus) (model.ProfileStatus, bool) {
	for _, status := range statuses {
		if status.Profile.Tool == model.ToolCodex && status.State.AuthStatus == model.AuthStatusActive {
			return status, true
		}
	}
	return model.ProfileStatus{}, false
}

func usageLimitReached(status model.ProfileStatus) bool {
	if status.State.Usage == nil {
		return false
	}
	return windowLimitReached(status.State.Usage.Primary) || windowLimitReached(status.State.Usage.Secondary)
}

func windowLimitReached(window *model.UsageWindow) bool {
	return window != nil && window.RemainingPercent <= 0.5
}

func quotaScore(status model.ProfileStatus, now time.Time) (float64, bool) {
	if status.State.Usage == nil {
		return 0, false
	}
	primary := EffectiveWindowForDisplay(status.State.Usage.Primary, status.State.AuthStatus, now)
	secondary := EffectiveWindowForDisplay(status.State.Usage.Secondary, status.State.AuthStatus, now)
	if primary == nil && secondary == nil {
		return 0, false
	}
	score := 100.0
	if primary != nil {
		if primary.RemainingPercent <= 0.5 {
			return 0, false
		}
		score = minFloat(score, primary.RemainingPercent)
	}
	if secondary != nil {
		if secondary.RemainingPercent <= 0.5 {
			return 0, false
		}
		score = minFloat(score, secondary.RemainingPercent)
	}
	return score, true
}

func minFloat(left float64, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

func FormatWindow(window *model.UsageWindow) string {
	return FormatWindowSummary(window)
}

func FormatWindowSummary(window *model.UsageWindow) string {
	if window == nil {
		return "-"
	}
	label := fmt.Sprintf("%.0f%% left", window.RemainingPercent)
	if window.ResetsAt != nil {
		label += "  resets " + window.ResetsAt.Local().Format("2006-01-02 15:04")
	}
	return label
}

func EffectiveWindowForDisplay(window *model.UsageWindow, authStatus string, now time.Time) *model.UsageWindow {
	if window == nil {
		return nil
	}
	if !WindowResetInferred(window, authStatus, now) {
		return window
	}
	copy := *window
	copy.UsedPercent = 0
	copy.RemainingPercent = 100
	return &copy
}

func WindowResetInferred(window *model.UsageWindow, authStatus string, now time.Time) bool {
	return authStatus == model.AuthStatusLoggedOut &&
		window != nil &&
		window.ResetsAt != nil &&
		!now.Before(*window.ResetsAt)
}

func FormatCredits(credits *model.CreditsSnapshot) string {
	if credits == nil {
		return "-"
	}
	if credits.Unlimited {
		return "unlimited"
	}
	if credits.Balance != "" {
		return credits.Balance
	}
	if credits.HasCredits {
		return "tracked"
	}
	return "-"
}

func hydrateProfileState(existing model.ProfileState, profileID string, now time.Time) model.ProfileState {
	state := existing
	state.ProfileID = profileID
	state.LastRefreshedAt = &now
	if state.AuthStatus == "" {
		state.AuthStatus = model.AuthStatusUnknown
	}
	return state
}

func authStatusFromLoadError(err error) string {
	if isLoggedOutAuthError(err) {
		return model.AuthStatusLoggedOut
	}
	return model.AuthStatusError
}

func isLoggedOutAuthError(err error) bool {
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not contain chatgpt tokens")
}

func findMatchingProfile(profiles []model.Profile, auth *codex.ProfileAuth) (model.Profile, bool) {
	for _, profile := range profiles {
		if profileMatchesAuth(profile, auth) {
			return profile, true
		}
	}
	return model.Profile{}, false
}

func profileMatchesAuth(profile model.Profile, auth *codex.ProfileAuth) bool {
	if auth == nil {
		return false
	}
	if auth.AccountID != "" && profile.AccountID != "" && strings.EqualFold(auth.AccountID, profile.AccountID) {
		return true
	}
	if auth.Email != "" && profile.Email != "" && strings.EqualFold(auth.Email, profile.Email) {
		return true
	}
	return false
}

func containsProfile(profiles []model.Profile, profileID string) bool {
	for _, profile := range profiles {
		if profile.ID == profileID {
			return true
		}
	}
	return false
}

func chooseNonEmpty(primary string, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}
