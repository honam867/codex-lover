package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codex-lover/internal/claude"
	"codex-lover/internal/codex"
	"codex-lover/internal/kimi"
	"codex-lover/internal/model"
	"codex-lover/internal/opencode"
	"codex-lover/internal/store"
)

type Service struct {
	store *store.Store
}

func (s *Service) LoadConfig() (model.Config, error) {
	return s.store.LoadConfig()
}

func (s *Service) SaveConfig(cfg model.Config) error {
	return s.store.SaveConfig(cfg)
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

type RefreshOptions struct {
	SkipUsageForTools map[string]bool
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

func (s *Service) ImportClaudeProfile(label string, homePath string) (model.Profile, error) {
	auth, err := claude.LoadProfileAuth(homePath)
	if err != nil {
		return model.Profile{}, err
	}
	profile := claude.ProfileFromAuth(label, homePath, auth)
	if strings.TrimSpace(label) == "" {
		profile = claude.ObservedProfileFromAuth(homePath, auth)
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

func (s *Service) AddManagedClaudeAccount(loginHomePath string) (model.Profile, error) {
	auth, err := claude.LoadProfileAuth(loginHomePath)
	if err != nil {
		return model.Profile{}, err
	}
	runtimeHome, err := s.defaultClaudeHome()
	if err != nil {
		return model.Profile{}, err
	}
	profile := claude.ObservedProfileFromAuth(runtimeHome, auth)
	if profile.Label == "" {
		profile.Label = profile.ID
	}
	if err := s.store.UpsertProfile(profile); err != nil {
		return model.Profile{}, err
	}
	if err := claude.CacheHomeAuth(s.claudeAuthCacheRoot(), profile.ID, loginHomePath); err != nil {
		return model.Profile{}, err
	}
	if err := s.removeManagedClaudeHome(loginHomePath); err != nil {
		return model.Profile{}, err
	}
	return profile, nil
}

func (s *Service) ImportKimiProfile(label string, homePath string) (model.Profile, error) {
	auth, err := kimi.LoadProfileAuth(homePath)
	if err != nil {
		return model.Profile{}, err
	}
	profile := kimi.ProfileFromAuth(label, homePath, auth)
	if strings.TrimSpace(label) == "" {
		profile = kimi.ObservedProfileFromAuth(homePath, auth)
	}
	if profile.Label == "" {
		profile.Label = profile.ID
	}
	if err := s.store.UpsertProfile(profile); err != nil {
		return model.Profile{}, err
	}
	return profile, nil
}

func (s *Service) AddManagedKimiAccount(loginHomePath string) (model.Profile, error) {
	auth, err := kimi.LoadProfileAuth(loginHomePath)
	if err != nil {
		return model.Profile{}, err
	}
	runtimeHome, err := s.defaultKimiHome()
	if err != nil {
		return model.Profile{}, err
	}
	profile := kimi.ObservedProfileFromAuth(runtimeHome, auth)
	if profile.Label == "" {
		profile.Label = profile.ID
	}
	if err := s.store.UpsertProfile(profile); err != nil {
		return model.Profile{}, err
	}
	if err := kimi.CacheHomeAuth(s.kimiAuthCacheRoot(), profile.ID, loginHomePath); err != nil {
		return model.Profile{}, err
	}
	if err := s.removeManagedKimiHome(loginHomePath); err != nil {
		return model.Profile{}, err
	}
	return profile, nil
}

func (s *Service) RefreshAll() ([]model.ProfileStatus, error) {
	return s.RefreshAllWithOptions(RefreshOptions{})
}

func (s *Service) RefreshAllWithOptions(opts RefreshOptions) ([]model.ProfileStatus, error) {
	if err := s.normalizeManagedCodexProfiles(); err != nil {
		return nil, err
	}
	if err := s.normalizeManagedClaudeProfiles(); err != nil {
		return nil, err
	}
	if err := s.normalizeManagedKimiProfiles(); err != nil {
		return nil, err
	}
	if err := s.ensureDefaultClaudeProfile(); err != nil {
		return nil, err
	}
	if err := s.ensureDefaultKimiProfile(); err != nil {
		return nil, err
	}
	if err := s.normalizeAnonymousClaudeProfiles(); err != nil {
		return nil, err
	}
	if err := s.normalizeDuplicateCodexProfiles(); err != nil {
		return nil, err
	}
	if err := s.normalizeLegacyProfileProviders(); err != nil {
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
	claudeProfilesByHome := map[string][]model.Profile{}
	kimiProfilesByHome := map[string][]model.Profile{}
	for _, profile := range cfg.Profiles {
		switch profile.Tool {
		case model.ToolCodex:
			profilesByHome[profile.HomePath] = append(profilesByHome[profile.HomePath], profile)
		case model.ToolClaude:
			claudeProfilesByHome[profile.HomePath] = append(claudeProfilesByHome[profile.HomePath], profile)
		case model.ToolKimi:
			kimiProfilesByHome[profile.HomePath] = append(kimiProfilesByHome[profile.HomePath], profile)
		}
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

		activeProfile, existed := findMatchingProfile(profiles, auth.AccountID, auth.Email)
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

		skipUsage := shouldSkipUsageForTool(opts, model.ToolCodex)
		var usage *model.UsageSnapshot
		var usageErr error
		if !skipUsage {
			usage, usageErr = codex.FetchUsage(auth)
		}

		for _, profile := range profiles {
			state := hydrateProfileState(currentState.Profiles[profile.ID], profile.ID, now)
			if profile.ID == activeProfile.ID {
				state.AuthStatus = model.AuthStatusActive
				state.AuthFingerprint = codex.AuthFingerprint(auth)
				authFingerprints[profile.ID] = state.AuthFingerprint
				state.LastSeenActiveAt = &now
				state.LastSeenLoggedOutAt = nil
				if skipUsage {
					state.LastError = ""
				} else if usageErr != nil {
					if shouldKeepCachedUsageWithoutError(state, usageErr) {
						state.LastError = ""
					} else {
						state.LastError = usageErr.Error()
					}
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
		if !skipUsage && usageErr == nil {
			_ = codex.CacheHomeAuth(s.codexAuthCacheRoot(), activeProfile.ID, activeProfile.HomePath)
		}
	}

	for homePath, profiles := range claudeProfilesByHome {
		now := time.Now().UTC()
		auth, err := claude.LoadProfileAuth(homePath)
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

		activeProfile, existed := findMatchingProfile(profiles, auth.AccountID, auth.Email)
		if !existed {
			activeProfile = claude.ObservedProfileFromAuth(homePath, auth)
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

		skipUsage := shouldSkipUsageForTool(opts, model.ToolClaude)
		var usage *model.UsageSnapshot
		var usageErr error
		if !skipUsage {
			usage, usageErr = claude.FetchUsage(auth)
		}

		for _, profile := range profiles {
			state := hydrateProfileState(currentState.Profiles[profile.ID], profile.ID, now)
			if profile.ID == activeProfile.ID {
				state.AuthStatus = model.AuthStatusActive
				state.AuthFingerprint = claude.AuthFingerprint(auth)
				authFingerprints[profile.ID] = state.AuthFingerprint
				state.LastSeenActiveAt = &now
				state.LastSeenLoggedOutAt = nil
				if skipUsage {
					state.LastError = ""
				} else if usageErr != nil {
					if shouldKeepCachedUsageWithoutError(state, usageErr) {
						state.LastError = ""
					} else {
						state.LastError = usageErr.Error()
					}
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
		if !skipUsage && usageErr == nil {
			_ = claude.CacheHomeAuth(s.claudeAuthCacheRoot(), activeProfile.ID, activeProfile.HomePath)
		}
	}

	for homePath, profiles := range kimiProfilesByHome {
		now := time.Now().UTC()
		auth, err := kimi.LoadProfileAuth(homePath)
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

		activeProfile, existed := findMatchingProfile(profiles, auth.AccountID, auth.Email)
		if !existed {
			activeProfile = kimi.ObservedProfileFromAuth(homePath, auth)
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

		skipUsage := shouldSkipUsageForTool(opts, model.ToolKimi)
		var usage *model.UsageSnapshot
		var usageErr error
		if !skipUsage {
			usage, usageErr = kimi.FetchUsage(auth)
		}

		for _, profile := range profiles {
			state := hydrateProfileState(currentState.Profiles[profile.ID], profile.ID, now)
			if profile.ID == activeProfile.ID {
				state.AuthStatus = model.AuthStatusActive
				state.AuthFingerprint = kimi.AuthFingerprint(auth)
				authFingerprints[profile.ID] = state.AuthFingerprint
				state.LastSeenActiveAt = &now
				state.LastSeenLoggedOutAt = nil
				if skipUsage {
					state.LastError = ""
				} else if usageErr != nil {
					if shouldKeepCachedUsageWithoutError(state, usageErr) {
						state.LastError = ""
					} else {
						state.LastError = usageErr.Error()
					}
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
		if !skipUsage && usageErr == nil {
			_ = kimi.CacheHomeAuth(s.kimiAuthCacheRoot(), activeProfile.ID, activeProfile.HomePath)
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

func shouldSkipUsageForTool(opts RefreshOptions, tool string) bool {
	if len(opts.SkipUsageForTools) == 0 {
		return false
	}
	return opts.SkipUsageForTools[strings.ToLower(strings.TrimSpace(tool))]
}

func (s *Service) ProfileStatuses() ([]model.ProfileStatus, error) {
	if err := s.normalizeManagedCodexProfiles(); err != nil {
		return nil, err
	}
	if err := s.normalizeManagedClaudeProfiles(); err != nil {
		return nil, err
	}
	if err := s.normalizeManagedKimiProfiles(); err != nil {
		return nil, err
	}
	if err := s.ensureDefaultClaudeProfile(); err != nil {
		return nil, err
	}
	if err := s.ensureDefaultKimiProfile(); err != nil {
		return nil, err
	}
	if err := s.normalizeAnonymousClaudeProfiles(); err != nil {
		return nil, err
	}
	if err := s.normalizeDuplicateCodexProfiles(); err != nil {
		return nil, err
	}
	if err := s.normalizeLegacyProfileProviders(); err != nil {
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
	if err := s.normalizeManagedClaudeProfiles(); err != nil {
		return LogoutResult{}, err
	}
	if err := s.normalizeManagedKimiProfiles(); err != nil {
		return LogoutResult{}, err
	}
	if err := s.ensureDefaultClaudeProfile(); err != nil {
		return LogoutResult{}, err
	}
	if err := s.ensureDefaultKimiProfile(); err != nil {
		return LogoutResult{}, err
	}
	if err := s.normalizeAnonymousClaudeProfiles(); err != nil {
		return LogoutResult{}, err
	}
	if err := s.normalizeDuplicateCodexProfiles(); err != nil {
		return LogoutResult{}, err
	}
	if err := s.normalizeLegacyProfileProviders(); err != nil {
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
	if selected.State.AuthStatus == model.AuthStatusActive {
		switch selected.Profile.Tool {
		case model.ToolCodex:
			auth, err := codex.LoadProfileAuth(selected.Profile.HomePath)
			if err == nil && profileMatchesAuth(selected.Profile, auth.AccountID, auth.Email) {
				if err := codex.DeleteHomeAuth(selected.Profile.HomePath); err != nil {
					return result, err
				}
				result.RemovedHomeAuth = true
			}
		case model.ToolClaude:
			auth, err := claude.LoadProfileAuth(selected.Profile.HomePath)
			if err == nil && profileMatchesAuth(selected.Profile, auth.AccountID, auth.Email) {
				if err := claude.DeleteHomeAuth(selected.Profile.HomePath); err != nil {
					return result, err
				}
				result.RemovedHomeAuth = true
			}
		case model.ToolKimi:
			auth, err := kimi.LoadProfileAuth(selected.Profile.HomePath)
			if err == nil && profileMatchesAuth(selected.Profile, auth.AccountID, auth.Email) {
				if err := kimi.DeleteHomeAuth(selected.Profile.HomePath); err != nil {
					return result, err
				}
				result.RemovedHomeAuth = true
			}
		}
	}

	if s.hasCachedAuthForProfile(selected.Profile.ID, selected.Profile.Tool) {
		if err := s.deleteCachedAuthByTool(selected.Profile.Tool, selected.Profile.ID); err != nil {
			return result, err
		}
		result.RemovedCache = true
	}
	if err := s.removeManagedCodexHome(selected.Profile.HomePath); err != nil {
		return result, err
	}
	if err := s.removeManagedClaudeHome(selected.Profile.HomePath); err != nil {
		return result, err
	}
	if err := s.removeManagedKimiHome(selected.Profile.HomePath); err != nil {
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
	if err := s.normalizeManagedClaudeProfiles(); err != nil {
		return ActivateResult{}, err
	}
	if err := s.normalizeManagedKimiProfiles(); err != nil {
		return ActivateResult{}, err
	}
	if err := s.ensureDefaultClaudeProfile(); err != nil {
		return ActivateResult{}, err
	}
	if err := s.ensureDefaultKimiProfile(); err != nil {
		return ActivateResult{}, err
	}
	if err := s.normalizeAnonymousClaudeProfiles(); err != nil {
		return ActivateResult{}, err
	}
	if err := s.normalizeDuplicateCodexProfiles(); err != nil {
		return ActivateResult{}, err
	}
	if err := s.normalizeLegacyProfileProviders(); err != nil {
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
	if selected.State.AuthStatus == model.AuthStatusActive {
		return ActivateResult{Profile: selected.Profile, Home: selected.Profile.HomePath}, nil
	}
	sourceProfileID, ok := s.cachedAuthSourceProfileID(selected.Profile, statusesToProfiles(statuses))
	if !ok {
		return ActivateResult{}, fmt.Errorf("account %q does not have cached credentials", profileLabel(selected.Profile))
	}
	if err := s.restoreCachedAuthByTool(selected.Profile.Tool, sourceProfileID, selected.Profile.HomePath); err != nil {
		return ActivateResult{}, err
	}
	if sourceProfileID != selected.Profile.ID && !s.hasCachedAuthForProfile(selected.Profile.ID, selected.Profile.Tool) {
		if err := s.moveCachedAuthByTool(selected.Profile.Tool, sourceProfileID, selected.Profile.ID); err != nil {
			return ActivateResult{}, err
		}
	}
	return ActivateResult{
		Profile: selected.Profile,
		Home:    selected.Profile.HomePath,
	}, nil
}

func (s *Service) HasCachedAuth(profileID string) bool {
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return s.hasCachedAuthForProfile(profileID, "")
	}
	var selected model.Profile
	found := false
	for _, profile := range cfg.Profiles {
		if profile.ID == profileID {
			selected = profile
			found = true
			break
		}
	}
	if !found {
		return s.hasCachedAuthForProfile(profileID, "")
	}
	_, ok := s.cachedAuthSourceProfileID(selected, cfg.Profiles)
	return ok
}

func (s *Service) RefreshLoggedOutCachedUsage(statuses []model.ProfileStatus) ([]model.ProfileStatus, int, error) {
	currentState, err := s.store.LoadState()
	if err != nil {
		return nil, 0, err
	}
	profiles := statusesToProfiles(statuses)

	refreshedCount := 0
	for _, status := range statuses {
		if status.State.AuthStatus != model.AuthStatusLoggedOut {
			continue
		}
		sourceProfileID, ok := s.cachedAuthSourceProfileID(status.Profile, profiles)
		if !ok {
			continue
		}

		usage, auth, err := s.fetchUsageFromCachedAuthByTool(status.Profile.Tool, sourceProfileID)
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
			profile.Email = chooseNonEmpty(profileAuthEmail(auth), profile.Email)
			profile.AccountID = chooseNonEmpty(profileAuthAccountID(auth), profile.AccountID)
			profile.Plan = chooseNonEmpty(profileAuthPlan(auth), profile.Plan)
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

func (s *Service) cachedAuthSourceProfileID(profile model.Profile, profiles []model.Profile) (string, bool) {
	if s.hasCachedAuthForProfile(profile.ID, profile.Tool) {
		return profile.ID, true
	}
	for _, candidate := range profiles {
		if candidate.ID == profile.ID {
			continue
		}
		if !profilesRepresentSameAccount(profile, candidate) {
			continue
		}
		if s.hasCachedAuthForProfile(candidate.ID, candidate.Tool) {
			return candidate.ID, true
		}
	}
	return "", false
}

func statusesToProfiles(statuses []model.ProfileStatus) []model.Profile {
	profiles := make([]model.Profile, 0, len(statuses))
	for _, status := range statuses {
		profiles = append(profiles, status.Profile)
	}
	return profiles
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

func (s *Service) AutoRotateCodex(statuses []model.ProfileStatus) (SwitchResult, error) {
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return SwitchResult{}, err
	}
	if !cfg.AutoRotateCodex {
		return SwitchResult{Checked: true, Reason: "auto-rotate disabled"}, nil
	}
	active, ok := activeCodexStatus(statuses)
	if !ok {
		return SwitchResult{Checked: true, Reason: "no active Codex account"}, nil
	}
	var candidates []model.ProfileStatus
	for _, status := range statuses {
		if status.Profile.Tool != model.ToolCodex {
			continue
		}
		if !codex.HasCachedHomeAuth(s.codexAuthCacheRoot(), status.Profile.ID) {
			continue
		}
		if weeklyRemaining(status) <= 0.5 {
			continue
		}
		candidates = append(candidates, status)
	}
	if len(candidates) == 0 {
		return SwitchResult{Checked: true, From: active.Profile, Reason: "no cached ready account"}, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		return fiveHourRemaining(candidates[i]) > fiveHourRemaining(candidates[j])
	})
	top := candidates[0]
	if top.Profile.ID == active.Profile.ID {
		return SwitchResult{Checked: true, From: active.Profile, Reason: "active account is already best"}, nil
	}
	diff := fiveHourRemaining(top) - fiveHourRemaining(active)
	if diff <= cfg.AutoRotateThreshold {
		return SwitchResult{Checked: true, From: active.Profile, Reason: fmt.Sprintf("top account diff %.2f%% <= threshold %.2f%%", diff, cfg.AutoRotateThreshold)}, nil
	}
	if err := codex.RestoreCachedHomeAuth(s.codexAuthCacheRoot(), top.Profile.ID, active.Profile.HomePath); err != nil {
		return SwitchResult{Checked: true, From: active.Profile, To: top.Profile}, err
	}
	return SwitchResult{
		Checked: true,
		Changed: true,
		From:    active.Profile,
		To:      top.Profile,
		Reason:  fmt.Sprintf("auto-rotate: top account has %.2f%% more 5h remaining", diff),
	}, nil
}

func fiveHourRemaining(status model.ProfileStatus) float64 {
	if status.State.Usage == nil {
		return 0
	}
	if status.State.Usage.Secondary != nil {
		return status.State.Usage.Secondary.RemainingPercent
	}
	if status.State.Usage.Primary != nil {
		return status.State.Usage.Primary.RemainingPercent
	}
	return 0
}

func weeklyRemaining(status model.ProfileStatus) float64 {
	if status.State.Usage == nil {
		return 0
	}
	if status.State.Usage.Primary != nil {
		return status.State.Usage.Primary.RemainingPercent
	}
	if status.State.Usage.Secondary != nil {
		return status.State.Usage.Secondary.RemainingPercent
	}
	return 0
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

func (s *Service) claudeAuthCacheRoot() string {
	return filepath.Join(s.store.Root(), "claude-auth")
}

func (s *Service) kimiAuthCacheRoot() string {
	return filepath.Join(s.store.Root(), "kimi-auth")
}

func (s *Service) ManagedCodexHomesRoot() string {
	return filepath.Join(s.store.Root(), "homes", "codex")
}

func (s *Service) ManagedClaudeHomesRoot() string {
	return filepath.Join(s.store.Root(), "homes", "claude")
}

func (s *Service) ManagedKimiHomesRoot() string {
	return filepath.Join(s.store.Root(), "homes", "kimi")
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

func (s *Service) removeManagedClaudeHome(homePath string) error {
	managedRoot := s.ManagedClaudeHomesRoot()
	if !pathWithinRoot(homePath, managedRoot) {
		return nil
	}
	if err := os.RemoveAll(homePath); err != nil {
		return fmt.Errorf("remove managed Claude home: %w", err)
	}
	return nil
}

func (s *Service) removeManagedKimiHome(homePath string) error {
	managedRoot := s.ManagedKimiHomesRoot()
	if !pathWithinRoot(homePath, managedRoot) {
		return nil
	}
	if err := os.RemoveAll(homePath); err != nil {
		return fmt.Errorf("remove managed Kimi home: %w", err)
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

func (s *Service) normalizeManagedClaudeProfiles() error {
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return err
	}
	runtimeHome, err := s.defaultClaudeHome()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, profile := range cfg.Profiles {
		if profile.Tool != model.ToolClaude {
			continue
		}
		oldHomePath := profile.HomePath
		if !pathWithinRoot(oldHomePath, s.ManagedClaudeHomesRoot()) {
			continue
		}
		authPath := filepath.Join(oldHomePath, ".credentials.json")
		if _, err := os.Stat(authPath); err == nil {
			_ = claude.CacheHomeAuth(s.claudeAuthCacheRoot(), profile.ID, oldHomePath)
		}
		profile.HomePath = runtimeHome
		profile.UpdatedAt = now
		if err := s.store.UpsertProfile(profile); err != nil {
			return err
		}
		if err := s.removeManagedClaudeHome(oldHomePath); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) normalizeManagedKimiProfiles() error {
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return err
	}
	runtimeHome, err := s.defaultKimiHome()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, profile := range cfg.Profiles {
		if profile.Tool != model.ToolKimi {
			continue
		}
		oldHomePath := profile.HomePath
		if !pathWithinRoot(oldHomePath, s.ManagedKimiHomesRoot()) {
			continue
		}
		authPath := filepath.Join(oldHomePath, "credentials", "kimi-code.json")
		if _, err := os.Stat(authPath); err == nil {
			_ = kimi.CacheHomeAuth(s.kimiAuthCacheRoot(), profile.ID, oldHomePath)
		}
		profile.HomePath = runtimeHome
		profile.UpdatedAt = now
		if err := s.store.UpsertProfile(profile); err != nil {
			return err
		}
		if err := s.removeManagedKimiHome(oldHomePath); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) normalizeAnonymousClaudeProfiles() error {
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return err
	}
	state, err := s.store.LoadState()
	if err != nil {
		return err
	}

	profilesByHome := map[string][]model.Profile{}
	for _, profile := range cfg.Profiles {
		if profile.Tool != model.ToolClaude {
			continue
		}
		profilesByHome[profile.HomePath] = append(profilesByHome[profile.HomePath], profile)
	}

	changed := false
	removed := map[string]bool{}
	now := time.Now().UTC()

	for _, profiles := range profilesByHome {
		var canonical *model.Profile
		for i := range profiles {
			if strings.TrimSpace(profiles[i].Email) != "" || strings.TrimSpace(profiles[i].AccountID) != "" {
				candidate := profiles[i]
				canonical = &candidate
				break
			}
		}
		if canonical == nil {
			continue
		}

		for _, profile := range profiles {
			if profile.ID == canonical.ID {
				continue
			}
			if strings.TrimSpace(profile.Email) != "" || strings.TrimSpace(profile.AccountID) != "" {
				continue
			}
			if s.hasCachedAuthForProfile(profile.ID, profile.Tool) {
				if !s.hasCachedAuthForProfile(canonical.ID, canonical.Tool) {
					if err := s.moveCachedAuthByTool(profile.Tool, profile.ID, canonical.ID); err != nil {
						return err
					}
				} else {
					if err := s.deleteCachedAuthByTool(profile.Tool, profile.ID); err != nil {
						return err
					}
				}
			}

			canonicalProfile := mergeCanonicalProfile(*canonical, profile, now)
			*canonical = canonicalProfile
			state.Profiles[canonical.ID] = mergeProfileStates(state.Profiles[canonical.ID], state.Profiles[profile.ID])
			delete(state.Profiles, profile.ID)
			removed[profile.ID] = true
			changed = true
		}
		if changed {
			if err := s.store.UpsertProfile(*canonical); err != nil {
				return err
			}
		}
	}

	if !changed {
		return nil
	}

	nextProfiles := make([]model.Profile, 0, len(cfg.Profiles))
	for _, profile := range cfg.Profiles {
		if removed[profile.ID] {
			continue
		}
		nextProfiles = append(nextProfiles, profile)
	}
	cfg.Profiles = nextProfiles
	if err := s.store.SaveConfig(cfg); err != nil {
		return err
	}
	if err := s.store.SaveState(state); err != nil {
		return err
	}
	return nil
}

func (s *Service) ensureDefaultClaudeProfile() error {
	homePath, err := s.defaultClaudeHome()
	if err != nil {
		return err
	}
	auth, err := claude.LoadProfileAuth(homePath)
	if err != nil {
		return nil
	}
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return err
	}
	for _, profile := range cfg.Profiles {
		if profile.Tool != model.ToolClaude {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(profile.HomePath), strings.TrimSpace(homePath)) {
			continue
		}
		if strings.TrimSpace(profile.Email) != "" || strings.TrimSpace(profile.AccountID) != "" {
			continue
		}
		profile.Label = chooseNonEmpty(claude.ObservedProfileFromAuth(homePath, auth).Label, profile.Label)
		profile.Email = chooseNonEmpty(auth.Email, profile.Email)
		profile.AccountID = chooseNonEmpty(auth.AccountID, profile.AccountID)
		profile.Plan = chooseNonEmpty(auth.Plan, profile.Plan)
		profile.Provider = chooseNonEmpty(model.ToolClaude, profile.Provider)
		profile.UpdatedAt = time.Now().UTC()
		return s.store.UpsertProfile(profile)
	}

	for _, profile := range cfg.Profiles {
		if profile.Tool != model.ToolClaude {
			continue
		}
		if profileMatchesAuth(profile, auth.AccountID, auth.Email) {
			return nil
		}
	}

	return s.store.UpsertProfile(claude.ObservedProfileFromAuth(homePath, auth))
}

func (s *Service) ensureDefaultKimiProfile() error {
	homePath, err := s.defaultKimiHome()
	if err != nil {
		return err
	}
	auth, err := kimi.LoadProfileAuth(homePath)
	if err != nil {
		return nil
	}
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return err
	}
	for _, profile := range cfg.Profiles {
		if profile.Tool != model.ToolKimi {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(profile.HomePath), strings.TrimSpace(homePath)) {
			continue
		}
		if strings.TrimSpace(profile.Email) != "" || strings.TrimSpace(profile.AccountID) != "" {
			continue
		}
		profile.Label = chooseNonEmpty(kimi.ObservedProfileFromAuth(homePath, auth).Label, profile.Label)
		profile.Email = chooseNonEmpty(auth.Email, profile.Email)
		profile.AccountID = chooseNonEmpty(auth.AccountID, profile.AccountID)
		profile.Plan = chooseNonEmpty(auth.Plan, profile.Plan)
		profile.Provider = chooseNonEmpty(model.ToolKimi, profile.Provider)
		profile.UpdatedAt = time.Now().UTC()
		return s.store.UpsertProfile(profile)
	}

	for _, profile := range cfg.Profiles {
		if profile.Tool != model.ToolKimi {
			continue
		}
		if profileMatchesAuth(profile, auth.AccountID, auth.Email) {
			return nil
		}
	}

	return s.store.UpsertProfile(kimi.ObservedProfileFromAuth(homePath, auth))
}

func (s *Service) normalizeDuplicateCodexProfiles() error {
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return err
	}
	state, err := s.store.LoadState()
	if err != nil {
		return err
	}

	type profileGroupKey struct {
		Tool     string
		Provider string
		HomePath string
		Identity string
	}

	groups := map[profileGroupKey][]model.Profile{}
	for _, profile := range cfg.Profiles {
		identity := profileIdentityKey(profile)
		if identity == "" {
			continue
		}
		key := profileGroupKey{
			Tool:     strings.ToLower(strings.TrimSpace(profile.Tool)),
			Provider: strings.ToLower(strings.TrimSpace(profile.Provider)),
			HomePath: strings.ToLower(strings.TrimSpace(profile.HomePath)),
			Identity: identity,
		}
		groups[key] = append(groups[key], profile)
	}

	profilesByID := make(map[string]model.Profile, len(cfg.Profiles))
	for _, profile := range cfg.Profiles {
		profilesByID[profile.ID] = profile
	}

	removed := map[string]bool{}
	changed := false
	now := time.Now().UTC()

	for _, group := range groups {
		if len(group) < 2 {
			continue
		}

		canonical := selectCanonicalProfile(group, state.Profiles, s)
		canonicalState := state.Profiles[canonical.ID]

		for _, duplicate := range group {
			if duplicate.ID == canonical.ID || removed[duplicate.ID] {
				continue
			}

			if s.hasCachedAuthForProfile(duplicate.ID, duplicate.Tool) {
				if !s.hasCachedAuthForProfile(canonical.ID, canonical.Tool) {
					if err := s.moveCachedAuthByTool(duplicate.Tool, duplicate.ID, canonical.ID); err != nil {
						return err
					}
				} else {
					if err := s.deleteCachedAuthByTool(duplicate.Tool, duplicate.ID); err != nil {
						return err
					}
				}
			}

			canonical = mergeCanonicalProfile(canonical, duplicate, now)
			canonicalState = mergeProfileStates(canonicalState, state.Profiles[duplicate.ID])
			delete(state.Profiles, duplicate.ID)
			delete(profilesByID, duplicate.ID)
			removed[duplicate.ID] = true
			changed = true
		}

		profilesByID[canonical.ID] = canonical
		state.Profiles[canonical.ID] = canonicalState
	}

	if !changed {
		return nil
	}

	nextProfiles := make([]model.Profile, 0, len(cfg.Profiles))
	for _, profile := range cfg.Profiles {
		if removed[profile.ID] {
			continue
		}
		if replacement, ok := profilesByID[profile.ID]; ok {
			nextProfiles = append(nextProfiles, replacement)
			continue
		}
		nextProfiles = append(nextProfiles, profile)
	}
	cfg.Profiles = nextProfiles

	if err := s.store.SaveConfig(cfg); err != nil {
		return err
	}
	if err := s.store.SaveState(state); err != nil {
		return err
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

func (s *Service) normalizeLegacyProfileProviders() error {
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	changed := false
	for _, profile := range cfg.Profiles {
		nextProvider := strings.TrimSpace(profile.Provider)
		if nextProvider == "" {
			nextProvider = strings.TrimSpace(profile.Tool)
		}
		if nextProvider == "" {
			nextProvider = model.ToolCodex
		}
		if profile.Provider == nextProvider {
			continue
		}
		profile.Provider = nextProvider
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

func (s *Service) defaultClaudeHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

func (s *Service) defaultKimiHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, ".kimi"), nil
}

func (s *Service) hasCachedAuthForProfile(profileID string, tool string) bool {
	switch tool {
	case model.ToolCodex:
		return codex.HasCachedHomeAuth(s.codexAuthCacheRoot(), profileID)
	case model.ToolClaude:
		return claude.HasCachedHomeAuth(s.claudeAuthCacheRoot(), profileID)
	case model.ToolKimi:
		return kimi.HasCachedHomeAuth(s.kimiAuthCacheRoot(), profileID)
	default:
		return codex.HasCachedHomeAuth(s.codexAuthCacheRoot(), profileID) || claude.HasCachedHomeAuth(s.claudeAuthCacheRoot(), profileID) || kimi.HasCachedHomeAuth(s.kimiAuthCacheRoot(), profileID)
	}
}

func (s *Service) restoreCachedAuthByTool(tool string, profileID string, homePath string) error {
	switch tool {
	case model.ToolCodex:
		return codex.RestoreCachedHomeAuth(s.codexAuthCacheRoot(), profileID, homePath)
	case model.ToolClaude:
		return claude.RestoreCachedHomeAuth(s.claudeAuthCacheRoot(), profileID, homePath)
	case model.ToolKimi:
		return kimi.RestoreCachedHomeAuth(s.kimiAuthCacheRoot(), profileID, homePath)
	default:
		return fmt.Errorf("tool %q does not support cached auth restore", tool)
	}
}

func (s *Service) moveCachedAuthByTool(tool string, sourceProfileID string, targetProfileID string) error {
	switch tool {
	case model.ToolCodex:
		return codex.MoveCachedHomeAuth(s.codexAuthCacheRoot(), sourceProfileID, targetProfileID)
	case model.ToolClaude:
		return claude.MoveCachedHomeAuth(s.claudeAuthCacheRoot(), sourceProfileID, targetProfileID)
	case model.ToolKimi:
		return kimi.MoveCachedHomeAuth(s.kimiAuthCacheRoot(), sourceProfileID, targetProfileID)
	default:
		return fmt.Errorf("tool %q does not support cached auth move", tool)
	}
}

func (s *Service) deleteCachedAuthByTool(tool string, profileID string) error {
	switch tool {
	case model.ToolCodex:
		return codex.DeleteCachedHomeAuth(s.codexAuthCacheRoot(), profileID)
	case model.ToolClaude:
		return claude.DeleteCachedHomeAuth(s.claudeAuthCacheRoot(), profileID)
	case model.ToolKimi:
		return kimi.DeleteCachedHomeAuth(s.kimiAuthCacheRoot(), profileID)
	default:
		return fmt.Errorf("tool %q does not support cached auth deletion", tool)
	}
}

func (s *Service) fetchUsageFromCachedAuthByTool(tool string, profileID string) (*model.UsageSnapshot, any, error) {
	switch tool {
	case model.ToolCodex:
		usage, auth, err := codex.FetchUsageFromCachedAuth(s.codexAuthCacheRoot(), profileID)
		return usage, auth, err
	case model.ToolClaude:
		usage, auth, err := claude.FetchUsageFromCachedAuth(s.claudeAuthCacheRoot(), profileID)
		return usage, auth, err
	case model.ToolKimi:
		usage, auth, err := kimi.FetchUsageFromCachedAuth(s.kimiAuthCacheRoot(), profileID)
		return usage, auth, err
	default:
		return nil, nil, fmt.Errorf("tool %q does not support cached usage refresh", tool)
	}
}

func profileAuthEmail(value any) string {
	switch auth := value.(type) {
	case *codex.ProfileAuth:
		return auth.Email
	case *claude.ProfileAuth:
		return auth.Email
	case *kimi.ProfileAuth:
		return auth.Email
	default:
		return ""
	}
}

func profileAuthAccountID(value any) string {
	switch auth := value.(type) {
	case *codex.ProfileAuth:
		return auth.AccountID
	case *claude.ProfileAuth:
		return auth.AccountID
	case *kimi.ProfileAuth:
		return auth.AccountID
	default:
		return ""
	}
}

func profileAuthPlan(value any) string {
	switch auth := value.(type) {
	case *codex.ProfileAuth:
		return auth.Plan
	case *claude.ProfileAuth:
		return auth.Plan
	case *kimi.ProfileAuth:
		return auth.Plan
	default:
		return ""
	}
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

func shouldKeepCachedUsageWithoutError(state model.ProfileState, err error) bool {
	if err == nil || state.Usage == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") || strings.Contains(msg, "rate_limit")
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
	return strings.Contains(msg, "does not contain chatgpt tokens") ||
		strings.Contains(msg, "does not contain claude oauth tokens") ||
		strings.Contains(msg, "logged out")
}

func findMatchingProfile(profiles []model.Profile, accountID string, email string) (model.Profile, bool) {
	for _, profile := range profiles {
		if profileMatchesAuth(profile, accountID, email) {
			return profile, true
		}
	}
	return model.Profile{}, false
}

func profileMatchesAuth(profile model.Profile, accountID string, email string) bool {
	if accountID != "" && profile.AccountID != "" && strings.EqualFold(accountID, profile.AccountID) {
		return true
	}
	if email != "" && profile.Email != "" && strings.EqualFold(email, profile.Email) {
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
	if shouldDisplayEmail(profile) {
		return profile.Email
	}
	if strings.TrimSpace(profile.Label) != "" {
		return profile.Label
	}
	if strings.TrimSpace(profile.Email) != "" {
		return profile.Email
	}
	return profile.ID
}

func shouldDisplayEmail(profile model.Profile) bool {
	if strings.TrimSpace(profile.Email) == "" {
		return false
	}
	label := strings.TrimSpace(profile.Label)
	if label == "" {
		return true
	}
	return normalizeObservedLikeLabel(profile.Email) == strings.ToLower(label)
}

func normalizeLegacyProfileLabel(profile model.Profile) string {
	for _, candidate := range []string{profile.Email, profile.AccountID, profile.ID} {
		if value := normalizeObservedLikeLabel(candidate); value != "" {
			return value
		}
	}
	return ""
}

func profileIdentityKey(profile model.Profile) string {
	if accountID := strings.ToLower(strings.TrimSpace(profile.AccountID)); accountID != "" {
		return "account:" + accountID
	}
	if email := strings.ToLower(strings.TrimSpace(profile.Email)); email != "" {
		return "email:" + email
	}
	return ""
}

func profilesRepresentSameAccount(left model.Profile, right model.Profile) bool {
	if strings.TrimSpace(left.Tool) != strings.TrimSpace(right.Tool) {
		return false
	}
	if strings.TrimSpace(left.Provider) != strings.TrimSpace(right.Provider) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(left.HomePath), strings.TrimSpace(right.HomePath)) {
		return false
	}
	leftIdentity := profileIdentityKey(left)
	rightIdentity := profileIdentityKey(right)
	return leftIdentity != "" && leftIdentity == rightIdentity
}

func selectCanonicalProfile(profiles []model.Profile, states map[string]model.ProfileState, svc *Service) model.Profile {
	best := profiles[0]
	bestScore := canonicalProfileScore(best, states[best.ID], svc)
	for _, profile := range profiles[1:] {
		score := canonicalProfileScore(profile, states[profile.ID], svc)
		if score > bestScore {
			best = profile
			bestScore = score
		}
	}
	return best
}

func canonicalProfileScore(profile model.Profile, state model.ProfileState, svc *Service) int {
	score := 0
	if svc.hasCachedAuthForProfile(profile.ID, profile.Tool) {
		score += 100
	}
	if state.AuthStatus == model.AuthStatusActive {
		score += 50
	}
	if profile.AutoDiscovered {
		score += 20
	}
	if strings.TrimSpace(profile.AccountID) != "" {
		score += 10
	}
	if strings.TrimSpace(strings.ToLower(profile.Label)) == "default" {
		score -= 10
	}
	return score
}

func mergeCanonicalProfile(canonical model.Profile, duplicate model.Profile, now time.Time) model.Profile {
	canonical.Email = chooseNonEmpty(canonical.Email, duplicate.Email)
	canonical.AccountID = chooseNonEmpty(canonical.AccountID, duplicate.AccountID)
	canonical.Plan = chooseNonEmpty(canonical.Plan, duplicate.Plan)
	canonical.Provider = chooseNonEmpty(canonical.Provider, duplicate.Provider)
	canonical.Tool = chooseNonEmpty(canonical.Tool, duplicate.Tool)
	canonical.HomePath = chooseNonEmpty(canonical.HomePath, duplicate.HomePath)
	canonical.Enabled = canonical.Enabled || duplicate.Enabled
	canonical.AutoDiscovered = canonical.AutoDiscovered || duplicate.AutoDiscovered

	if strings.TrimSpace(strings.ToLower(canonical.Label)) == "default" {
		if nextLabel := normalizeLegacyProfileLabel(duplicate); nextLabel != "" {
			canonical.Label = nextLabel
		}
	}
	if canonical.CreatedAt.IsZero() || (!duplicate.CreatedAt.IsZero() && duplicate.CreatedAt.Before(canonical.CreatedAt)) {
		canonical.CreatedAt = duplicate.CreatedAt
	}
	canonical.UpdatedAt = now
	return canonical
}

func mergeProfileStates(canonical model.ProfileState, duplicate model.ProfileState) model.ProfileState {
	base := canonical
	if preferredProfileState(duplicate, canonical) {
		base = duplicate
	}

	base.ProfileID = chooseNonEmpty(base.ProfileID, canonical.ProfileID)
	base.LastRefreshedAt = laterTimePtr(canonical.LastRefreshedAt, duplicate.LastRefreshedAt)
	base.LastSeenActiveAt = laterTimePtr(canonical.LastSeenActiveAt, duplicate.LastSeenActiveAt)
	base.LastSeenLoggedOutAt = laterTimePtr(canonical.LastSeenLoggedOutAt, duplicate.LastSeenLoggedOutAt)
	if base.LastError == "" {
		if canonical.LastError != "" {
			base.LastError = canonical.LastError
		} else {
			base.LastError = duplicate.LastError
		}
	}
	if base.AuthStatus == "" {
		if canonical.AuthStatus != "" {
			base.AuthStatus = canonical.AuthStatus
		} else {
			base.AuthStatus = duplicate.AuthStatus
		}
	}
	if base.Usage == nil {
		if canonical.Usage != nil {
			base.Usage = canonical.Usage
		} else {
			base.Usage = duplicate.Usage
		}
	}
	return base
}

func preferredProfileState(left model.ProfileState, right model.ProfileState) bool {
	if stateAuthStatusRank(left.AuthStatus) != stateAuthStatusRank(right.AuthStatus) {
		return stateAuthStatusRank(left.AuthStatus) < stateAuthStatusRank(right.AuthStatus)
	}
	return laterTimePtr(left.LastRefreshedAt, right.LastRefreshedAt) == left.LastRefreshedAt && left.LastRefreshedAt != nil
}

func laterTimePtr(left *time.Time, right *time.Time) *time.Time {
	switch {
	case left == nil:
		return right
	case right == nil:
		return left
	case left.After(*right):
		return left
	default:
		return right
	}
}

func stateAuthStatusRank(value string) int {
	switch value {
	case model.AuthStatusActive:
		return 0
	case model.AuthStatusLoggedOut:
		return 1
	case model.AuthStatusError:
		return 2
	default:
		return 3
	}
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
