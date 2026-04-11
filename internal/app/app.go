package app

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"codex-lover/internal/daemon"
	"codex-lover/internal/model"
	"codex-lover/internal/notify"
	"codex-lover/internal/service"
	"codex-lover/internal/store"
	"golang.org/x/term"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

const (
	ansiReset    = "\x1b[0m"
	ansiBold     = "\x1b[1m"
	ansiDim      = "\x1b[2m"
	ansiBlue     = "\x1b[38;5;39m"
	ansiCyan     = "\x1b[38;5;45m"
	ansiGreen    = "\x1b[38;5;42m"
	ansiYellow   = "\x1b[38;5;220m"
	ansiOrange   = "\x1b[38;5;214m"
	ansiRed      = "\x1b[38;5;203m"
	ansiMuted    = "\x1b[38;5;246m"
	ansiBgGreen  = "\x1b[48;5;42m"
	ansiBgOrange = "\x1b[48;5;214m"
	ansiBgRed    = "\x1b[48;5;203m"
	ansiBgTrack  = "\x1b[48;5;238m"
)

func Run(ctx context.Context, args []string) error {
	st, err := store.New()
	if err != nil {
		return err
	}
	if err := st.Ensure(); err != nil {
		return err
	}
	svc := service.New(st)

	if len(args) == 0 {
		return runMenu(ctx, svc, st)
	}

	switch args[0] {
	case "profile":
		return runProfileCommand(svc, st, args[1:])
	case "refresh":
		statuses, err := svc.RefreshAll()
		if err != nil {
			return err
		}
		printStatuses(statuses)
		return nil
	case "status":
		statuses, err := svc.RefreshAll()
		if err != nil {
			return err
		}
		printStatuses(statuses)
		return nil
	case "watch":
		return runWatch(ctx, svc)
	case "daemon":
		cfg, err := st.LoadConfig()
		if err != nil {
			return err
		}
		server := daemon.New(cfg.Daemon.ListenAddress, svc)
		fmt.Printf("codex-lover daemon listening on http://%s\n", cfg.Daemon.ListenAddress)
		return server.Run(ctx, time.Duration(cfg.PollIntervalSeconds)*time.Second)
	case "daemon-status":
		cfg, err := st.LoadConfig()
		if err != nil {
			return err
		}
		statuses, err := fetchDaemonStatuses(cfg.Daemon.ListenAddress)
		if err != nil {
			return err
		}
		printStatuses(statuses)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runProfileCommand(svc *service.Service, st *store.Store, args []string) error {
	if len(args) == 0 {
		return errors.New("profile command requires a subcommand")
	}
	switch args[0] {
	case "import":
		if len(args) < 2 || args[1] != model.ToolCodex {
			return errors.New("usage: codex-lover profile import codex --label NAME --home PATH")
		}
		label, homePath, err := parseImportFlags(args[2:])
		if err != nil {
			return err
		}
		if !filepath.IsAbs(homePath) {
			abs, err := filepath.Abs(homePath)
			if err != nil {
				return err
			}
			homePath = abs
		}
		profile, err := svc.ImportCodexProfile(label, homePath)
		if err != nil {
			return err
		}
		fmt.Printf("Imported profile %s (%s)\n", profile.Label, profile.HomePath)
		return nil
	case "list":
		statuses, err := st.ProfileStatuses()
		if err != nil {
			return err
		}
		printStatuses(statuses)
		return nil
	default:
		return fmt.Errorf("unknown profile subcommand %q", args[0])
	}
}

func parseImportFlags(args []string) (string, string, error) {
	var label string
	var home string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--label":
			i++
			if i >= len(args) {
				return "", "", errors.New("missing value for --label")
			}
			label = args[i]
		case "--home":
			i++
			if i >= len(args) {
				return "", "", errors.New("missing value for --home")
			}
			home = args[i]
		default:
			return "", "", fmt.Errorf("unknown flag %q", args[i])
		}
	}
	if home == "" {
		return "", "", errors.New("missing required flag --home")
	}
	return label, home, nil
}

