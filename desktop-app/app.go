package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"codex-lover/internal/model"
	"codex-lover/internal/service"
	"codex-lover/internal/store"
	"github.com/wailsapp/wails/v2/pkg/options"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
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
	Provider            string `json:"provider"`
	Plan                string `json:"plan"`
	AuthStatus          string `json:"authStatus"`
	Freshness           string `json:"freshness"`
	IsActive            bool   `json:"isActive"`
	PrimaryPercent      int    `json:"primaryPercent"`
	PrimarySummary      string `json:"primarySummary"`
	SecondaryPercent    int    `json:"secondaryPercent"`
	SecondarySummary    string `json:"secondarySummary"`
	LastError           string `json:"lastError"`
	CanLoginFromCache   bool   `json:"canLoginFromCache"`
	LastRefreshedAtText string `json:"lastRefreshedAtText"`
}

type ActionResponse struct {
	Message  string   `json:"message"`
	Error    string   `json:"error,omitempty"`
	Snapshot Snapshot `json:"snapshot"`
}

type SystemStatus struct {
	HasCodexCLI     bool   `json:"hasCodexCli"`
	CodexInstallURL string `json:"codexInstallUrl"`
}

const codexInstallURL = "https://github.com/openai/codex"

type trayController interface {
	Update(Snapshot)
	Close()
}

