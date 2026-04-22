package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"codex-lover/internal/model"
	"codex-lover/internal/service"
)

const loggedOutRefreshInterval = 15 * time.Minute

type Server struct {
	address string
	svc     *service.Service
}

type profileActionRequest struct {
	ProfileID string `json:"profile_id"`
}

type runtimeRefreshResult struct {
	statuses     []model.ProfileStatus
	syncStatus   string
	switchResult service.SwitchResult
}

func New(address string, svc *service.Service) *Server {
	return &Server{
		address: address,
		svc:     svc,
	}
}

func (s *Server) Run(ctx context.Context, pollInterval time.Duration) error {
	if pollInterval <= 0 {
		pollInterval = 15 * time.Second
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", s.handleStatus)
	mux.HandleFunc("/v1/refresh", s.handleRefresh)
	mux.HandleFunc("/v1/accounts", s.handleAccounts)
	mux.HandleFunc("/v1/accounts/switch", s.handleSwitch)
	mux.HandleFunc("/v1/accounts/remove", s.handleRemove)

	server := &http.Server{
		Addr:    s.address,
		Handler: mux,
	}

	refreshTicker := time.NewTicker(pollInterval)
	defer refreshTicker.Stop()

	backgroundUsageTicker := time.NewTicker(loggedOutRefreshInterval)
	defer backgroundUsageTicker.Stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	result, err := s.refreshRuntime(true)
	if err != nil {
		s.logError("startup", err)
	} else {
		s.logSnapshot("startup", result.statuses, result.syncStatus)
		s.logAutoSwitch("startup", result.switchResult)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-refreshTicker.C:
				result, err := s.refreshRuntime(false)
				if err != nil {
					s.logError("poll", err)
					continue
				}
				s.logSnapshot("poll", result.statuses, result.syncStatus)
				s.logAutoSwitch("poll", result.switchResult)
			case <-backgroundUsageTicker.C:
				statuses, refreshedCount, err := s.refreshLoggedOutUsage()
				if err != nil {
					s.logError("cached", err)
					continue
				}
				s.logCachedRefresh(statuses, refreshedCount)
			}
		}
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("daemon listen: %w", err)
	}
	return nil
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	statuses, err := s.svc.ProfileStatuses()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	statuses = codexStatuses(statuses)
	writeJSON(w, http.StatusOK, statuses)
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := s.refreshRuntime(true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.logSnapshot("manual-refresh", result.statuses, result.syncStatus)
	s.logAutoSwitch("manual-refresh", result.switchResult)
	statuses := codexStatuses(result.statuses)
	writeJSON(w, http.StatusOK, statuses)
}

func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	statuses, err := s.svc.ProfileStatuses()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	statuses = codexStatuses(statuses)
	writeJSON(w, http.StatusOK, statuses)
}

