package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"codex-lover/internal/model"
	"codex-lover/internal/service"
	"codex-lover/internal/store"
)

// watchUIState holds the mutable state of the interactive watch dashboard.
type watchUIState struct {
	statuses           []model.ProfileStatus
	selectedID         string
	lastIndex          int
	statusMsg          string
	pendingRemoveID    string
	pendingRemoveLabel string
}

func (s *watchUIState) confirming() bool { return s.pendingRemoveID != "" }

func profileIDs(statuses []model.ProfileStatus) []string {
	ids := make([]string, 0, len(statuses))
	for _, status := range statuses {
		ids = append(ids, status.Profile.ID)
	}
	return ids
}

// reconcile refreshes the cached list and keeps the selection pinned to a still
// existing profile, clearing a stale pending-remove prompt.
func (s *watchUIState) reconcile(statuses []model.ProfileStatus) {
	s.statuses = statuses
	ids := profileIDs(statuses)
	id, idx := reconcileSelection(ids, s.selectedID, s.lastIndex)
	s.selectedID = id
	if idx >= 0 {
		s.lastIndex = idx
	}
	if s.pendingRemoveID != "" && indexOf(ids, s.pendingRemoveID) < 0 {
		s.pendingRemoveID = ""
		s.pendingRemoveLabel = ""
	}
}

func (s *watchUIState) selectedStatus() (model.ProfileStatus, bool) {
	for _, status := range s.statuses {
		if status.Profile.ID == s.selectedID {
			return status, true
		}
	}
	return model.ProfileStatus{}, false
}

func runWatchInteractive(ctx context.Context, svc *service.Service, st *store.Store) error {
	term, err := enterRawTerminal()
	if err != nil {
		return runWatchReadOnly(ctx, svc, st)
	}
	defer term.restore()

	// Safety net: restore the terminal if the process is signalled. In raw mode
	// Ctrl+C arrives as a byte (handled as quit), so this is mainly for external
	// signals and for Ctrl+C during the cooked add flow.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigCh)
	go func() {
		if _, ok := <-sigCh; ok {
			term.restore()
			os.Exit(1)
		}
	}()

	state := &watchUIState{}
	render := func() {
		statuses, err := statusesForDisplay(svc, st, false)
		if err != nil {
			renderWatchError(err)
			return
		}
		state.reconcile(codexStatuses(statuses))
		renderWatchInteractive(state, svc)
	}
	render()
	lastRender := time.Now()

	buf := make([]byte, 16)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		n, rerr := readStdin(buf)
		if rerr == syscall.EINTR || rerr == syscall.EAGAIN {
			// Interrupted (e.g. SIGWINCH) or no data on a non-blocking fd: not a
			// real error. Avoid a busy-spin if stdin happens to be non-blocking.
			rerr = nil
			if n == 0 {
				time.Sleep(20 * time.Millisecond)
			}
		}
		acted := false
		data := buf[:n]
		for len(data) > 0 {
			k, consumed := decodeKey(data)
			if consumed <= 0 {
				break
			}
			data = data[consumed:]
			if k == keyNone {
				continue
			}
			quit, didAct := applyWatchAction(ctx, svc, st, term, state, resolveAction(k, state.confirming()))
			if quit {
				return nil
			}
			acted = acted || didAct
		}
		if rerr != nil {
			return nil
		}
		if acted || time.Since(lastRender) >= 15*time.Second {
			render()
			lastRender = time.Now()
		}
	}
}

// applyWatchAction performs one action and reports whether to quit and whether
// the screen should be redrawn.
func applyWatchAction(ctx context.Context, svc *service.Service, st *store.Store, term *rawTerminal, state *watchUIState, action watchAction) (quit bool, acted bool) {
	switch action {
	case actUp, actDown:
		delta := -1
		if action == actDown {
			delta = 1
		}
		id, idx := moveSelection(profileIDs(state.statuses), state.selectedID, delta)
		state.selectedID = id
		if idx >= 0 {
			state.lastIndex = idx
		}
		return false, true
	case actSwitch:
		state.statusMsg = watchDoSwitch(svc, st, state)
		return false, true
	case actRemovePrompt:
		if status, ok := state.selectedStatus(); ok {
			state.pendingRemoveID = status.Profile.ID
			state.pendingRemoveLabel = profileLabelOrID(status.Profile)
		}
		return false, true
	case actConfirmRemove:
		state.statusMsg = watchDoRemove(svc, st, state)
		state.pendingRemoveID = ""
		state.pendingRemoveLabel = ""
		return false, true
	case actCancelRemove:
		state.pendingRemoveID = ""
		state.pendingRemoveLabel = ""
		state.statusMsg = "remove cancelled"
		return false, true
	case actRefresh:
		if _, err := svc.RefreshAll(); err != nil {
			state.statusMsg = "refresh failed: " + err.Error()
		} else {
			_ = tryDaemonRefresh(st)
			state.statusMsg = "refreshed"
		}
		return false, true
	case actAdd:
		watchDoAdd(ctx, svc, st, term, state)
		return false, true
	case actQuit:
		return true, false
	default:
		return false, false
	}
}

