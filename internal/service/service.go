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

type LogoutResult struct {
	Profile         model.Profile
	RemovedHomeAuth bool
	RemovedCache    bool
}

type ActivateResult struct {
	Profile model.Profile
	Home    string
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
	if strings.TrimSpace(label) == "" {
		profile = codex.ObservedProfileFromAuth(homePath, auth)
	}
	if profile.Label == "" {
		profile.Label = profile.ID
	}
	if err := s.store.UpsertProfile(profile); err != nil {
		return model.Profile{}, err
	}
	return profile, nil
}

func (s *Service) AddManagedCodexAccount(loginHomePath string) (model.Profile, error) {
	auth, err := codex.LoadProfileAuth(loginHomePath)
	if err != nil {
		return model.Profile{}, err
	}
	runtimeHome, err := s.defaultCodexHome()
	if err != nil {
		return model.Profile{}, err
	}
	profile := codex.ObservedProfileFromAuth(runtimeHome, auth)
	if profile.Label == "" {
		profile.Label = profile.ID
	}
	if err := s.store.UpsertProfile(profile); err != nil {
		return model.Profile{}, err
	}
	if err := codex.CacheHomeAuth(s.codexAuthCacheRoot(), profile.ID, loginHomePath); err != nil {
		return model.Profile{}, err
	}
	if err := s.removeManagedCodexHome(loginHomePath); err != nil {
		return model.Profile{}, err
	}
	return profile, nil
}

func (s *Service) RefreshAll() ([]model.ProfileStatus, error) {
	if err := s.normalizeManagedCodexProfiles(); err != nil {
		return nil, err
	}
	if err := s.normalizeLegacyProfileLabels(); err != nil {
		return nil, err
	}
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
	if err := s.normalizeManagedCodexProfiles(); err != nil {
		return nil, err
	}
	if err := s.normalizeLegacyProfileLabels(); err != nil {
		return nil, err
	}
	return s.store.ProfileStatuses()
}

func (s *Service) LogoutProfile(profileID string) (LogoutResult, error) {
	if err := s.normalizeManagedCodexProfiles(); err != nil {
		return LogoutResult{}, err
	}
	if err := s.normalizeLegacyProfileLabels(); err != nil {
		return LogoutResult{}, err
	}
	statuses, err := s.store.ProfileStatuses()
	if err != nil {
		return LogoutResult{}, err
	}

	var selected model.ProfileStatus
	found := false
	for _, status := range statuses {
		if status.Profile.ID == profileID {
			selected = status
			found = true
			break
		}
	}
	if !found {
		return LogoutResult{}, fmt.Errorf("profile %q not found", profileID)
	}

	result := LogoutResult{Profile: selected.Profile}
	if selected.Profile.Tool == model.ToolCodex && selected.State.AuthStatus == model.AuthStatusActive {
		auth, err := codex.LoadProfileAuth(selected.Profile.HomePath)
		if err == nil && profileMatchesAuth(selected.Profile, auth) {
			if err := codex.DeleteHomeAuth(selected.Profile.HomePath); err != nil {
				return result, err
			}
			result.RemovedHomeAuth = true
		}
	}

	if codex.HasCachedHomeAuth(s.codexAuthCacheRoot(), selected.Profile.ID) {
		if err := codex.DeleteCachedHomeAuth(s.codexAuthCacheRoot(), selected.Profile.ID); err != nil {
			return result, err
		}
		result.RemovedCache = true
	}
	if err := s.removeManagedCodexHome(selected.Profile.HomePath); err != nil {
		return result, err
	}

	if err := s.store.RemoveProfile(selected.Profile.ID); err != nil {
		return result, err
	}

	return result, nil
}

func (s *Service) ActivateProfile(profileID string) (ActivateResult, error) {
	if err := s.normalizeManagedCodexProfiles(); err != nil {
		return ActivateResult{}, err
	}
	if err := s.normalizeLegacyProfileLabels(); err != nil {
		return ActivateResult{}, err
	}
	statuses, err := s.store.ProfileStatuses()
	if err != nil {
		return ActivateResult{}, err
	}

	var selected model.ProfileStatus
	found := false
	for _, status := range statuses {
		if status.Profile.ID == profileID {
			selected = status
			found = true
			break
		}
	}
	if !found {
		return ActivateResult{}, fmt.Errorf("profile %q not found", profileID)
	}
	if selected.Profile.Tool != model.ToolCodex {
		return ActivateResult{}, fmt.Errorf("profile %q is not a Codex account", profileID)
	}
	if selected.State.AuthStatus == model.AuthStatusActive {
		return ActivateResult{Profile: selected.Profile, Home: selected.Profile.HomePath}, nil
	}
	if !codex.HasCachedHomeAuth(s.codexAuthCacheRoot(), selected.Profile.ID) {
		return ActivateResult{}, fmt.Errorf("account %q does not have cached credentials", profileLabel(selected.Profile))
	}
	if err := codex.RestoreCachedHomeAuth(s.codexAuthCacheRoot(), selected.Profile.ID, selected.Profile.HomePath); err != nil {
		return ActivateResult{}, err
	}
	return ActivateResult{
		Profile: selected.Profile,
		Home:    selected.Profile.HomePath,
	}, nil
}

