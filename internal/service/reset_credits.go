package service

import (
	"crypto/rand"
	"fmt"
	"strings"

	"codex-lover/internal/codex"
	"codex-lover/internal/model"
)

// resetAuthTarget says which auth file a reset-credit call uses: the runtime
// home auth for the active account (so token rotation stays in sync with Codex),
// otherwise the cached auth of the profile or a sibling representing it.
type resetAuthTarget struct {
	ProfileID       string
	HomePath        string
	SourceProfileID string
	AccountID       string
	Email           string
}

type CodexResetCredits struct {
	List         *codex.ResetCreditList
	History      *codex.ResetCreditHistory
	HistoryError string
}

func resolveResetAuthTarget(
	statuses []model.ProfileStatus,
	profileID string,
	cachedSource func(model.Profile) (string, bool),
) (resetAuthTarget, error) {
	for _, status := range statuses {
		if status.Profile.ID != profileID {
			continue
		}
		if status.Profile.Tool != model.ToolCodex {
			return resetAuthTarget{}, fmt.Errorf("reset credits are only available for Codex accounts")
		}
		if status.State.AuthStatus == model.AuthStatusActive && status.Profile.HomePath != "" {
			return resetAuthTarget{
				ProfileID: profileID,
				HomePath:  status.Profile.HomePath,
				AccountID: status.Profile.AccountID,
				Email:     status.Profile.Email,
			}, nil
		}
		sourceProfileID, ok := cachedSource(status.Profile)
		if !ok {
			return resetAuthTarget{}, fmt.Errorf("account has no cached auth; log in to it once first")
		}
		return resetAuthTarget{ProfileID: profileID, SourceProfileID: sourceProfileID}, nil
	}
	return resetAuthTarget{}, fmt.Errorf("Codex profile %q not found", profileID)
}

func (s *Service) loadResetAuth(statuses []model.ProfileStatus, profileID string) (resetAuthTarget, *codex.ProfileAuth, error) {
	target, err := resolveResetAuthTarget(statuses, profileID, s.cachedSourceFunc(statusesToProfiles(statuses)))
	if err != nil {
		return target, nil, err
	}
	var auth *codex.ProfileAuth
	if target.HomePath != "" {
		auth, err = codex.LoadProfileAuth(target.HomePath)
	} else {
		auth, err = codex.LoadCachedProfileAuth(s.codexAuthCacheRoot(), target.SourceProfileID)
	}
	if err != nil {
		return target, nil, fmt.Errorf("load Codex auth: %w", err)
	}
	if target.HomePath != "" && !homeAuthMatchesTarget(auth, target) {
		// The runtime auth was switched after the last refresh; never spend or
		// persist tokens for a different account than the one the user picked.
		return target, nil, fmt.Errorf("active Codex account changed; refresh and try again")
	}
	return target, auth, nil
}

func homeAuthMatchesTarget(auth *codex.ProfileAuth, target resetAuthTarget) bool {
	if auth.AccountID != "" && target.AccountID != "" && !strings.EqualFold(auth.AccountID, target.AccountID) {
		return false
	}
	if auth.Email != "" && target.Email != "" && !strings.EqualFold(auth.Email, target.Email) {
		return false
	}
	return true
}

// persistResetAuth writes rotated tokens back to the file they came from. For
// the active account it also refreshes the profile's cache copy.
func (s *Service) persistResetAuth(target resetAuthTarget, refreshed *codex.AuthFile) error {
	if refreshed == nil {
		return nil
	}
	if target.HomePath == "" {
		return codex.PersistRefreshedAuthAtPath(codex.CachedAuthPath(s.codexAuthCacheRoot(), target.SourceProfileID), refreshed)
	}
	if err := codex.PersistRefreshedAuthAtPath(codex.HomeAuthPath(target.HomePath), refreshed); err != nil {
		return err
	}
	return codex.CacheHomeAuth(s.codexAuthCacheRoot(), target.ProfileID, target.HomePath)
}

// ListCodexResetCredits fetches available credits and the redeem history. A
// history failure is reported but does not fail the call.
func (s *Service) ListCodexResetCredits(statuses []model.ProfileStatus, profileID string) (CodexResetCredits, error) {
	target, auth, err := s.loadResetAuth(statuses, profileID)
	if err != nil {
		return CodexResetCredits{}, err
	}

	list, refreshed, err := codex.ListResetCredits(auth)
	if perr := s.persistResetAuth(target, refreshed); perr != nil && err == nil {
		err = perr
	}
	if err != nil {
		return CodexResetCredits{}, err
	}

	result := CodexResetCredits{List: list}
	history, refreshed, herr := codex.ListResetCreditHistory(auth)
	if perr := s.persistResetAuth(target, refreshed); perr != nil && herr == nil {
		herr = perr
	}
	if herr != nil {
		result.HistoryError = herr.Error()
	} else {
		result.History = history
	}
	return result, nil
}

func (s *Service) ConsumeCodexResetCredit(statuses []model.ProfileStatus, profileID string, creditID string) (*codex.ConsumeResetResult, error) {
	target, auth, err := s.loadResetAuth(statuses, profileID)
	if err != nil {
		return nil, err
	}
	result, refreshed, err := codex.ConsumeResetCredit(auth, newRedeemRequestID(), creditID)
	if perr := s.persistResetAuth(target, refreshed); perr != nil && err == nil {
		// The credit was consumed; only token persistence failed. Report the result.
		return result, fmt.Errorf("reset applied but saving refreshed token failed: %w", perr)
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SaveCodexResetCreditCount stores a freshly listed credit count so the card
// matches the detail view without waiting for the next usage refresh.
func (s *Service) SaveCodexResetCreditCount(profileID string, count int) error {
	state, err := s.store.LoadState()
	if err != nil {
		return err
	}
	updated, changed := applyResetCreditCount(state, profileID, count)
	if !changed {
		return nil
	}
	return s.store.SaveState(updated)
}

func applyResetCreditCount(state model.State, profileID string, count int) (model.State, bool) {
	current, ok := state.Profiles[profileID]
	if !ok || current.Usage == nil {
		return state, false
	}
	if existing := current.Usage.ResetCreditsAvailable; existing != nil && *existing == count {
		return state, false
	}
	usage := *current.Usage
	usage.ResetCreditsAvailable = &count
	current.Usage = &usage

	profiles := make(map[string]model.ProfileState, len(state.Profiles))
	for id, profileState := range state.Profiles {
		profiles[id] = profileState
	}
	profiles[profileID] = current
	state.Profiles = profiles
	return state, true
}

func newRedeemRequestID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}
