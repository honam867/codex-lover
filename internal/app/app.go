package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"codex-lover/internal/daemon"
	"codex-lover/internal/model"
	"codex-lover/internal/service"
	"codex-lover/internal/store"
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
		return runWatch(ctx, svc, st)
	}

	switch args[0] {
	case "run":
		return runServerCommand(ctx, svc, st, []string{"run"})
	case "server":
		return runServerCommand(ctx, svc, st, args[1:])
	case "watch":
		return runWatch(ctx, svc, st)
	case "account":
		return runAccountCommand(ctx, svc, st, args[1:])
	case "profile":
		return runProfileCommand(svc, st, args[1:])
	case "refresh":
		return runStatusCommand(svc, st, true, hasJSONFlag(args[1:]))
	case "status":
		return runStatusCommand(svc, st, false, hasJSONFlag(args[1:]))
	case "daemon":
		return runServerCommand(ctx, svc, st, []string{"run"})
	case "daemon-status":
		return runStatusCommand(svc, st, false, hasJSONFlag(args[1:]))
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runServerCommand(ctx context.Context, svc *service.Service, st *store.Store, args []string) error {
	if len(args) == 0 {
		args = []string{"run"}
	}
	switch args[0] {
	case "run":
		cfg, err := st.LoadConfig()
		if err != nil {
			return err
		}
		cleanupPID, err := registerManagedServerPID(st)
		if err != nil {
			return err
		}
		defer cleanupPID()
		server := daemon.New(cfg.Daemon.ListenAddress, svc)
		fmt.Printf("codex-lover server listening on http://%s\n", cfg.Daemon.ListenAddress)
		return server.Run(ctx, time.Duration(cfg.PollIntervalSeconds)*time.Second)
	case "start":
		return startManagedServer(st)
	case "stop":
		return stopManagedServer(st)
	case "status":
		return printServerStatus(st)
	default:
		return fmt.Errorf("unknown server subcommand %q", args[0])
	}
}

func runStatusCommand(svc *service.Service, st *store.Store, refresh bool, asJSON bool) error {
	statuses, err := statusesForDisplay(svc, st, refresh)
	if err != nil {
		return err
	}
	statuses = codexStatuses(statuses)
	if asJSON {
		return printJSON(statuses)
	}
	printStatuses(statuses, svc)
	return nil
}

func statusesForDisplay(svc *service.Service, st *store.Store, refresh bool) ([]model.ProfileStatus, error) {
	address, err := ensureDaemonRunning(st)
	if err == nil {
		if refresh {
			statuses, err := postDaemonRefresh(address)
			if err == nil {
				return statuses, nil
			}
			if !errors.Is(err, ErrDaemonUnavailable) {
				return nil, err
			}
		} else {
			statuses, err := fetchDaemonStatuses(address)
			if err == nil {
				return statuses, nil
			}
			if !errors.Is(err, ErrDaemonUnavailable) {
				return nil, err
			}
		}
	}
	return svc.RefreshAll()
}

func runAccountCommand(ctx context.Context, svc *service.Service, st *store.Store, args []string) error {
	if len(args) == 0 {
		return errors.New("account command requires a subcommand")
	}
	switch args[0] {
	case "add":
		return runAccountAdd(ctx, svc, st, args[1:])
	case "list":
		return runAccountList(svc, st, hasJSONFlag(args[1:]))
	case "switch":
		return runAccountSwitch(svc, st, args[1:])
	case "remove", "delete":
		return runAccountRemove(svc, st, args[1:])
	default:
		return fmt.Errorf("unknown account subcommand %q", args[0])
	}
}

func runAccountAdd(ctx context.Context, svc *service.Service, st *store.Store, args []string) error {
	if len(args) > 0 {
		provider := strings.ToLower(strings.TrimSpace(args[0]))
		if provider != "" && provider != model.ToolCodex {
			return fmt.Errorf("unsupported account provider %q", provider)
		}
	}
	profile, err := addCodexAccount(ctx, svc, st)
	if err != nil {
		return err
	}
	_ = tryDaemonRefresh(st)
	fmt.Printf("Added account %s (%s)\n", profileLabelOrID(profile), profile.HomePath)
	return nil
}

func runAccountList(svc *service.Service, st *store.Store, asJSON bool) error {
	statuses, err := statusesForDisplay(svc, st, false)
	if err != nil {
		return err
	}
	statuses = codexStatuses(statuses)
	if asJSON {
		return printJSON(statuses)
	}
	printAccountList(statuses, svc)
	return nil
}

func runAccountSwitch(svc *service.Service, st *store.Store, args []string) error {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return errors.New("usage: codex-lover account switch <profile-id>")
	}
	profileID := strings.TrimSpace(args[0])
	address, err := ensureDaemonRunning(st)
	if err == nil {
		statuses, err := postDaemonSwitch(address, profileID)
		if err == nil {
			fmt.Printf("Switched active account to %s\n\n", profileID)
			printAccountList(codexStatuses(statuses), svc)
			return nil
		}
		if !errors.Is(err, ErrDaemonUnavailable) {
			return err
		}
	}
	if _, err := svc.ActivateProfile(profileID); err != nil {
		return err
	}
	statuses, err := svc.RefreshAll()
	if err != nil {
		return err
	}
	fmt.Printf("Switched active account to %s\n\n", profileID)
	printAccountList(codexStatuses(statuses), svc)
	return nil
}