func (s *Service) HasCachedAuth(profileID string) bool {
	return codex.HasCachedHomeAuth(s.codexAuthCacheRoot(), profileID)
}

func (s *Service) RefreshLoggedOutCachedUsage(statuses []model.ProfileStatus) ([]model.ProfileStatus, int, error) {
	currentState, err := s.store.LoadState()
	if err != nil {
		return nil, 0, err
	}

	refreshedCount := 0
	for _, status := range statuses {
		if status.Profile.Tool != model.ToolCodex {
			continue
		}
		if status.State.AuthStatus != model.AuthStatusLoggedOut {
			continue
		}
		if !codex.HasCachedHomeAuth(s.codexAuthCacheRoot(), status.Profile.ID) {
			continue
		}

		usage, auth, err := codex.FetchUsageFromCachedAuth(s.codexAuthCacheRoot(), status.Profile.ID)
		if err != nil {
			continue
		}

		now := time.Now().UTC()
		state := hydrateProfileState(currentState.Profiles[status.Profile.ID], status.Profile.ID, now)
		state.AuthStatus = model.AuthStatusLoggedOut
		state.LastSeenLoggedOutAt = &now
		state.Usage = usage
		state.LastError = ""
		if err := s.store.UpdateProfileState(status.Profile.ID, state); err != nil {
			return nil, refreshedCount, err
		}
		currentState.Profiles[status.Profile.ID] = state
		refreshedCount++

		if auth != nil {
			profile := status.Profile
			profile.Email = chooseNonEmpty(auth.Email, profile.Email)
			profile.AccountID = chooseNonEmpty(auth.AccountID, profile.AccountID)
			profile.Plan = chooseNonEmpty(auth.Plan, profile.Plan)
			profile.UpdatedAt = now
			if err := s.store.UpsertProfile(profile); err != nil {
				return nil, refreshedCount, err
			}
		}
	}

	updatedStatuses, err := s.store.ProfileStatuses()
	if err != nil {
		return nil, refreshedCount, err
	}
	return updatedStatuses, refreshedCount, nil
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

func (s *Service) ManagedCodexHomesRoot() string {
	return filepath.Join(s.store.Root(), "homes", "codex")
}

func (s *Service) removeManagedCodexHome(homePath string) error {
	managedRoot := s.ManagedCodexHomesRoot()
	managedParent := filepath.Dir(homePath)
	if !pathWithinRoot(managedParent, managedRoot) {
		return nil
	}
	if err := os.RemoveAll(managedParent); err != nil {
		return fmt.Errorf("remove managed Codex home: %w", err)
	}
	return nil
}

func (s *Service) normalizeManagedCodexProfiles() error {
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return err
	}
	runtimeHome, err := s.defaultCodexHome()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, profile := range cfg.Profiles {
		if profile.Tool != model.ToolCodex {
			continue
		}
		oldHomePath := profile.HomePath
		if !pathWithinRoot(oldHomePath, s.ManagedCodexHomesRoot()) {
			continue
		}
		authPath := filepath.Join(oldHomePath, "auth.json")
		if _, err := os.Stat(authPath); err == nil {
			_ = codex.CacheHomeAuth(s.codexAuthCacheRoot(), profile.ID, oldHomePath)
		}
		profile.HomePath = runtimeHome
		profile.UpdatedAt = now
		if err := s.store.UpsertProfile(profile); err != nil {
			return err
		}
		if err := s.removeManagedCodexHome(oldHomePath); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) normalizeLegacyProfileLabels() error {
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	changed := false
	for _, profile := range cfg.Profiles {
		if profile.Tool != model.ToolCodex {
			continue
		}
		if strings.TrimSpace(strings.ToLower(profile.Label)) != "default" {
			continue
		}
		nextLabel := normalizeLegacyProfileLabel(profile)
		if nextLabel == "" || nextLabel == "default" || nextLabel == profile.Label {
			continue
		}
		profile.Label = nextLabel
		profile.UpdatedAt = now
		if err := s.store.UpsertProfile(profile); err != nil {
			return err
		}
		changed = true
	}
	if changed {
		return nil
	}
	return nil
}

func (s *Service) defaultCodexHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, ".codex"), nil
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

func profileLabel(profile model.Profile) string {
	if strings.TrimSpace(profile.Label) != "" {
		return profile.Label
	}
	if strings.TrimSpace(profile.Email) != "" {
		return profile.Email
	}
	return profile.ID
}

func normalizeLegacyProfileLabel(profile model.Profile) string {
	for _, candidate := range []string{profile.Email, profile.AccountID, profile.ID} {
		if value := normalizeObservedLikeLabel(candidate); value != "" {
			return value
		}
	}
	return ""
}

func normalizeObservedLikeLabel(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"@", "-",
		".", "-",
		"_", "-",
		" ", "-",
	)
	value = replacer.Replace(value)
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		allowed := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if allowed {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	value = strings.Trim(builder.String(), "-")
	return value
}

func pathWithinRoot(path string, root string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != "" && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