func (s *Server) handleSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, err := decodeProfileActionRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.svc.ActivateProfile(req.ProfileID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	result, err := s.refreshRuntime(true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.logSnapshot("switch", result.statuses, result.syncStatus)
	s.logAutoSwitch("switch", result.switchResult)
	statuses := codexStatuses(result.statuses)
	writeJSON(w, http.StatusOK, statuses)
}

func (s *Server) handleRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, err := decodeProfileActionRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.svc.LogoutProfile(req.ProfileID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	result, err := s.refreshRuntime(true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.logSnapshot("remove", result.statuses, result.syncStatus)
	s.logAutoSwitch("remove", result.switchResult)
	statuses := codexStatuses(result.statuses)
	writeJSON(w, http.StatusOK, statuses)
}

func (s *Server) refreshRuntime(warmLoggedOut bool) (runtimeRefreshResult, error) {
	statuses, err := s.svc.RefreshAll()
	if err != nil {
		return runtimeRefreshResult{}, err
	}
	if s.shouldWarmLoggedOutUsage(statuses, warmLoggedOut) {
		statuses, _, err = s.svc.RefreshLoggedOutCachedUsage(statuses)
		if err != nil {
			return runtimeRefreshResult{}, err
		}
	}

	switchResult, err := s.svc.AutoSwitchLimitedCodex(statuses)
	if err != nil {
		return runtimeRefreshResult{}, err
	}
	if switchResult.Changed {
		statuses, err = s.svc.RefreshAll()
		if err != nil {
			return runtimeRefreshResult{}, err
		}
		statuses, _, err = s.svc.RefreshLoggedOutCachedUsage(statuses)
		if err != nil {
			return runtimeRefreshResult{}, err
		}
	}

	if !hasActiveCodex(statuses) {
		return runtimeRefreshResult{statuses: statuses, syncStatus: "idle", switchResult: switchResult}, nil
	}
	result, err := s.svc.SyncOpenCodeFromActiveCodex(statuses)
	if err != nil {
		return runtimeRefreshResult{statuses: statuses, syncStatus: "error", switchResult: switchResult}, nil
	}
	if result.Changed {
		return runtimeRefreshResult{statuses: statuses, syncStatus: "updated", switchResult: switchResult}, nil
	}
	return runtimeRefreshResult{statuses: statuses, syncStatus: "unchanged", switchResult: switchResult}, nil
}

func (s *Server) refreshLoggedOutUsage() ([]model.ProfileStatus, int, error) {
	statuses, err := s.svc.ProfileStatuses()
	if err != nil {
		return nil, 0, err
	}
	return s.svc.RefreshLoggedOutCachedUsage(statuses)
}

func (s *Server) logSnapshot(source string, statuses []model.ProfileStatus, syncStatus string) {
	activeLabel, usageSummary := activeCodexSummary(statuses)
	fmt.Printf(
		"[%s] source=%s active=%s window=%s accounts=%d opencode=%s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		source,
		activeLabel,
		usageSummary,
		countCodexProfiles(statuses),
		syncStatus,
	)
}

func (s *Server) logCachedRefresh(statuses []model.ProfileStatus, refreshedCount int) {
	activeLabel, usageSummary := activeCodexSummary(statuses)
	fmt.Printf(
		"[%s] source=cached active=%s window=%s refreshed_cached=%d accounts=%d\n",
		time.Now().Format("2006-01-02 15:04:05"),
		activeLabel,
		usageSummary,
		refreshedCount,
		countCodexProfiles(statuses),
	)
}

func (s *Server) logAutoSwitch(source string, result service.SwitchResult) {
	if !result.Changed {
		return
	}
	fmt.Printf(
		"[%s] source=%s autoswitch=changed from=%s to=%s reason=%s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		source,
		profileLabel(result.From),
		profileLabel(result.To),
		strings.TrimSpace(result.Reason),
	)
}

func (s *Server) shouldWarmLoggedOutUsage(statuses []model.ProfileStatus, requested bool) bool {
	if requested {
		return true
	}
	active, ok := currentActiveCodexStatus(statuses)
	if !ok || !serviceStatusLimitReached(active) {
		return false
	}
	for _, status := range statuses {
		if status.Profile.Tool != model.ToolCodex {
			continue
		}
		if status.Profile.ID == active.Profile.ID || status.State.AuthStatus != model.AuthStatusLoggedOut {
			continue
		}
		if s.svc.HasCachedAuth(status.Profile.ID) {
			return true
		}
	}
	return false
}

func currentActiveCodexStatus(statuses []model.ProfileStatus) (model.ProfileStatus, bool) {
	for _, status := range statuses {
		if status.Profile.Tool == model.ToolCodex && status.State.AuthStatus == model.AuthStatusActive {
			return status, true
		}
	}
	return model.ProfileStatus{}, false
}

func serviceStatusLimitReached(status model.ProfileStatus) bool {
	return windowLimitReached(status.State.UsageWindowPrimary()) || windowLimitReached(status.State.UsageWindowSecondary())
}

func windowLimitReached(window *model.UsageWindow) bool {
	return window != nil && window.RemainingPercent <= 0.5
}

func (s *Server) logError(source string, err error) {
	fmt.Printf(
		"[%s] source=%s refresh=error err=%s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		source,
		strings.TrimSpace(err.Error()),
	)
}

func decodeProfileActionRequest(r *http.Request) (profileActionRequest, error) {
	defer r.Body.Close()
	var req profileActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return profileActionRequest{}, fmt.Errorf("decode request: %w", err)
	}
	req.ProfileID = strings.TrimSpace(req.ProfileID)
	if req.ProfileID == "" {
		return profileActionRequest{}, errors.New("missing profile_id")
	}
	return req, nil
}

func hasActiveCodex(statuses []model.ProfileStatus) bool {
	for _, status := range statuses {
		if status.Profile.Tool == model.ToolCodex && status.State.AuthStatus == model.AuthStatusActive {
			return true
		}
	}
	return false
}

func countCodexProfiles(statuses []model.ProfileStatus) int {
	count := 0
	for _, status := range statuses {
		if status.Profile.Tool == model.ToolCodex {
			count++
		}
	}
	return count
}

func codexStatuses(statuses []model.ProfileStatus) []model.ProfileStatus {
	filtered := make([]model.ProfileStatus, 0, len(statuses))
	for _, status := range statuses {
		if status.Profile.Tool == model.ToolCodex {
			filtered = append(filtered, status)
		}
	}
	return filtered
}

func activeCodexSummary(statuses []model.ProfileStatus) (string, string) {
	now := time.Now()
	for _, status := range statuses {
		if status.Profile.Tool != model.ToolCodex || status.State.AuthStatus != model.AuthStatusActive {
			continue
		}
		window := service.EffectiveWindowForDisplay(status.State.UsageWindowPrimary(), status.State.AuthStatus, now)
		if window == nil {
			return profileLabel(status.Profile), "unavailable"
		}
		remaining := int(window.RemainingPercent + 0.5)
		if remaining < 0 {
			remaining = 0
		}
		if remaining > 100 {
			remaining = 100
		}
		return profileLabel(status.Profile), fmt.Sprintf("%s %s", progressBar(remaining, 12), service.FormatWindowSummary(window))
	}
	return "none", "unavailable"
}

func profileLabel(profile model.Profile) string {
	if strings.TrimSpace(profile.Email) != "" {
		return profile.Email
	}
	if strings.TrimSpace(profile.Label) != "" {
		return profile.Label
	}
	return profile.ID
}

func progressBar(percent int, width int) string {
	if width <= 0 {
		width = 10
	}
	filled := (percent*width + 50) / 100
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + fmt.Sprintf("] %d%%", percent)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
