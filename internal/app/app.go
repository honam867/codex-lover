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
	"codex-lover/internal/service"
	"codex-lover/internal/store"
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
	blinkTicker := time.NewTicker(500 * time.Millisecond)
	defer blinkTicker.Stop()

	fmt.Print("\x1b[?25l")
	defer fmt.Print("\x1b[?25h")
	clearScreen()

	statuses, err := svc.RefreshAll()
	if err != nil {
		return err
	}
	switchText := autoSwitchCodexFromWatch(svc, &statuses)
	openCodeSync := &openCodeSyncCache{}
	openCodeSyncText := openCodeSync.Sync(svc, statuses)
	liveDotVisible := true
	printWatch(statuses, liveDotVisible, openCodeSyncText, switchText)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-refreshTicker.C:
			statuses, err = svc.RefreshAll()
			if err != nil {
				return err
			}
			switchText = autoSwitchCodexFromWatch(svc, &statuses)
			openCodeSyncText = openCodeSync.Sync(svc, statuses)
			printWatch(statuses, liveDotVisible, openCodeSyncText, switchText)
		case <-blinkTicker.C:
			liveDotVisible = !liveDotVisible
			printWatch(statuses, liveDotVisible, openCodeSyncText, switchText)
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
		case "q", "quit", "exit":
			return nil
		default:
			fmt.Println()
			fmt.Printf("Unknown choice: %s\n", choice)
			waitForEnter(reader)
		}
	}
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
	if len(statuses) == 0 {
		fmt.Println("No profiles imported yet.")
		return
	}

	width := statusCardWidth()
	for i, item := range statuses {
		if i > 0 {
			fmt.Println()
		}
		fmt.Print(renderStatusCard(item, width, options))
	}
}

func printWatch(statuses []model.ProfileStatus, liveDotVisible bool, openCodeSyncText string, switchText string) {
	moveCursorHome()
	fmt.Printf("codex-lover watch  %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
	if strings.TrimSpace(switchText) != "" {
		fmt.Println(switchText)
	}
	if strings.TrimSpace(openCodeSyncText) != "" {
		fmt.Println(openCodeSyncText)
	}
	if strings.TrimSpace(switchText) != "" || strings.TrimSpace(openCodeSyncText) != "" {
		fmt.Println()
	}
	printStatusesWithOptions(statuses, statusRenderOptions{liveDotVisible: liveDotVisible})
	clearToEndOfScreen()
}

func autoSwitchCodexFromWatch(svc *service.Service, statuses *[]model.ProfileStatus) string {
	result, err := svc.AutoSwitchLimitedCodex(*statuses)
	if err != nil {
		return style("Auto switch: "+err.Error(), ansiRed)
	}
	if !result.Checked {
		return ""
	}
	if result.Changed {
		refreshed, err := svc.RefreshAll()
		if err != nil {
			return style("Auto switch: switched but refresh failed: "+err.Error(), ansiRed)
		}
		*statuses = refreshed
		return style("Auto switch: "+profileLabelOrID(result.From)+" -> "+profileLabelOrID(result.To), ansiGreen)
	}
	if strings.TrimSpace(result.Reason) == "" || result.Reason == "active account still has quota" {
		return style("Auto switch: standing by", ansiMuted)
	}
	return style("Auto switch: "+result.Reason, ansiOrange)
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

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func clip(value string, max int) string {
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

func renderStatusCard(item model.ProfileStatus, width int, options statusRenderOptions) string {
	inner := width - 4
	label := clip(profileLabelOrID(item.Profile), 28)
	plan := strings.ToUpper(emptyDash(profileStatusPlan(item)))
	state := strings.ToUpper(profileStateText(item))
	email := clip(emptyDash(item.Profile.Email), inner-len("Email: "))
	credits := service.FormatCredits(profileStatusCredits(item))
	lastError := "-"
	if strings.TrimSpace(item.State.LastError) != "" {
		lastError = clip(item.State.LastError, inner-len("Last error: "))
	}

	lines := []string{
		cardBorder(width),
		cardRowWithRight(
			style(label, ansiBold, ansiCyan)+"  "+renderBadge(plan, ansiYellow)+" "+renderBadge(authStatusBadgeText(item), authStatusColor(item))+" "+renderBadge(state, stateColor(item)),
			renderLiveDot(item, options.liveDotVisible),
			inner,
		),
		cardRow(style("Email:", ansiBold, ansiBlue)+" "+style(email, ansiMuted), inner),
		cardRow(style("Home:", ansiBold, ansiBlue)+" "+style(clip(item.Profile.HomePath, inner-len("Home: ")), ansiDim, ansiMuted), inner),
		cardRow("", inner),
		cardRow(renderUsageLine("5H", profileStatusPrimary(item), item.State.AuthStatus), inner),
		cardRow(renderUsageLine("WEEKLY", profileStatusSecondary(item), item.State.AuthStatus), inner),
		cardRow("", inner),
		cardRow(style("Credits:", ansiBold, ansiBlue)+" "+style(credits, ansiOrange), inner),
		cardRow(style("Last error:", ansiBold, ansiBlue)+" "+lastErrorColor(lastError), inner),
		cardBorder(width),
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderUsageLine(label string, window *model.UsageWindow, authStatus string) string {
	if window == nil {
		summary := "unavailable"
		if authStatus == model.AuthStatusLoggedOut {
			summary = "no cached usage"
		}
		return style(padRight(label, 7), ansiBold, ansiBlue) + " " + renderProgressBar(0, 24, ansiMuted) + " " + style(summary, ansiDim, ansiMuted)
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

	return style(padRight(label, 7), ansiBold, ansiBlue) +
		" " +
		renderProgressBar(displayWindow.RemainingPercent, 24, quotaColor(displayWindow.RemainingPercent)) +
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
	if width < 36 {
		width = 36
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
	width := 96
	if raw := strings.TrimSpace(os.Getenv("COLUMNS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			width = parsed - 2
		}
	}
	if width < 72 {
		width = 72
	}
	if width > 112 {
		width = 112
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
