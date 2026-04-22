package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"codex-lover/internal/model"
	"codex-lover/internal/notify"
	"codex-lover/internal/service"
)

func (a *App) runBackgroundLoops() {
	if a.ensureReady() != nil {
		return
	}

	a.mu.Lock()
	_, _ = a.refreshLockedWithOptions(true, service.RefreshOptions{})
	a.mu.Unlock()

	refreshTicker := time.NewTicker(15 * time.Second)
	defer refreshTicker.Stop()

	backgroundUsageTicker := time.NewTicker(15 * time.Minute)
	defer backgroundUsageTicker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-refreshTicker.C:
			a.mu.Lock()
			_, _ = a.refreshLockedWithOptions(true, a.backgroundRefreshOptionsLocked())
			a.mu.Unlock()
		case <-backgroundUsageTicker.C:
			a.mu.Lock()
			_ = a.refreshLoggedOutLocked()
			a.mu.Unlock()
		}
	}
}

func (a *App) refreshLocked(emitNotifications bool) (Snapshot, error) {
	return a.refreshLockedWithOptions(emitNotifications, service.RefreshOptions{})
}

func (a *App) refreshLockedWithOptions(emitNotifications bool, opts service.RefreshOptions) (Snapshot, error) {
	statuses, err := a.svc.RefreshAllWithOptions(opts)
	if err != nil {
		return Snapshot{}, err
	}
	a.usageSchedule.MarkUsageAttempted(opts, time.Now())

	if emitNotifications {
		for _, event := range a.thresholds.collectThresholdEvents(statuses) {
			_ = notify.New().Send(notify.Event{
				Title:   event.Title,
				Message: event.Message,
				Level:   notify.LevelWarning,
			})
		}
	}

	switchResult, err := a.svc.AutoSwitchLimitedCodex(statuses)
	if err == nil && switchResult.Changed {
		if emitNotifications {
			_ = notify.New().Send(notify.Event{
				Title:   "Codex account switched",
				Message: "Da chuyen tu " + profileLabel(switchResult.From) + " sang " + profileLabel(switchResult.To),
				Level:   notify.LevelInfo,
			})
		}
		statuses, err = a.svc.RefreshAllWithOptions(opts)
		if err == nil {
			a.usageSchedule.MarkUsageAttempted(opts, time.Now())
		}
	}
	if err != nil {
		return Snapshot{}, err
	}

	rotateResult, err := a.svc.AutoRotateCodex(statuses)
	if err == nil && rotateResult.Changed {
		statuses, err = a.svc.RefreshAllWithOptions(opts)
		if err == nil {
			a.usageSchedule.MarkUsageAttempted(opts, time.Now())
		}
	}
	if err != nil {
		return Snapshot{}, err
	}

	_ = a.syncOpenCodeLocked(statuses)

	snapshot := buildSnapshot(statuses, a.svc)
	a.lastSnapshot = snapshot
	if a.tray != nil {
		a.tray.Update(snapshot)
	}
	return snapshot, nil
}

func (a *App) backgroundRefreshOptionsLocked() service.RefreshOptions {
	return service.RefreshOptions{
		SkipUsageForTools: a.usageSchedule.SkipUsageTools(time.Now()),
	}
}

func (a *App) currentSnapshotLocked() (Snapshot, error) {
	statuses, err := a.svc.ProfileStatuses()
	if err != nil {
		if a.lastSnapshot.GeneratedAt != "" {
			return a.lastSnapshot, nil
		}
		return Snapshot{}, err
	}
	snapshot := buildSnapshot(statuses, a.svc)
	a.lastSnapshot = snapshot
	if a.tray != nil {
		a.tray.Update(snapshot)
	}
	return snapshot, nil
}

func (a *App) refreshLoggedOutLocked() error {
	statuses, err := a.svc.ProfileStatuses()
	if err != nil {
		return err
	}
	statuses, _, err = a.svc.RefreshLoggedOutCachedUsage(statuses)
	if err != nil {
		return err
	}
	a.lastSnapshot = buildSnapshot(statuses, a.svc)
	if a.tray != nil {
		a.tray.Update(a.lastSnapshot)
	}
	return nil
}

func (a *App) syncOpenCodeLocked(statuses []model.ProfileStatus) error {
	fingerprint := activeCodexSyncFingerprint(statuses)
	if fingerprint == "" || fingerprint == a.openCodeSync.fingerprint {
		return nil
	}

	result, err := a.svc.SyncOpenCodeFromActiveCodex(statuses)
	if err != nil {
		a.openCodeSync.fingerprint = ""
		return err
	}
	if result.AccountID != "" {
		a.openCodeSync.fingerprint = fingerprint
	}
	return nil
}

type desktopOpenCodeSyncCache struct {
	fingerprint string
}

type providerUsageSchedule struct {
	lastAttempted map[string]time.Time
}

func newProviderUsageSchedule() providerUsageSchedule {
	return providerUsageSchedule{
		lastAttempted: map[string]time.Time{},
	}
}

func (s *providerUsageSchedule) SkipUsageTools(now time.Time) map[string]bool {
	skipped := map[string]bool{}
	if last := s.lastAttempted[model.ToolClaude]; !last.IsZero() && now.Sub(last) < 60*time.Second {
		skipped[model.ToolClaude] = true
	}
	return skipped
}

func (s *providerUsageSchedule) MarkUsageAttempted(opts service.RefreshOptions, now time.Time) {
	if s.lastAttempted == nil {
		s.lastAttempted = map[string]time.Time{}
	}
	if !shouldSkipUsageTool(opts, model.ToolCodex) {
		s.lastAttempted[model.ToolCodex] = now
	}
	if !shouldSkipUsageTool(opts, model.ToolClaude) {
		s.lastAttempted[model.ToolClaude] = now
	}
}

func shouldSkipUsageTool(opts service.RefreshOptions, tool string) bool {
	if len(opts.SkipUsageForTools) == 0 {
		return false
	}
	return opts.SkipUsageForTools[tool]
}

func activeCodexSyncFingerprint(statuses []model.ProfileStatus) string {
	for _, status := range statuses {
		if status.Profile.Tool != model.ToolCodex || status.State.AuthStatus != model.AuthStatusActive {
			continue
		}
		if status.Profile.AccountID == "" || status.State.AuthFingerprint == "" {
			return ""
		}
		sum := sha256.Sum256([]byte(strings.Join([]string{
			status.Profile.ID,
			status.Profile.HomePath,
			status.Profile.AccountID,
			status.State.AuthFingerprint,
		}, "\x00")))
		return hex.EncodeToString(sum[:])
	}
	return ""
}