func runAccountRemove(svc *service.Service, st *store.Store, args []string) error {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return errors.New("usage: codex-lover account remove <profile-id>")
	}
	profileID := strings.TrimSpace(args[0])
	address, err := ensureDaemonRunning(st)
	if err == nil {
		statuses, err := postDaemonRemove(address, profileID)
		if err == nil {
			fmt.Printf("Removed account %s\n\n", profileID)
			printAccountList(codexStatuses(statuses), svc)
			return nil
		}
		if !errors.Is(err, ErrDaemonUnavailable) {
			return err
		}
	}
	if _, err := svc.LogoutProfile(profileID); err != nil {
		return err
	}
	statuses, err := svc.RefreshAll()
	if err != nil {
		return err
	}
	fmt.Printf("Removed account %s\n\n", profileID)
	printAccountList(codexStatuses(statuses), svc)
	return nil
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
		_ = tryDaemonRefresh(st)
		fmt.Printf("Imported profile %s (%s)\n", profileLabelOrID(profile), profile.HomePath)
		return nil
	case "list":
		return runAccountList(svc, st, hasJSONFlag(args[1:]))
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

func addCodexAccount(ctx context.Context, svc *service.Service, st *store.Store) (model.Profile, error) {
	basePath, homePath, err := prepareManagedCodexLoginHome(st.Root())
	if err != nil {
		return model.Profile{}, err
	}
	defer func() {
		_ = os.RemoveAll(basePath)
	}()

	fmt.Println("Add Codex account")
	fmt.Printf("Managed home: %s\n", homePath)
	fmt.Println("Login mode: browser login")

	if err := launchCodexLogin(ctx, basePath, homePath); err != nil {
		return model.Profile{}, err
	}
	profile, err := svc.AddManagedCodexAccount(homePath)
	if err != nil {
		return model.Profile{}, fmt.Errorf("login finished but account import failed: %w", err)
	}
	if _, err := svc.RefreshAll(); err != nil {
		return model.Profile{}, err
	}
	return profile, nil
}

func prepareManagedCodexLoginHome(storeRoot string) (string, string, error) {
	root := filepath.Join(storeRoot, "homes", "codex")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", "", fmt.Errorf("create managed Codex homes root: %w", err)
	}

	var basePath string
	for attempt := 0; attempt < 100; attempt++ {
		name := time.Now().UTC().Format("20060102-150405")
		if attempt > 0 {
			name += "-" + strconv.Itoa(attempt+1)
		}
		candidate := filepath.Join(root, name)
		if _, err := os.Stat(candidate); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return "", "", err
		}
		if err := os.MkdirAll(candidate, 0o700); err != nil {
			return "", "", fmt.Errorf("create managed Codex login home: %w", err)
		}
		basePath = candidate
		break
	}
	if basePath == "" {
		return "", "", errors.New("could not allocate managed Codex login home")
	}

	homePath := filepath.Join(basePath, ".codex")
	if err := os.MkdirAll(homePath, 0o700); err != nil {
		return "", "", fmt.Errorf("create managed Codex auth dir: %w", err)
	}
	return basePath, homePath, nil
}

