package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"codex-lover/internal/model"
	"codex-lover/internal/service"
	"codex-lover/internal/store"
	"golang.org/x/sys/windows"
)

type Snapshot struct {
	GeneratedAt string        `json:"generatedAt"`
	Profiles    []ProfileCard `json:"profiles"`
}

type ProfileCard struct {
	ID                  string `json:"id"`
	Label               string `json:"label"`
	Email               string `json:"email"`
	Plan                string `json:"plan"`
	AuthStatus          string `json:"authStatus"`
	Freshness           string `json:"freshness"`
	IsActive            bool   `json:"isActive"`
	PrimaryPercent      int    `json:"primaryPercent"`
	PrimarySummary      string `json:"primarySummary"`
	SecondaryPercent    int    `json:"secondaryPercent"`
	SecondarySummary    string `json:"secondarySummary"`
	Credits             string `json:"credits"`
	LastError           string `json:"lastError"`
	CanLoginFromCache   bool   `json:"canLoginFromCache"`
	LastRefreshedAtText string `json:"lastRefreshedAtText"`
}

type ActionResponse struct {
	Message  string   `json:"message"`
	Error    string   `json:"error,omitempty"`
	Snapshot Snapshot `json:"snapshot"`
}

type App struct {
	ctx          context.Context
	svc          *service.Service
	initErr      error
	mu           sync.Mutex
	lastSnapshot Snapshot
	thresholds   *desktopWatchNotifications
	openCodeSync desktopOpenCodeSyncCache
}

