package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"codex-lover/internal/model"
	"codex-lover/internal/service"
	"codex-lover/internal/store"
)

func runWatch(ctx context.Context, svc *service.Service, st *store.Store) error {
	if isInteractiveTerminal() {
		return runWatchInteractive(ctx, svc, st)
	}
	return runWatchReadOnly(ctx, svc, st)
}

func runWatchReadOnly(ctx context.Context, svc *service.Service, st *store.Store) error {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		statuses, err := statusesForDisplay(svc, st, false)
		if err != nil {
			renderWatchError(err)
		} else {
			renderWatch(statuses, svc)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func renderWatch(statuses []model.ProfileStatus, svc *service.Service) {
	statuses = codexStatuses(statuses)
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
		fmt.Println(watchDim("Ctrl+C to stop."))
		return
	}

	for i, status := range statuses {
		if i > 0 {
			fmt.Println(watchDivider(72))
		}

		primaryWindow := profileStatusPrimary(status)
		secondaryWindow := profileStatusSecondary(status)
		fmt.Printf("%s %s %s %s\n", watchMarker(status), watchProfileTitle(profileLabelOrID(status.Profile), status.State.AuthStatus == model.AuthStatusActive), watchAuthBadge(status.State.AuthStatus), watchFreshnessBadge(profileFreshness(status)))
		fmt.Printf("  id: %s\n", watchDim(status.Profile.ID))
		if status.State.AuthStatus != model.AuthStatusActive && svc != nil {
			fmt.Printf("  switchable: %s\n", watchSwitchableBadge(svc.HasCachedAuth(status.Profile.ID)))
		}
		fmt.Printf("  %s: %s\n", windowDisplayLabel(primaryWindow, "primary"), watchWindowText(primaryWindow, status.State.AuthStatus, now))
		fmt.Printf("  %s: %s\n", windowDisplayLabel(secondaryWindow, "weekly"), watchWindowText(secondaryWindow, status.State.AuthStatus, now))
		if status.State.LastRefreshedAt != nil {
			fmt.Printf("  refreshed: %s\n", watchDim(status.State.LastRefreshedAt.Local().Format("2006-01-02 15:04:05")))
		}
		if strings.TrimSpace(status.State.LastError) != "" {
			fmt.Printf("  error: %s\n", watchError(strings.TrimSpace(status.State.LastError)))
		}
		fmt.Println()
	}

	fmt.Println(watchDim("Ctrl+C to stop."))
}

func renderWatchError(err error) {
	clearScreen()
	fmt.Printf("%s  %s\n\n", watchHeader("CODEX-LOVER WATCH"), watchDim(time.Now().Format("2006-01-02 15:04:05")))
	fmt.Printf("error: %s\n\n", watchError(strings.TrimSpace(err.Error())))
	fmt.Println(watchDim("Ctrl+C to stop."))
}

func watchActiveSummary(statuses []model.ProfileStatus, now time.Time) (string, string, string, string, string) {
	for _, status := range statuses {
		if status.State.AuthStatus != model.AuthStatusActive {
			continue
		}
		return profileLabelOrID(status.Profile),
			windowDisplayLabel(profileStatusPrimary(status), "primary"),
			watchWindowText(profileStatusPrimary(status), status.State.AuthStatus, now),
			windowDisplayLabel(profileStatusSecondary(status), "weekly"),
			watchWindowText(profileStatusSecondary(status), status.State.AuthStatus, now)
	}
	return "none", "primary", watchMuted("unavailable"), "weekly", watchMuted("unavailable")
}

func watchWindowText(window *model.UsageWindow, authStatus string, now time.Time) string {
	if window == nil {
		return watchMuted(formatWindowText(window, authStatus, now))
	}
	displayWindow := service.EffectiveWindowForDisplay(window, authStatus, now)
	if displayWindow == nil {
		return watchMuted("unavailable")
	}
	remaining := int(displayWindow.RemainingPercent + 0.5)
	if remaining < 0 {
		remaining = 0
	}
	if remaining > 100 {
		remaining = 100
	}
	return fmt.Sprintf("%s %s %s", renderProgressBar(remaining, 24), watchUsageBadge(remaining), watchWindowSummary(window, authStatus, now))
}

func renderProgressBar(percent int, width int) string {
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
	bar := "[" + strings.Repeat("=", filled) + strings.Repeat(".", width-filled) + "]"
	return watchTone(bar+fmt.Sprintf(" %3d%%", percent), watchPercentColor(percent), true)
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func watchWindowSummary(window *model.UsageWindow, authStatus string, now time.Time) string {
	summary := formatWindowText(window, authStatus, now)
	if summary == "unavailable" || summary == "no cached usage" {
		return watchMuted(summary)
	}
	if strings.Contains(summary, "reset inferred") || strings.Contains(summary, "cached") {
		return watchTone(summary, watchColorCyan, false)
	}
	return summary
}

func watchMarker(status model.ProfileStatus) string {
	if status.State.AuthStatus == model.AuthStatusActive {
		return watchTone(">", watchColorGreen, true)
	}
	if status.State.LastError != "" {
		return watchTone("!", watchColorRed, true)
	}
	return watchDim("-")
}

func watchProfileTitle(label string, active bool) string {
	if active {
		return watchTone(label, watchColorWhite, true)
	}
	return label
}

func watchAuthBadge(authStatus string) string {
	value := strings.ToUpper(emptyDash(authStatus))
	switch authStatus {
	case model.AuthStatusActive:
		return watchBadge(value, watchColorGreen)
	case model.AuthStatusLoggedOut:
		return watchBadge(value, watchColorYellow)
	case model.AuthStatusError:
		return watchBadge(value, watchColorRed)
	default:
		return watchBadge(value, watchColorBlue)
	}
}

func watchFreshnessBadge(freshness string) string {
	value := strings.ToUpper(emptyDash(freshness))
	switch freshness {
	case "fresh":
		return watchBadge(value, watchColorGreen)
	case "cached":
		return watchBadge(value, watchColorCyan)
	case "error":
		return watchBadge(value, watchColorRed)
	default:
		return watchBadge(value, watchColorBlue)
	}
}

func watchSwitchableBadge(value bool) string {
	if value {
		return watchBadge("READY", watchColorGreen)
	}
	return watchBadge("NO CACHE", watchColorYellow)
}

func watchUsageBadge(percent int) string {
	switch {
	case percent <= 10:
		return watchBadge("LIMIT", watchColorRed)
	case percent <= 20:
		return watchBadge("LOW", watchColorYellow)
	case percent <= 40:
		return watchBadge("WATCH", watchColorYellow)
	default:
		return watchBadge("OK", watchColorGreen)
	}
}

func watchHeader(text string) string {
	return watchTone(text, watchColorCyan, true)
}

func watchSectionLabel(text string) string {
	return watchBadge(text, watchColorMagenta)
}

func watchDivider(width int) string {
	if width <= 0 {
		width = 40
	}
	return watchDim(strings.Repeat("-", width))
}

func watchBadge(text string, color string) string {
	return watchTone("["+text+"]", color, true)
}

func watchDim(text string) string {
	return watchTone(text, watchColorDim, false)
}

func watchMuted(text string) string {
	return watchTone(text, watchColorBlue, false)
}

func watchError(text string) string {
	return watchTone(text, watchColorRed, true)
}

func watchPercentColor(percent int) string {
	switch {
	case percent <= 10:
		return watchColorRed
	case percent <= 20:
		return watchColorYellow
	case percent <= 40:
		return watchColorCyan
	default:
		return watchColorGreen
	}
}

func watchTone(text string, color string, bold bool) string {
	if !watchUsesANSI() || text == "" {
		return text
	}
	var prefix strings.Builder
	prefix.WriteString("\033[")
	if bold {
		prefix.WriteString("1;")
	}
	prefix.WriteString(color)
	prefix.WriteByte('m')
	return prefix.String() + text + "\033[0m"
}

func watchUsesANSI() bool {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("TERM")), "dumb")
}

const (
	watchColorDim     = "2"
	watchColorRed     = "31"
	watchColorGreen   = "32"
	watchColorYellow  = "33"
	watchColorBlue    = "34"
	watchColorMagenta = "35"
	watchColorCyan    = "36"
	watchColorWhite   = "97"
)