func launchCodexLogin(ctx context.Context, basePath string, homePath string) error {
	cmdPath, err := resolveCodexLoginCommand()
	if err != nil {
		return err
	}

	fmt.Println("Starting `codex login`...")
	tmpDir := filepath.Join(basePath, "tmp")
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		return fmt.Errorf("create managed Codex temp dir: %w", err)
	}

	// Browser-link login (ported from the main branch): plain `codex login`
	// opens a browser/redirect flow instead of the device-code flow. The
	// `cli_auth_credentials_store=file` override is kept so the credentials are
	// written to auth.json (Linux codex may otherwise use the OS keyring),
	// which is what AddManagedCodexAccount reads back.
	cmd := exec.CommandContext(
		ctx,
		cmdPath,
		"login",
		"-c",
		`cli_auth_credentials_store="file"`,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = codexLoginEnv(basePath, homePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run codex login: %w", err)
	}
	if _, err := os.Stat(filepath.Join(homePath, "auth.json")); err != nil {
		return fmt.Errorf("codex login finished but %s was not created", filepath.Join(homePath, "auth.json"))
	}
	return nil
}

func resolveCodexLoginCommand() (string, error) {
	if path, err := exec.LookPath("codex"); err == nil {
		return path, nil
	}
	return "", errors.New("could not locate Codex CLI")
}

func codexLoginEnv(basePath string, homePath string) []string {
	env := os.Environ()
	tmpDir := filepath.Join(basePath, "tmp")
	env = setEnvValue(env, "HOME", basePath)
	env = setEnvValue(env, "CODEX_HOME", homePath)
	env = setEnvValue(env, "TMPDIR", tmpDir)
	env = setEnvValue(env, "TMP", tmpDir)
	env = setEnvValue(env, "TEMP", tmpDir)
	return env
}

func setEnvValue(env []string, key string, value string) []string {
	prefix := key + "="
	replaced := false
	for i, entry := range env {
		if strings.HasPrefix(strings.ToUpper(entry), strings.ToUpper(prefix)) {
			env[i] = prefix + value
			replaced = true
		}
	}
	if !replaced {
		env = append(env, prefix+value)
	}
	return env
}

func printStatuses(statuses []model.ProfileStatus, svc *service.Service) {
	statuses = codexStatuses(statuses)
	if len(statuses) == 0 {
		fmt.Println("No Codex profiles imported yet.")
		return
	}

	now := time.Now()
	for i, item := range statuses {
		if i > 0 {
			fmt.Println()
		}
		primaryWindow := profileStatusPrimary(item)
		secondaryWindow := profileStatusSecondary(item)
		fmt.Println(profileLabelOrID(item.Profile))
		fmt.Printf("  id: %s\n", item.Profile.ID)
		fmt.Printf("  email: %s\n", emptyDash(item.Profile.Email))
		fmt.Printf("  plan: %s\n", emptyDash(profileStatusPlan(item)))
		fmt.Printf("  auth: %s\n", emptyDash(item.State.AuthStatus))
		fmt.Printf("  freshness: %s\n", profileFreshness(item))
		if item.State.AuthStatus != model.AuthStatusActive && svc != nil {
			fmt.Printf("  switchable: %s\n", yesNo(svc.HasCachedAuth(item.Profile.ID)))
		}
		fmt.Printf("  %s: %s\n", windowDisplayLabel(primaryWindow, "primary"), formatWindowText(primaryWindow, item.State.AuthStatus, now))
		fmt.Printf("  %s: %s\n", windowDisplayLabel(secondaryWindow, "weekly"), formatWindowText(secondaryWindow, item.State.AuthStatus, now))
		fmt.Printf("  credits: %s\n", service.FormatCredits(profileStatusCredits(item)))
		if item.State.LastRefreshedAt != nil {
			fmt.Printf("  refreshed: %s\n", item.State.LastRefreshedAt.Local().Format("2006-01-02 15:04:05"))
		}
		if strings.TrimSpace(item.State.LastError) != "" {
			fmt.Printf("  error: %s\n", item.State.LastError)
		}
	}
}