type App struct {
	ctx           context.Context
	svc           *service.Service
	initErr       error
	mu            sync.Mutex
	lastSnapshot  Snapshot
	thresholds    *desktopWatchNotifications
	openCodeSync  desktopOpenCodeSyncCache
	tray          trayController
	hiddenToTray  bool
	quitting      bool
	usageSchedule providerUsageSchedule
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
		svc:           service.New(st),
		thresholds:    newDesktopWatchNotifications(),
		usageSchedule: newProviderUsageSchedule(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.startTray()
	go a.runBackgroundLoops()
}

func (a *App) shutdown(ctx context.Context) {
	a.mu.Lock()
	a.quitting = true
	tray := a.tray
	a.mu.Unlock()

	if tray != nil {
		tray.Close()
	}
	_ = ctx
}

func (a *App) onSecondInstanceLaunch(_ options.SecondInstanceData) {
	a.restoreFromTray()
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

func (a *App) GetConfig() model.Config {
	if err := a.ensureReady(); err != nil {
		return model.Config{}
	}
	cfg, err := a.svc.LoadConfig()
	if err != nil {
		return model.Config{}
	}
	return cfg
}

func (a *App) SetAutoRotateCodex(enabled bool) error {
	if err := a.ensureReady(); err != nil {
		return err
	}
	cfg, err := a.svc.LoadConfig()
	if err != nil {
		return err
	}
	cfg.AutoRotateCodex = enabled
	return a.svc.SaveConfig(cfg)
}

func (a *App) SetAutoRotateThreshold(threshold float64) error {
	if err := a.ensureReady(); err != nil {
		return err
	}
	cfg, err := a.svc.LoadConfig()
	if err != nil {
		return err
	}
	cfg.AutoRotateThreshold = threshold
	return a.svc.SaveConfig(cfg)
}

func (a *App) AddAccount(provider string) ActionResponse {
	message, err := launchAddAccountConsole(provider)
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

func (a *App) GetSystemStatus() SystemStatus {
	return SystemStatus{
		HasCodexCLI:     hasCommand("codex.cmd", "codex"),
		CodexInstallURL: codexInstallURL,
	}
}

func (a *App) OpenCodexInstallPage() error {
	if a.ctx == nil {
		return fmt.Errorf("app context is not ready")
	}
	wailsruntime.BrowserOpenURL(a.ctx, codexInstallURL)
	return nil
}

func (a *App) HideToTray() {
	a.mu.Lock()
	if a.hiddenToTray || a.quitting {
		a.mu.Unlock()
		return
	}
	a.hiddenToTray = true
	a.mu.Unlock()

	if a.ctx != nil {
		wailsruntime.WindowHide(a.ctx)
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

func (a *App) restoreFromTray() {
	a.mu.Lock()
	a.hiddenToTray = false
	a.mu.Unlock()

	if a.ctx == nil {
		return
	}
	wailsruntime.Show(a.ctx)
	wailsruntime.WindowShow(a.ctx)
	wailsruntime.WindowUnminimise(a.ctx)
}

func (a *App) quitFromTray() {
	a.mu.Lock()
	if a.quitting {
		a.mu.Unlock()
		return
	}
	a.quitting = true
	tray := a.tray
	a.mu.Unlock()

	if tray != nil {
		tray.Close()
	}
	if a.ctx != nil {
		wailsruntime.Quit(a.ctx)
	}
}

func buildSnapshot(statuses []model.ProfileStatus, svc *service.Service) Snapshot {
	now := time.Now()
	profiles := make([]ProfileCard, 0, len(statuses))
	for _, status := range statuses {
		primary := service.EffectiveWindowForDisplay(status.State.UsageWindowPrimary(), status.State.AuthStatus, now)
		secondary := service.EffectiveWindowForDisplay(status.State.UsageWindowSecondary(), status.State.AuthStatus, now)
		canLoginFromCache := false
		if svc != nil && status.State.AuthStatus != model.AuthStatusActive {
			canLoginFromCache = svc.HasCachedAuth(status.Profile.ID)
		}
		profiles = append(profiles, ProfileCard{
			ID:                  status.Profile.ID,
			Label:               profileLabel(status.Profile),
			Email:               nonEmpty(status.Profile.Email, "-"),
			Provider:            profileProvider(status.Profile),
			Plan:                nonEmpty(status.Profile.Plan, "-"),
			AuthStatus:          nonEmpty(status.State.AuthStatus, model.AuthStatusUnknown),
			Freshness:           freshnessLabel(status),
			IsActive:            status.State.AuthStatus == model.AuthStatusActive,
			PrimaryPercent:      progressPercent(primary),
			PrimarySummary:      service.FormatWindowSummary(primary),
			SecondaryPercent:    progressPercent(secondary),
			SecondarySummary:    service.FormatWindowSummary(secondary),
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
	if shouldDisplayEmail(profile) {
		return profile.Email
	}
	if profile.Label != "" {
		return profile.Label
	}
	if profile.Email != "" {
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
	return normalizeDisplayLabel(profile.Email) == strings.ToLower(label)
}

func normalizeDisplayLabel(value string) string {
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
	return strings.Trim(builder.String(), "-")
}

func nonEmpty(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func profileProvider(profile model.Profile) string {
	if strings.TrimSpace(profile.Provider) != "" {
		return strings.ToLower(strings.TrimSpace(profile.Provider))
	}
	if strings.TrimSpace(profile.Tool) != "" {
		return strings.ToLower(strings.TrimSpace(profile.Tool))
	}
	return model.ToolCodex
}

func providerDisplayName(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case model.ToolClaude:
		return "Claude"
	case model.ToolCodex:
		return "Codex"
	case model.ToolKimi:
		return "Kimi"
	default:
		return strings.TrimSpace(provider)
	}
}

func hasCommand(candidates ...string) bool {
	for _, candidate := range candidates {
		if _, err := exec.LookPath(candidate); err == nil {
			return true
		}
	}
	return false
}

func formatTimePointer(value *time.Time) string {
	if value == nil {
		return "-"
	}
	return value.Local().Format("2006-01-02 15:04:05")
}

func launchAddAccountConsole(provider string) (string, error) {
	cliPath, err := resolveCLIExecutable()
	if err != nil {
		return "", err
	}
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		provider = model.ToolCodex
	}

	cmd := exec.Command(cliPath, "account", "add", provider)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_CONSOLE,
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("launch add account console: %w", err)
	}
	return fmt.Sprintf("Opened a separate console for %s login. Complete login there, then refresh this window.", providerDisplayName(provider)), nil
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