func watchDoSwitch(svc *service.Service, st *store.Store, state *watchUIState) string {
	status, ok := state.selectedStatus()
	if !ok {
		return "no account selected"
	}
	if status.State.AuthStatus == model.AuthStatusActive {
		return profileLabelOrID(status.Profile) + " is already active"
	}
	id := status.Profile.ID
	label := profileLabelOrID(status.Profile)
	if address, err := ensureDaemonRunning(st); err == nil {
		if _, err := postDaemonSwitch(address, id); err == nil {
			return "switched to " + label
		} else if !errors.Is(err, ErrDaemonUnavailable) {
			return "switch failed: " + err.Error()
		}
	}
	if _, err := svc.ActivateProfile(id); err != nil {
		return "switch failed: " + err.Error()
	}
	return "switched to " + label
}

func watchDoRemove(svc *service.Service, st *store.Store, state *watchUIState) string {
	id := state.pendingRemoveID
	label := state.pendingRemoveLabel
	if id == "" {
		return "no account selected"
	}
	if address, err := ensureDaemonRunning(st); err == nil {
		if _, err := postDaemonRemove(address, id); err == nil {
			return "removed " + label
		} else if !errors.Is(err, ErrDaemonUnavailable) {
			return "remove failed: " + err.Error()
		}
	}
	if _, err := svc.LogoutProfile(id); err != nil {
		return "remove failed: " + err.Error()
	}
	return "removed " + label
}

// watchDoAdd suspends the dashboard, runs the inline browser-login add flow in a
// cooked terminal, then resumes raw mode.
func watchDoAdd(ctx context.Context, svc *service.Service, st *store.Store, term *rawTerminal, state *watchUIState) {
	term.restore()
	clearScreen()
	profile, err := addCodexAccount(ctx, svc, st)
	fmt.Println()
	if err != nil {
		state.statusMsg = "add failed: " + err.Error()
		fmt.Printf("Add failed: %v\n", err)
	} else {
		state.selectedID = profile.ID
		state.statusMsg = "added " + profileLabelOrID(profile)
		fmt.Printf("Added %s\n", profileLabelOrID(profile))
	}
	fmt.Println()
	fmt.Println(watchDim("Press any key to return to watch..."))
	_ = term.raw()
	waitAnyKey(ctx)
}

func waitAnyKey(ctx context.Context) {
	buf := make([]byte, 8)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if n, err := readStdin(buf); err != nil || n > 0 {
			return
		}
	}
}

func renderWatchInteractive(state *watchUIState, svc *service.Service) {
	statuses := state.statuses
	now := time.Now()
	clearScreen()
	fmt.Printf("%s  %s\n\n", watchHeader("CODEX-LOVER WATCH"), watchDim(now.Format("2006-01-02 15:04:05")))

	activeLabel, activePrimaryLabel, activePrimaryWindow, activeSecondaryLabel, activeSecondaryWindow := watchActiveSummary(statuses, now)
	fmt.Printf("%s %s\n", watchSectionLabel("ACTIVE"), watchProfileTitle(activeLabel, true))
	fmt.Printf("  %s: %s\n", activePrimaryLabel, activePrimaryWindow)
	fmt.Printf("  %s: %s\n\n", activeSecondaryLabel, activeSecondaryWindow)

	if len(statuses) == 0 {
		fmt.Println(watchMuted("No Codex accounts imported yet."))
		fmt.Println()
		renderWatchFooter(state)
		return
	}

	for i, status := range statuses {
		if i > 0 {
			fmt.Println(watchDivider(72))
		}
		selected := status.Profile.ID == state.selectedID
		cursor := "  "
		title := watchProfileTitle(profileLabelOrID(status.Profile), status.State.AuthStatus == model.AuthStatusActive)
		if selected {
			cursor = watchTone("> ", watchColorCyan, true)
			title = watchTone(profileLabelOrID(status.Profile), watchColorCyan, true)
		}
		primaryWindow := profileStatusPrimary(status)
		secondaryWindow := profileStatusSecondary(status)
		fmt.Printf("%s%s %s %s %s\n", cursor, watchMarker(status), title, watchAuthBadge(status.State.AuthStatus), watchFreshnessBadge(profileFreshness(status)))
		fmt.Printf("    id: %s\n", watchDim(status.Profile.ID))
		if status.State.AuthStatus != model.AuthStatusActive && svc != nil {
			fmt.Printf("    switchable: %s\n", watchSwitchableBadge(svc.HasCachedAuth(status.Profile.ID)))
		}
		fmt.Printf("    %s: %s\n", windowDisplayLabel(primaryWindow, "primary"), watchWindowText(primaryWindow, status.State.AuthStatus, now))
		fmt.Printf("    %s: %s\n", windowDisplayLabel(secondaryWindow, "weekly"), watchWindowText(secondaryWindow, status.State.AuthStatus, now))
		if strings.TrimSpace(status.State.LastError) != "" {
			fmt.Printf("    error: %s\n", watchError(strings.TrimSpace(status.State.LastError)))
		}
		fmt.Println()
	}
	renderWatchFooter(state)
}

func renderWatchFooter(state *watchUIState) {
	if strings.TrimSpace(state.statusMsg) != "" {
		fmt.Printf("%s %s\n", watchSectionLabel("STATUS"), state.statusMsg)
	}
	if state.confirming() {
		fmt.Printf("%s Remove %s? (y/n)\n", watchBadge("CONFIRM", watchColorRed), watchTone(state.pendingRemoveLabel, watchColorYellow, true))
		return
	}
	fmt.Println(watchDim("[↑/↓ j/k] move  [s/Enter] switch  [a] add  [d] remove  [r] refresh  [q] quit"))
}