func runWatch(ctx context.Context, svc *service.Service) error {
	refreshTicker := time.NewTicker(15 * time.Second)
	defer refreshTicker.Stop()
	backgroundUsageTicker := time.NewTicker(15 * time.Minute)
	defer backgroundUsageTicker.Stop()
	blinkTicker := time.NewTicker(500 * time.Millisecond)
	defer blinkTicker.Stop()

	enterAlternateScreen()
	defer exitAlternateScreen()
	fmt.Print("\x1b[?25l")
	defer fmt.Print("\x1b[?25h")
	clearScreen()

	statuses, err := svc.RefreshAll()
	if err != nil {
		return err
	}
	watchNotifications := newWatchNotifications()
	notificationText := watchNotifications.ProcessThresholds(statuses)
	switchResult := autoSwitchCodexFromWatch(svc, &statuses)
	switchText := switchResult.Text
	if switchResult.Result.Changed {
		notificationText = watchNotifications.ProcessSwitch(switchResult.Result)
	}
	openCodeSync := &openCodeSyncCache{}
	openCodeSyncText := openCodeSync.Sync(svc, statuses)
	backgroundUsageText := style("Background usage: standing by", ansiMuted)
	liveDotVisible := true
	screen := &screenRenderState{}
	printWatch(screen, statuses, liveDotVisible, openCodeSyncText, switchText, notificationText, backgroundUsageText)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-refreshTicker.C:
			statuses, err = svc.RefreshAll()
			if err != nil {
				return err
			}
			notificationText = watchNotifications.ProcessThresholds(statuses)
			switchResult = autoSwitchCodexFromWatch(svc, &statuses)
			switchText = switchResult.Text
			if switchResult.Result.Changed {
				notificationText = watchNotifications.ProcessSwitch(switchResult.Result)
			}
			openCodeSyncText = openCodeSync.Sync(svc, statuses)
			printWatch(screen, statuses, liveDotVisible, openCodeSyncText, switchText, notificationText, backgroundUsageText)
		case <-backgroundUsageTicker.C:
			var refreshedCount int
			statuses, refreshedCount, err = svc.RefreshLoggedOutCachedUsage(statuses)
			if err != nil {
				backgroundUsageText = style("Background usage: "+err.Error(), ansiRed)
			} else if refreshedCount > 0 {
				backgroundUsageText = style(
					fmt.Sprintf("Background usage: refreshed %d logged out account(s)", refreshedCount),
					ansiGreen,
				)
			} else {
				backgroundUsageText = style("Background usage: no cached logged out account ready", ansiMuted)
			}
			printWatch(screen, statuses, liveDotVisible, openCodeSyncText, switchText, notificationText, backgroundUsageText)
		case <-blinkTicker.C:
			liveDotVisible = !liveDotVisible
			printWatch(screen, statuses, liveDotVisible, openCodeSyncText, switchText, notificationText, backgroundUsageText)
		}
	}
}

func runMenu(ctx context.Context, svc *service.Service, st *store.Store) error {
	reader := bufio.NewReader(os.Stdin)

	for {
		clearScreen()
		fmt.Println("codex-lover")
		fmt.Println()

		statuses, err := st.ProfileStatuses()
		if err == nil && len(statuses) > 0 {
			printStatuses(statuses)
			fmt.Println()
		} else {
			fmt.Println("No profiles imported yet.")
			fmt.Println()
		}

		fmt.Println("Menu:")
		fmt.Println("  1. Import default Codex profile (~/.codex)")
		fmt.Println("  2. Refresh and show status")
		fmt.Println("  3. Watch live status")
		fmt.Println("  4. Start daemon")
		fmt.Println("  5. List profiles")
		fmt.Println("  6. Manage accounts")
		fmt.Println("  q. Quit")
		fmt.Println()
		fmt.Print("Choose: ")

		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		choice := strings.TrimSpace(strings.ToLower(line))

		switch choice {
		case "1":
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			defaultHome := filepath.Join(home, ".codex")
			if _, err := svc.ImportCodexProfile("default", defaultHome); err != nil {
				fmt.Printf("\nImport failed: %v\n", err)
			} else {
				fmt.Printf("\nImported default Codex profile from %s\n", defaultHome)
			}
			waitForEnter(reader)
		case "2":
			statuses, err := svc.RefreshAll()
			if err != nil {
				fmt.Printf("\nRefresh failed: %v\n", err)
			} else {
				clearScreen()
				fmt.Println("codex-lover status")
				fmt.Println()
				printStatuses(statuses)
			}
			waitForEnter(reader)
		case "3":
			return runWatch(ctx, svc)
		case "4":
			cfg, err := st.LoadConfig()
			if err != nil {
				return err
			}
			server := daemon.New(cfg.Daemon.ListenAddress, svc)
			fmt.Printf("\nStarting daemon on http://%s\n\n", cfg.Daemon.ListenAddress)
			return server.Run(ctx, time.Duration(cfg.PollIntervalSeconds)*time.Second)
		case "5":
			clearScreen()
			fmt.Println("codex-lover profiles")
			fmt.Println()
			statuses, err := st.ProfileStatuses()
			if err != nil {
				fmt.Printf("Failed to load profiles: %v\n", err)
			} else {
				printStatuses(statuses)
			}
			waitForEnter(reader)
		case "6":
			if err := runManageAccounts(svc, st); err != nil {
				fmt.Printf("\nManage accounts failed: %v\n", err)
				waitForEnter(reader)
			}
		case "q", "quit", "exit":
			return nil
		default:
			fmt.Println()
			fmt.Printf("Unknown choice: %s\n", choice)
			waitForEnter(reader)
		}
	}
}

