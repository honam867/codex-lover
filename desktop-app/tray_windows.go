//go:build windows

package main

import (
	_ "embed"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/getlantern/systray"
)

//go:embed build/windows/icon.ico
var trayIconICO []byte

type windowsTray struct {
	app *App

	readyOnce sync.Once
	closeOnce sync.Once
	readyCh   chan struct{}
	doneCh    chan struct{}

	mu          sync.Mutex
	lastTooltip string
	lastAccount string
	lastReset   string

	accountItem *systray.MenuItem
	resetItem   *systray.MenuItem
	openItem    *systray.MenuItem
	quitItem    *systray.MenuItem
}

func (a *App) startTray() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.tray != nil {
		return
	}
	tray := newWindowsTray(a)
	a.tray = tray
	tray.Start()
}

func newWindowsTray(app *App) *windowsTray {
	return &windowsTray{
		app:     app,
		readyCh: make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

func (t *windowsTray) Start() {
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		systray.Run(t.onReady, t.onExit)
	}()
}

func (t *windowsTray) onReady() {
	if len(trayIconICO) > 0 {
		systray.SetIcon(trayIconICO)
	}
	systray.SetTooltip("codex-lover")

	t.accountItem = systray.AddMenuItem("Active: loading...", "")
	t.accountItem.Disable()
	t.resetItem = systray.AddMenuItem("5H - | W -", "")
	t.resetItem.Disable()
	systray.AddSeparator()
	t.openItem = systray.AddMenuItem("Open", "Open account manager")
	t.quitItem = systray.AddMenuItem("Quit", "Quit codex-lover")

	go t.handleOpen()
	go t.handleQuit()

	t.readyOnce.Do(func() {
		close(t.readyCh)
	})
	t.applyCachedState()
}

func (t *windowsTray) onExit() {
	select {
	case <-t.doneCh:
	default:
		close(t.doneCh)
	}
}

func (t *windowsTray) handleOpen() {
	for {
		select {
		case <-t.openItem.ClickedCh:
			t.app.restoreFromTray()
		case <-t.doneCh:
			return
		}
	}
}

func (t *windowsTray) handleQuit() {
	for {
		select {
		case <-t.quitItem.ClickedCh:
			t.app.quitFromTray()
			return
		case <-t.doneCh:
			return
		}
	}
}

func (t *windowsTray) Update(snapshot Snapshot) {
	account, resets, tooltip := trayTextFromSnapshot(snapshot)

	t.mu.Lock()
	t.lastAccount = account
	t.lastReset = resets
	t.lastTooltip = tooltip
	t.mu.Unlock()

	select {
	case <-t.readyCh:
		t.applyCachedState()
	default:
	}
}

func (t *windowsTray) applyCachedState() {
	t.mu.Lock()
	account := t.lastAccount
	resets := t.lastReset
	tooltip := t.lastTooltip
	t.mu.Unlock()

	if account == "" {
		account = "Active: loading..."
	}
	if resets == "" {
		resets = "5H - | W -"
	}
	if tooltip == "" {
		tooltip = "codex-lover"
	}

	if t.accountItem != nil {
		t.accountItem.SetTitle(account)
		t.accountItem.Disable()
	}
	if t.resetItem != nil {
		t.resetItem.SetTitle(resets)
		t.resetItem.Disable()
	}
	systray.SetTooltip(tooltip)
}

func (t *windowsTray) Close() {
	t.closeOnce.Do(func() {
		systray.Quit()
		<-t.doneCh
	})
}

func trayTextFromSnapshot(snapshot Snapshot) (string, string, string) {
	for _, profile := range snapshot.Profiles {
		if !profile.IsActive {
			continue
		}
		label := strings.TrimSpace(profile.Label)
		if label == "" {
			label = strings.TrimSpace(profile.Email)
		}
		if label == "" {
			label = "unknown"
		}

		accountLine := "Active: " + truncateRunes(label, 44)
		reset5h := summaryResetValue(profile.PrimarySummary)
		resetWeekly := summaryResetValue(profile.SecondarySummary)
		resetLine := "5H " + reset5h + " | W " + resetWeekly
		tooltip := truncateRunes(accountLine+" | 5H "+reset5h+" | W "+resetWeekly, 120)
		return accountLine, resetLine, tooltip
	}
	return "Active: none", "5H - | W -", "No active account"
}

func summaryResetValue(summary string) string {
	const marker = "resets "
	idx := strings.Index(summary, marker)
	if idx == -1 {
		return "-"
	}
	value := strings.TrimSpace(summary[idx+len(marker):])
	if value == "" {
		return "-"
	}
	return value
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}
