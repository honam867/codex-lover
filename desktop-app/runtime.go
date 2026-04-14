package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"codex-lover/internal/model"
	"codex-lover/internal/notify"
)

func (a *App) runBackgroundLoops() {
	if a.ensureReady() != nil {
		return
	}

	a.mu.Lock()
	_, _ = a.refreshLocked(true)
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
			_, _ = a.refreshLocked(true)
			a.mu.Unlock()
		case <-backgroundUsageTicker.C:
			a.mu.Lock()
			_ = a.refreshLoggedOutLocked()
			a.mu.Unlock()
		}
	}
}

func (a *App) refreshLocked(emitNotifications bool) (Snapshot, error) {
	statuses, err := a.svc.RefreshAll()
	if err != nil {
		return Snapshot{}, err
	}

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
		statuses, err = a.svc.RefreshAll()
	}
	if err != nil {
		return Snapshot{}, err
	}

	_ = a.syncOpenCodeLocked(statuses)

	snapshot := buildSnapshot(statuses, a.svc)
	a.lastSnapshot = snapshot
	return snapshot, nil
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