func runManageAccounts(svc *service.Service, st *store.Store) error {
	statuses, err := svc.RefreshAll()
	if err != nil {
		statuses, err = st.ProfileStatuses()
		if err != nil {
			return err
		}
	}

	if len(statuses) == 0 {
		clearScreen()
		fmt.Println("Manage accounts")
		fmt.Println()
		fmt.Println("No accounts available.")
		fmt.Println()
		fmt.Println("Press Enter to go back.")
		reader := bufio.NewReader(os.Stdin)
		waitForEnter(reader)
		return nil
	}

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("manage accounts requires a terminal")
	}

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)
	enterAlternateScreen()
	defer exitAlternateScreen()

	reader := bufio.NewReader(os.Stdin)
	selectedIndex := 0
	actionIndex := 0
	mode := "list"
	statusText := style("Select an account and press Enter.", ansiMuted)

	for {
		if selectedIndex >= len(statuses) && len(statuses) > 0 {
			selectedIndex = len(statuses) - 1
		}
		if selectedIndex < 0 {
			selectedIndex = 0
		}

		clearScreen()
		fmt.Println("Manage accounts")
		fmt.Println()
		fmt.Println(statusText)
		fmt.Println()

		if mode == "list" {
			fmt.Println(style("Use Up/Down to move, Enter to open, q to go back.", ansiDim, ansiMuted))
			fmt.Println()
			for i, item := range statuses {
				prefix := "  "
				if i == selectedIndex {
					prefix = style("> ", ansiBold, ansiCyan)
				}
				fmt.Println(prefix + renderManageAccountLine(item))
			}
		} else {
			current := statuses[selectedIndex]
			fmt.Println(style("Use Up/Down to move, Enter to select, Esc to go back.", ansiDim, ansiMuted))
			fmt.Println()
			fmt.Println(renderManageAccountLine(current))
			fmt.Println()
			options := []string{
				"Log out and delete account data",
				"Back",
			}
			for i, option := range options {
				prefix := "  "
				if i == actionIndex {
					prefix = style("> ", ansiBold, ansiCyan)
				}
				fmt.Println(prefix + option)
			}
		}

		key, err := readMenuKey(reader)
		if err != nil {
			return err
		}

		switch mode {
		case "list":
			switch key {
			case menuKeyUp:
				selectedIndex--
				if selectedIndex < 0 {
					selectedIndex = len(statuses) - 1
				}
			case menuKeyDown:
				selectedIndex++
				if selectedIndex >= len(statuses) {
					selectedIndex = 0
				}
			case menuKeyEnter:
				mode = "actions"
				actionIndex = 0
				statusText = style("Choose an action for the selected account.", ansiMuted)
			case menuKeyEscape, menuKeyQuit:
				return nil
			}
		case "actions":
			switch key {
			case menuKeyUp:
				actionIndex--
				if actionIndex < 0 {
					actionIndex = 1
				}
			case menuKeyDown:
				actionIndex++
				if actionIndex > 1 {
					actionIndex = 0
				}
			case menuKeyEscape:
				mode = "list"
				statusText = style("Select an account and press Enter.", ansiMuted)
			case menuKeyEnter:
				if actionIndex == 0 {
					result, err := svc.LogoutProfile(statuses[selectedIndex].Profile.ID)
					if err != nil {
						statusText = style("Logout failed: "+err.Error(), ansiRed)
					} else {
						statuses, err = st.ProfileStatuses()
						if err != nil {
							return err
						}
						statusText = style(buildLogoutSummary(result), ansiGreen)
						mode = "list"
						if len(statuses) == 0 {
							return nil
						}
						if selectedIndex >= len(statuses) {
							selectedIndex = len(statuses) - 1
						}
					}
				} else {
					mode = "list"
					statusText = style("Select an account and press Enter.", ansiMuted)
				}
			case menuKeyQuit:
				mode = "list"
				statusText = style("Select an account and press Enter.", ansiMuted)
			}
		}
	}
}