func NewApp() *App {
	st, err := store.New()
	if err != nil {
		return &App{initErr: err}
	}
	if err := st.Ensure(); err != nil {
		return &App{initErr: err}
	}
	return &App{
		svc:        service.New(st),
		thresholds: newDesktopWatchNotifications(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go a.runBackgroundLoops()
}

func (a *App) GetInitialSnapshot() Snapshot {
	snapshot, err := a.snapshot(true)
	if err != nil {
		return Snapshot{
			GeneratedAt: time.Now().Format("2006-01-02 15:04:05"),
			Profiles:    []ProfileCard{},
		}
	}
	return snapshot
}

func (a *App) GetSnapshot() Snapshot {
	snapshot, err := a.snapshot(false)
	if err != nil {
		return a.mustSnapshotFallback()
	}
	return snapshot
}

func (a *App) RefreshSnapshot() ActionResponse {
	snapshot, err := a.snapshot(true)
	if err != nil {
		return ActionResponse{
			Message:  "Refresh failed",
			Error:    err.Error(),
			Snapshot: a.mustSnapshotFallback(),
		}
	}
	return ActionResponse{
		Message:  "Refreshed",
		Snapshot: snapshot,
	}
}

func (a *App) ActivateProfile(profileID string) ActionResponse {
	if err := a.ensureReady(); err != nil {
		return ActionResponse{
			Message:  "Activate failed",
			Error:    err.Error(),
			Snapshot: a.mustSnapshotFallback(),
		}
	}
	result, err := a.svc.ActivateProfile(profileID)
	if err != nil {
		return ActionResponse{
			Message:  "Activate failed",
			Error:    err.Error(),
			Snapshot: a.mustSnapshotFallback(),
		}
	}
	snapshot, err := a.snapshot(true)
	if err != nil {
		return ActionResponse{
			Message:  "Logged into " + profileLabel(result.Profile),
			Error:    err.Error(),
			Snapshot: a.mustSnapshotFallback(),
		}
	}
	return ActionResponse{
		Message:  "Logged into " + profileLabel(result.Profile),
		Snapshot: snapshot,
	}
}

func (a *App) LogoutProfile(profileID string) ActionResponse {
	if err := a.ensureReady(); err != nil {
		return ActionResponse{
			Message:  "Delete failed",
			Error:    err.Error(),
			Snapshot: a.mustSnapshotFallback(),
		}
	}
	result, err := a.svc.LogoutProfile(profileID)
	if err != nil {
		return ActionResponse{
			Message:  "Delete failed",
			Error:    err.Error(),
			Snapshot: a.mustSnapshotFallback(),
		}
	}
	snapshot, err := a.snapshot(true)
	if err != nil {
		return ActionResponse{
			Message:  "Deleted " + profileLabel(result.Profile),
			Error:    err.Error(),
			Snapshot: a.mustSnapshotFallback(),
		}
	}
	return ActionResponse{
		Message:  "Deleted " + profileLabel(result.Profile),
		Snapshot: snapshot,
	}
}

func (a *App) AddAccount() ActionResponse {
	message, err := launchAddAccountConsole()
	if err != nil {
		return ActionResponse{
			Message:  "Add account failed",
			Error:    err.Error(),
			Snapshot: a.mustSnapshotFallback(),
		}
	}
	return ActionResponse{
		Message:  message,
		Snapshot: a.mustSnapshotFallback(),
	}
}

func (a *App) ensureReady() error {
	if a.initErr != nil {
		return a.initErr
	}
	if a.svc == nil {
		return fmt.Errorf("service is not available")
	}
	return nil
}

func (a *App) snapshot(refresh bool) (Snapshot, error) {
	if err := a.ensureReady(); err != nil {
		return Snapshot{}, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if refresh {
		return a.refreshLocked(true)
	}
	return a.currentSnapshotLocked()
}

func (a *App) mustSnapshotFallback() Snapshot {
	snapshot, err := a.snapshot(false)
	if err == nil {
		return snapshot
	}
	return Snapshot{
		GeneratedAt: time.Now().Format("2006-01-02 15:04:05"),
		Profiles:    []ProfileCard{},
	}
}

func buildSnapshot(statuses []model.ProfileStatus, svc *service.Service) Snapshot {
	now := time.Now()
	profiles := make([]ProfileCard, 0, len(statuses))
	for _, status := range statuses {
		primary := service.EffectiveWindowForDisplay(status.State.UsageWindowPrimary(), status.State.AuthStatus, now)
		secondary := service.EffectiveWindowForDisplay(status.State.UsageWindowSecondary(), status.State.AuthStatus, now)
		canLoginFromCache := false
		if svc != nil && status.Profile.Tool == model.ToolCodex && status.State.AuthStatus != model.AuthStatusActive {
			canLoginFromCache = svc.HasCachedAuth(status.Profile.ID)
		}
		profiles = append(profiles, ProfileCard{
			ID:                  status.Profile.ID,
			Label:               profileLabel(status.Profile),
			Email:               nonEmpty(status.Profile.Email, "-"),
			Plan:                nonEmpty(status.Profile.Plan, "-"),
			AuthStatus:          nonEmpty(status.State.AuthStatus, model.AuthStatusUnknown),
			Freshness:           freshnessLabel(status),
			IsActive:            status.State.AuthStatus == model.AuthStatusActive,
			PrimaryPercent:      progressPercent(primary),
			PrimarySummary:      service.FormatWindowSummary(primary),
			SecondaryPercent:    progressPercent(secondary),
			SecondarySummary:    service.FormatWindowSummary(secondary),
			Credits:             service.FormatCredits(usageCredits(status)),
			LastError:           nonEmpty(status.State.LastError, "-"),
			CanLoginFromCache:   canLoginFromCache,
			LastRefreshedAtText: formatTimePointer(status.State.LastRefreshedAt),
		})
	}
	return Snapshot{
		GeneratedAt: now.Format("2006-01-02 15:04:05"),
		Profiles:    profiles,
	}
}

func progressPercent(window *model.UsageWindow) int {
	if window == nil {
		return 0
	}
	value := int(window.RemainingPercent + 0.5)
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func freshnessLabel(status model.ProfileStatus) string {
	if status.State.LastError != "" {
		return "error"
	}
	if status.State.AuthStatus == model.AuthStatusActive {
		return "fresh"
	}
	if status.State.Usage != nil {
		return "cached"
	}
	return "unknown"
}

func profileLabel(profile model.Profile) string {
	if profile.Label != "" {
		return profile.Label
	}
	if profile.Email != "" {
		return profile.Email
	}
	return profile.ID
}

func nonEmpty(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func formatTimePointer(value *time.Time) string {
	if value == nil {
		return "-"
	}
	return value.Local().Format("2006-01-02 15:04:05")
}

func usageCredits(status model.ProfileStatus) *model.CreditsSnapshot {
	if status.State.Usage == nil {
		return nil
	}
	return status.State.Usage.Credits
}

func launchAddAccountConsole() (string, error) {
	cliPath, err := resolveCLIExecutable()
	if err != nil {
		return "", err
	}

	cmd := exec.Command(cliPath, "account", "add")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_CONSOLE,
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("launch add account console: %w", err)
	}
	return "Opened a separate console for Codex login. Complete login there, then refresh this window.", nil
}

func resolveCLIExecutable() (string, error) {
	currentExe, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(currentExe), "codex-lover.exe")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
	}

	for _, candidate := range []string{"codex-lover.exe", "codex-lover.cmd", "codex-lover"} {
		if path, lookupErr := exec.LookPath(candidate); lookupErr == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("could not locate codex-lover CLI")
}