func printAccountList(statuses []model.ProfileStatus, svc *service.Service) {
	statuses = codexStatuses(statuses)
	if len(statuses) == 0 {
		fmt.Println("No Codex accounts imported yet.")
		return
	}
	now := time.Now()
	for _, item := range statuses {
		marker := " "
		if item.State.AuthStatus == model.AuthStatusActive {
			marker = "*"
		}
		primaryWindow := profileStatusPrimary(item)
		secondaryWindow := profileStatusSecondary(item)
		fmt.Printf("%s %s\n", marker, profileLabelOrID(item.Profile))
		fmt.Printf("  id: %s\n", item.Profile.ID)
		fmt.Printf("  auth: %s\n", emptyDash(item.State.AuthStatus))
		if item.State.AuthStatus != model.AuthStatusActive && svc != nil {
			fmt.Printf("  switchable: %s\n", yesNo(svc.HasCachedAuth(item.Profile.ID)))
		}
		fmt.Printf("  %s: %s\n", windowDisplayLabel(primaryWindow, "primary"), formatWindowText(primaryWindow, item.State.AuthStatus, now))
		fmt.Printf("  %s: %s\n", windowDisplayLabel(secondaryWindow, "weekly"), formatWindowText(secondaryWindow, item.State.AuthStatus, now))
	}
}

func printJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func formatWindowText(window *model.UsageWindow, authStatus string, now time.Time) string {
	if window == nil {
		if authStatus == model.AuthStatusLoggedOut {
			return "no cached usage"
		}
		return "unavailable"
	}

	displayWindow := service.EffectiveWindowForDisplay(window, authStatus, now)
	if displayWindow == nil {
		return "unavailable"
	}

	summary := service.FormatWindowSummary(displayWindow)
	if authStatus == model.AuthStatusLoggedOut {
		if service.WindowResetInferred(window, authStatus, now) {
			return summary + "  reset inferred"
		}
		return summary + "  cached"
	}
	return summary
}

func printUsage() {
	fmt.Println("codex-lover")
	fmt.Println()
	fmt.Println("Run with no command to open the live watch dashboard.")
	fmt.Println()
	fmt.Println("Ubuntu headless commands:")
	fmt.Println("  server run")
	fmt.Println("  server start")
	fmt.Println("  server stop")
	fmt.Println("  server status")
	fmt.Println("  status [--json]")
	fmt.Println("  refresh [--json]")
	fmt.Println("  watch")
	fmt.Println("  account add [codex]")
	fmt.Println("  account list [--json]")
	fmt.Println("  account switch <profile-id>")
	fmt.Println("  account remove <profile-id>")
	fmt.Println("  profile import codex --label NAME --home PATH")
	fmt.Println("  profile list [--json]")
}

func hasJSONFlag(args []string) bool {
	for _, arg := range args {
		if strings.TrimSpace(arg) == "--json" {
			return true
		}
	}
	return false
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

func profileLabelOrID(p model.Profile) string {
	if strings.TrimSpace(p.Email) != "" {
		return p.Email
	}
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

func profileFreshness(item model.ProfileStatus) string {
	if item.State.LastError != "" {
		return "error"
	}
	if item.State.AuthStatus == model.AuthStatusActive {
		return "fresh"
	}
	if item.State.Usage != nil {
		return "cached"
	}
	return "unknown"
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func windowDisplayLabel(window *model.UsageWindow, fallback string) string {
	if window == nil || window.WindowMinutes <= 0 {
		return fallback
	}
	switch window.WindowMinutes {
	case 5 * 60:
		return "5h"
	case 7 * 24 * 60:
		return "weekly"
	}
	if window.WindowMinutes%(24*60) == 0 {
		return fmt.Sprintf("%dd", window.WindowMinutes/(24*60))
	}
	if window.WindowMinutes%60 == 0 {
		return fmt.Sprintf("%dh", window.WindowMinutes/60)
	}
	return fmt.Sprintf("%dm", window.WindowMinutes)
}