type menuKey int

const (
	menuKeyUnknown menuKey = iota
	menuKeyUp
	menuKeyDown
	menuKeyEnter
	menuKeyEscape
	menuKeyQuit
)

func readMenuKey(reader *bufio.Reader) (menuKey, error) {
	b, err := reader.ReadByte()
	if err != nil {
		return menuKeyUnknown, err
	}

	switch b {
	case 0, 224:
		next, err := reader.ReadByte()
		if err != nil {
			return menuKeyUnknown, err
		}
		switch next {
		case 72:
			return menuKeyUp, nil
		case 80:
			return menuKeyDown, nil
		default:
			return menuKeyUnknown, nil
		}
	case 27:
		if reader.Buffered() >= 2 {
			next, err := reader.ReadByte()
			if err != nil {
				return menuKeyUnknown, err
			}
			third, err := reader.ReadByte()
			if err != nil {
				return menuKeyUnknown, err
			}
			if next == '[' {
				switch third {
				case 'A':
					return menuKeyUp, nil
				case 'B':
					return menuKeyDown, nil
				}
			}
		}
		return menuKeyEscape, nil
	case '\r', '\n':
		return menuKeyEnter, nil
	case 'q', 'Q':
		return menuKeyQuit, nil
	case 'k', 'K':
		return menuKeyUp, nil
	case 'j', 'J':
		return menuKeyDown, nil
	default:
		return menuKeyUnknown, nil
	}
}

func renderManageAccountLine(item model.ProfileStatus) string {
	account := profileLabelOrID(item.Profile)
	email := emptyDash(item.Profile.Email)
	return style(account, ansiBold, ansiCyan) +
		"  " + renderBadge(strings.ToUpper(emptyDash(profileStatusPlan(item))), ansiYellow) +
		" " + renderBadge(authStatusBadgeText(item), authStatusColor(item)) +
		" " + style(email, ansiMuted)
}

func buildLogoutSummary(result service.LogoutResult) string {
	parts := []string{"Removed " + profileLabelOrID(result.Profile)}
	if result.RemovedHomeAuth {
		parts = append(parts, "logged out current Codex auth")
	}
	if result.RemovedCache {
		parts = append(parts, "deleted cached auth")
	}
	return strings.Join(parts, " | ")
}

func fetchDaemonStatuses(address string) ([]model.ProfileStatus, error) {
	resp, err := http.Get("http://" + address + "/v1/status")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("daemon returned %s", resp.Status)
	}
	var statuses []model.ProfileStatus
	if err := json.NewDecoder(resp.Body).Decode(&statuses); err != nil {
		return nil, err
	}
	return statuses, nil
}

func printStatuses(statuses []model.ProfileStatus) {
	printStatusesWithOptions(statuses, statusRenderOptions{liveDotVisible: true})
}

type statusRenderOptions struct {
	liveDotVisible bool
}

func printStatusesWithOptions(statuses []model.ProfileStatus, options statusRenderOptions) {
	printStatusesWithWidth(statuses, options, statusCardWidth())
}

func printStatusesWithWidth(statuses []model.ProfileStatus, options statusRenderOptions, width int) {
	if len(statuses) == 0 {
		fmt.Println("No profiles imported yet.")
		return
	}

	for i, item := range statuses {
		if i > 0 {
			fmt.Println()
		}
		fmt.Print(renderStatusCard(item, width, options))
	}
}

type screenRenderState struct {
	width  int
	height int
}

func printWatch(screen *screenRenderState, statuses []model.ProfileStatus, liveDotVisible bool, openCodeSyncText string, switchText string, notificationText string, backgroundUsageText string) {
	cols, rows := terminalSize()
	if screen == nil || screen.width != cols || screen.height != rows {
		clearScreen()
		if screen != nil {
			screen.width = cols
			screen.height = rows
		}
	} else {
		moveCursorHome()
	}
	fmt.Printf("codex-lover watch  %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
	if strings.TrimSpace(switchText) != "" {
		fmt.Println(switchText)
	}
	if strings.TrimSpace(openCodeSyncText) != "" {
		fmt.Println(openCodeSyncText)
	}
	if strings.TrimSpace(notificationText) != "" {
		fmt.Println(notificationText)
	}
	if strings.TrimSpace(backgroundUsageText) != "" {
		fmt.Println(backgroundUsageText)
	}
	if strings.TrimSpace(switchText) != "" || strings.TrimSpace(openCodeSyncText) != "" || strings.TrimSpace(notificationText) != "" || strings.TrimSpace(backgroundUsageText) != "" {
		fmt.Println()
	}
	printStatusesWithWidth(statuses, statusRenderOptions{liveDotVisible: liveDotVisible}, statusCardWidthFor(cols))
	clearToEndOfScreen()
}

type watchSwitchResult struct {
	Text   string
	Result service.SwitchResult
}

func autoSwitchCodexFromWatch(svc *service.Service, statuses *[]model.ProfileStatus) watchSwitchResult {
	result, err := svc.AutoSwitchLimitedCodex(*statuses)
	if err != nil {
		return watchSwitchResult{Text: style("Auto switch: "+err.Error(), ansiRed)}
	}
	if !result.Checked {
		return watchSwitchResult{Result: result}
	}
	if result.Changed {
		refreshed, err := svc.RefreshAll()
		if err != nil {
			return watchSwitchResult{
				Text:   style("Auto switch: switched but refresh failed: "+err.Error(), ansiRed),
				Result: result,
			}
		}
		*statuses = refreshed
		return watchSwitchResult{
			Text:   style("Auto switch: "+profileLabelOrID(result.From)+" -> "+profileLabelOrID(result.To), ansiGreen),
			Result: result,
		}
	}
	if strings.TrimSpace(result.Reason) == "" || result.Reason == "active account still has quota" {
		return watchSwitchResult{
			Text:   style("Auto switch: standing by", ansiMuted),
			Result: result,
		}
	}
	return watchSwitchResult{
		Text:   style("Auto switch: "+result.Reason, ansiOrange),
		Result: result,
	}
}

type openCodeSyncCache struct {
	fingerprint string
	statusText  string
}

func (cache *openCodeSyncCache) Sync(svc *service.Service, statuses []model.ProfileStatus) string {
	fingerprint := activeCodexSyncFingerprint(statuses)
	if fingerprint == "" {
		cache.fingerprint = ""
		cache.statusText = style("OpenCode sync: no active Codex account", ansiRed)
		return cache.statusText
	}
	if fingerprint == cache.fingerprint && cache.statusText != "" {
		return cache.statusText
	}

	result, err := svc.SyncOpenCodeFromActiveCodex(statuses)
	if err != nil {
		cache.fingerprint = ""
		cache.statusText = style("OpenCode sync: "+err.Error(), ansiRed)
		return cache.statusText
	}
	accountID := clip(result.AccountID, 8)
	if result.Changed {
		cache.statusText = style("OpenCode sync: updated OpenAI oauth for "+accountID, ansiGreen)
	} else {
		cache.statusText = style("OpenCode sync: already on "+accountID, ansiMuted)
	}
	cache.fingerprint = fingerprint
	return cache.statusText
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

type watchNotifications struct {
	sender     *notify.Sender
	statusText string
	windows    map[string]windowThresholdState
}

func newWatchNotifications() *watchNotifications {
	return &watchNotifications{
		sender:     notify.New(),
		statusText: style("Notifications: standing by", ansiMuted),
		windows:    map[string]windowThresholdState{},
	}
}

func (watcher *watchNotifications) ProcessThresholds(statuses []model.ProfileStatus) string {
	events := watcher.collectThresholdEvents(statuses)
	if len(events) == 0 {
		return watcher.statusText
	}

	lastEvent := events[len(events)-1]
	for _, event := range events {
		_ = watcher.sender.Send(notify.Event{
			Title:   event.Title,
			Message: event.Message,
			Level:   notify.LevelWarning,
		})
	}
	watcher.statusText = style("Notifications: "+lastEvent.Message, ansiOrange)
	return watcher.statusText
}

func (watcher *watchNotifications) ProcessSwitch(result service.SwitchResult) string {
	if !result.Changed {
		return watcher.statusText
	}

	message := "Da chuyen tu " + profileLabelOrID(result.From) + " sang " + profileLabelOrID(result.To)
	_ = watcher.sender.Send(notify.Event{
		Title:   "Codex account switched",
		Message: message,
		Level:   notify.LevelInfo,
	})
	watcher.statusText = style("Notifications: "+message, ansiGreen)
	return watcher.statusText
}

type thresholdEvent struct {
	Title   string
	Message string
}

type windowThresholdState struct {
	ResetKey             string
	LastRemainingPercent float64
	HasLastRemaining     bool
	Notified20           bool
	Notified10           bool
}

func (watcher *watchNotifications) collectThresholdEvents(statuses []model.ProfileStatus) []thresholdEvent {
	now := time.Now()
	events := []thresholdEvent{}

	for _, status := range statuses {
		if status.Profile.Tool != model.ToolCodex || status.State.AuthStatus != model.AuthStatusActive {
			continue
		}
		events = append(events, watcher.collectWindowThresholdEvents(status, "5H", profileStatusPrimary(status), now)...)
		events = append(events, watcher.collectWindowThresholdEvents(status, "WEEKLY", profileStatusSecondary(status), now)...)
	}

	return events
}

func (watcher *watchNotifications) collectWindowThresholdEvents(
	status model.ProfileStatus,
	windowLabel string,
	window *model.UsageWindow,
	now time.Time,
) []thresholdEvent {
	if window == nil {
		return nil
	}

	displayWindow := service.EffectiveWindowForDisplay(window, status.State.AuthStatus, now)
	if displayWindow == nil {
		return nil
	}

	key := status.Profile.ID + "\x00" + windowLabel
	state := watcher.windows[key]
	resetKey := notificationResetKey(displayWindow)
	if state.ResetKey != resetKey {
		state = windowThresholdState{ResetKey: resetKey}
	}

	current := displayWindow.RemainingPercent
	events := []thresholdEvent{}
	if state.HasLastRemaining {
		switch {
		case !state.Notified10 && state.LastRemainingPercent > 10 && current <= 10:
			state.Notified10 = true
			state.Notified20 = true
			events = append(events, buildThresholdEvent(status, windowLabel, 10, displayWindow))
		case !state.Notified20 && state.LastRemainingPercent > 20 && current <= 20:
			state.Notified20 = true
			events = append(events, buildThresholdEvent(status, windowLabel, 20, displayWindow))
		}
	}

	state.LastRemainingPercent = current
	state.HasLastRemaining = true
	watcher.windows[key] = state
	return events
}

func buildThresholdEvent(
	status model.ProfileStatus,
	windowLabel string,
	threshold int,
	window *model.UsageWindow,
) thresholdEvent {
	account := thresholdAccountLabel(status)
	return thresholdEvent{
		Title: fmt.Sprintf("Codex account reached %d%%", threshold),
		Message: fmt.Sprintf(
			"Account %s da cham toi %d%% %s (%s)",
			account,
			threshold,
			windowLabel,
			service.FormatWindowSummary(window),
		),
	}
}

func thresholdAccountLabel(status model.ProfileStatus) string {
	if status.Profile.Email != "" {
		return status.Profile.Email
	}
	if status.Profile.Label != "" {
		return status.Profile.Label
	}
	if status.Profile.AccountID != "" {
		return status.Profile.AccountID
	}
	return status.Profile.ID
}

func notificationResetKey(window *model.UsageWindow) string {
	if window == nil || window.ResetsAt == nil {
		return ""
	}
	return window.ResetsAt.UTC().Format(time.RFC3339)
}

func printUsage() {
	fmt.Println("codex-lover")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  profile import codex --label NAME --home PATH")
	fmt.Println("  profile list")
	fmt.Println("  refresh")
	fmt.Println("  status")
	fmt.Println("  watch")
	fmt.Println("  daemon")
	fmt.Println("  daemon-status")
}

func waitForEnter(reader *bufio.Reader) {
	fmt.Println()
	fmt.Print("Press Enter to continue...")
	_, _ = reader.ReadString('\n')
}

func clearScreen() {
	fmt.Print("\x1b[2J\x1b[H")
}

func moveCursorHome() {
	fmt.Print("\x1b[H")
}

func clearToEndOfScreen() {
	fmt.Print("\x1b[J")
}

func enterAlternateScreen() {
	fmt.Print("\x1b[?1049h")
}

func exitAlternateScreen() {
	fmt.Print("\x1b[?1049l")
}

func terminalSize() (int, int) {
	if term.IsTerminal(int(os.Stdout.Fd())) {
		if width, height, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
			return width, height
		}
	}
	return 0, 0
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func clip(value string, max int) string {
	if max < 1 {
		return ""
	}
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func profileLabelOrID(p model.Profile) string {
	if p.Label != "" {
		return p.Label
	}
	return p.ID
}

func profileStatusPlan(ps model.ProfileStatus) string {
	if ps.State.Usage != nil && ps.State.Usage.PlanType != "" {
		return ps.State.Usage.PlanType
	}
	return ps.Profile.Plan
}

func profileStatusPrimary(ps model.ProfileStatus) *model.UsageWindow {
	if ps.State.Usage == nil {
		return nil
	}
	return ps.State.Usage.Primary
}

func profileStatusSecondary(ps model.ProfileStatus) *model.UsageWindow {
	if ps.State.Usage == nil {
		return nil
	}
	return ps.State.Usage.Secondary
}

func profileStatusCredits(ps model.ProfileStatus) *model.CreditsSnapshot {
	if ps.State.Usage == nil {
		return nil
	}
	return ps.State.Usage.Credits
}

func progressBarWidth(inner int) int {
	width := (inner - 18) / 2
	if width < 8 {
		width = 8
	}
	if width > 24 {
		width = 24
	}
	return width
}

func summaryWidth(inner int, barWidth int) int {
	width := inner - 7 - 1 - (barWidth + 2) - 1
	if width < 6 {
		width = 6
	}
	return width
}

func renderStatusCard(item model.ProfileStatus, width int, options statusRenderOptions) string {
	if width < 24 {
		width = 24
	}
	inner := width - 4
	headerLabel := renderStatusHeaderLabel(item, inner)
	headerMeta := renderStatusHeaderMeta(item, inner)
	email := clip(emptyDash(item.Profile.Email), inner-len("Email: "))
	credits := service.FormatCredits(profileStatusCredits(item))
	lastError := "-"
	if strings.TrimSpace(item.State.LastError) != "" {
		lastError = clip(item.State.LastError, inner-len("Last error: "))
	}

	lines := []string{
		cardBorder(width),
		cardRowWithRight(
			headerLabel,
			renderLiveDot(item, options.liveDotVisible),
			inner,
		),
		cardRow(headerMeta, inner),
		cardRow(style("Email:", ansiBold, ansiBlue)+" "+style(email, ansiMuted), inner),
		cardRow(style("Home:", ansiBold, ansiBlue)+" "+style(clip(item.Profile.HomePath, inner-len("Home: ")), ansiDim, ansiMuted), inner),
		cardRow("", inner),
		cardRow(renderUsageLine("5H", profileStatusPrimary(item), item.State.AuthStatus, inner), inner),
		cardRow(renderUsageLine("WEEKLY", profileStatusSecondary(item), item.State.AuthStatus, inner), inner),
		cardRow("", inner),
		cardRow(style("Credits:", ansiBold, ansiBlue)+" "+style(credits, ansiOrange), inner),
		cardRow(style("Last error:", ansiBold, ansiBlue)+" "+lastErrorColor(lastError), inner),
		cardBorder(width),
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderStatusHeaderLabel(item model.ProfileStatus, inner int) string {
	maxLabel := inner
	if maxLabel < 1 {
		maxLabel = 1
	}
	return style(clip(profileLabelOrID(item.Profile), maxLabel), ansiBold, ansiCyan)
}

func renderStatusHeaderMeta(item model.ProfileStatus, inner int) string {
	parts := []string{
		renderBadge(strings.ToUpper(emptyDash(profileStatusPlan(item))), ansiYellow),
		renderBadge(authStatusBadgeText(item), authStatusColor(item)),
		renderBadge(strings.ToUpper(profileStateText(item)), stateColor(item)),
	}
	for len(parts) > 1 && visibleLen(strings.Join(parts, " ")) > inner {
		parts = parts[:len(parts)-1]
	}
	meta := strings.Join(parts, " ")
	if visibleLen(meta) <= inner {
		return meta
	}
	return clip(meta, inner)
}

func renderUsageLine(label string, window *model.UsageWindow, authStatus string, inner int) string {
	labelPart := style(padRight(label, 7), ansiBold, ansiBlue)
	barWidth := progressBarWidth(inner)
	if window == nil {
		summary := "unavailable"
		if authStatus == model.AuthStatusLoggedOut {
			summary = "no cached usage"
		}
		summary = clip(summary, summaryWidth(inner, barWidth))
		return labelPart + " " + renderProgressBar(0, barWidth, ansiMuted) + " " + style(summary, ansiDim, ansiMuted)
	}

	now := time.Now()
	displayWindow := service.EffectiveWindowForDisplay(window, authStatus, now)
	summary := service.FormatWindowSummary(displayWindow)
	if authStatus == model.AuthStatusLoggedOut {
		if service.WindowResetInferred(window, authStatus, now) {
			summary += "  reset inferred"
		} else {
			summary += "  cached"
		}
	}
	summary = clip(summary, summaryWidth(inner, barWidth))

	return labelPart +
		" " +
		renderProgressBar(displayWindow.RemainingPercent, barWidth, quotaColor(displayWindow.RemainingPercent)) +
		" " +
		style(summary, ansiDim, ansiMuted)
}

func renderProgressBar(percent float64, width int, color string) string {
	if width < 4 {
		width = 4
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	filled := int((percent / 100.0 * float64(width)) + 0.5)
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return "[" +
		style(strings.Repeat(" ", filled), quotaBackgroundColor(percent)) +
		style(strings.Repeat(" ", width-filled), ansiBgTrack) +
		"]"
}

func renderBadge(text string, color string) string {
	return style("["+text+"]", ansiBold, color)
}

func renderLiveDot(item model.ProfileStatus, visible bool) string {
	if item.State.AuthStatus != model.AuthStatusActive {
		return ""
	}
	if !visible {
		return " "
	}
	return style("●", ansiBold, ansiRed)
}

func cardBorder(width int) string {
	if width < 2 {
		width = 2
	}
	return style("+"+strings.Repeat("-", width-2)+"+", ansiBlue)
}

func cardRow(content string, inner int) string {
	if inner < 1 {
		inner = 1
	}
	padding := inner - visibleLen(content)
	if padding < 0 {
		padding = 0
	}
	return style("|", ansiBlue) + " " + content + strings.Repeat(" ", padding) + " " + style("|", ansiBlue)
}

func cardRowWithRight(left string, right string, inner int) string {
	if right == "" {
		return cardRow(left, inner)
	}
	gap := inner - visibleLen(left) - visibleLen(right)
	if gap < 1 {
		return cardRow(left+" "+right, inner)
	}
	return style("|", ansiBlue) + " " + left + strings.Repeat(" ", gap) + right + " " + style("|", ansiBlue)
}

func statusCardWidth() int {
	cols, _ := terminalSize()
	return statusCardWidthFor(cols)
}

func statusCardWidthFor(cols int) int {
	width := 96
	if cols > 0 {
		width = cols - 2
	} else if raw := strings.TrimSpace(os.Getenv("COLUMNS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			width = parsed - 2
		}
	}
	if width > 112 {
		width = 112
	}
	if cols > 0 && width > cols-1 {
		width = cols - 1
	}
	if width < 24 {
		width = 24
	}
	return width
}

func profileStateText(item model.ProfileStatus) string {
	if item.State.LastError != "" {
		return "error"
	}
	if item.State.AuthStatus == model.AuthStatusLoggedOut && item.State.Usage != nil {
		return "cached"
	}
	if item.State.Usage != nil {
		return "fresh"
	}
	return "unknown"
}

func quotaColor(percent float64) string {
	switch {
	case percent >= 70:
		return ansiGreen
	case percent >= 35:
		return ansiOrange
	default:
		return ansiRed
	}
}

func quotaBackgroundColor(percent float64) string {
	switch {
	case percent >= 70:
		return ansiBgGreen
	case percent >= 35:
		return ansiBgOrange
	default:
		return ansiBgRed
	}
}

func stateColor(item model.ProfileStatus) string {
	switch profileStateText(item) {
	case "fresh":
		return ansiGreen
	case "cached":
		return ansiOrange
	case "error":
		return ansiRed
	default:
		return ansiMuted
	}
}

func authStatusBadgeText(item model.ProfileStatus) string {
	switch item.State.AuthStatus {
	case model.AuthStatusActive:
		return "ACTIVE"
	case model.AuthStatusLoggedOut:
		return "LOGGED OUT"
	case model.AuthStatusError:
		return "AUTH ERROR"
	default:
		return "UNKNOWN"
	}
}

func authStatusColor(item model.ProfileStatus) string {
	switch item.State.AuthStatus {
	case model.AuthStatusActive:
		return ansiGreen
	case model.AuthStatusLoggedOut:
		return ansiOrange
	case model.AuthStatusError:
		return ansiRed
	default:
		return ansiMuted
	}
}

func lastErrorColor(value string) string {
	if value == "-" {
		return style(value, ansiMuted)
	}
	return style(value, ansiRed)
}

func padRight(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}

func visibleLen(value string) int {
	return utf8.RuneCountInString(ansiPattern.ReplaceAllString(value, ""))
}

func style(value string, codes ...string) string {
	if value == "" || !colorsEnabled() || len(codes) == 0 {
		return value
	}
	return strings.Join(codes, "") + value + ansiReset
}

func colorsEnabled() bool {
	return os.Getenv("NO_COLOR") == "" && !strings.EqualFold(os.Getenv("TERM"), "dumb")
}
