package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"codex-lover/internal/daemon"
	"codex-lover/internal/desktop"
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
		return desktop.Run(ctx, svc)
	}

	switch args[0] {
	case "run":
		return desktop.Run(ctx, svc)
	case "watch":
		return desktop.Run(ctx, svc)
	case "account":
		return runAccountCommand(ctx, svc, st, args[1:])
	case "profile":
		return runProfileCommand(svc, args[1:])
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
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runAccountCommand(ctx context.Context, svc *service.Service, st *store.Store, args []string) error {
	if len(args) == 0 {
		return errors.New("account command requires a subcommand")
	}
	switch args[0] {
	case "add":
		provider := model.ToolCodex
		if len(args) > 1 {
			provider = strings.ToLower(strings.TrimSpace(args[1]))
		}
		profile, err := addAccount(ctx, svc, st, provider)
		if err != nil {
			return err
		}
		fmt.Printf("Added account %s (%s)\n", profileLabelOrID(profile), profile.HomePath)
		return nil
	default:
		return fmt.Errorf("unknown account subcommand %q", args[0])
	}
}

func runProfileCommand(svc *service.Service, args []string) error {
	if len(args) == 0 {
		return errors.New("profile command requires a subcommand")
	}
	switch args[0] {
	case "import":
		if len(args) < 2 || (args[1] != model.ToolCodex && args[1] != model.ToolClaude && args[1] != model.ToolKimi) {
			return errors.New("usage: codex-lover profile import <codex|claude|kimi> --label NAME --home PATH")
		}
		tool := args[1]
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
		var profile model.Profile
		switch tool {
		case model.ToolCodex:
			profile, err = svc.ImportCodexProfile(label, homePath)
		case model.ToolClaude:
			profile, err = svc.ImportClaudeProfile(label, homePath)
		case model.ToolKimi:
			profile, err = svc.ImportKimiProfile(label, homePath)
		default:
			err = fmt.Errorf("unsupported profile import tool %q", tool)
		}
		if err != nil {
			return err
		}
		fmt.Printf("Imported profile %s (%s)\n", profileLabelOrID(profile), profile.HomePath)
		return nil
	case "list":
		statuses, err := svc.ProfileStatuses()
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

func addAccount(ctx context.Context, svc *service.Service, st *store.Store, provider string) (model.Profile, error) {
	switch provider {
	case "", model.ToolCodex:
		return addCodexAccount(ctx, svc, st)
	case model.ToolClaude:
		return addClaudeAccount(ctx, svc, st)
	case model.ToolKimi:
		return addKimiAccount(ctx, svc, st)
	default:
		return model.Profile{}, fmt.Errorf("unsupported account provider %q", provider)
	}
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

	if err := launchCodexLogin(ctx, basePath, homePath); err != nil {
		return model.Profile{}, err
	}
	profile, err := svc.AddManagedCodexAccount(homePath)
	if err != nil {
		return model.Profile{}, fmt.Errorf("login finished but account import failed: %w", err)
	}
	if _, err := svc.ActivateProfile(profile.ID); err != nil {
		return model.Profile{}, fmt.Errorf("account added but auto-activation failed: %w", err)
	}
	if _, err := svc.RefreshAll(); err != nil {
		return model.Profile{}, err
	}
	return profile, nil
}

func addClaudeAccount(ctx context.Context, svc *service.Service, st *store.Store) (model.Profile, error) {
	homePath, err := prepareManagedClaudeLoginHome(st.Root())
	if err != nil {
		return model.Profile{}, err
	}
	defer func() {
		_ = os.RemoveAll(homePath)
	}()

	fmt.Println("Add Claude account")
	fmt.Printf("Managed home: %s\n", homePath)

	if err := launchClaudeLogin(ctx, homePath); err != nil {
		return model.Profile{}, err
	}
	profile, err := svc.AddManagedClaudeAccount(homePath)
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

func prepareManagedClaudeLoginHome(storeRoot string) (string, error) {
	root := filepath.Join(storeRoot, "homes", "claude")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create managed Claude homes root: %w", err)
	}

	var homePath string
	for attempt := 0; attempt < 100; attempt++ {
		name := time.Now().UTC().Format("20060102-150405")
		if attempt > 0 {
			name += "-" + strconv.Itoa(attempt+1)
		}
		candidate := filepath.Join(root, name)
		if _, err := os.Stat(candidate); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if err := os.MkdirAll(candidate, 0o700); err != nil {
			return "", fmt.Errorf("create managed Claude login home: %w", err)
		}
		homePath = candidate
		break
	}
	if homePath == "" {
		return "", errors.New("could not allocate managed Claude login home")
	}
	return homePath, nil
}

func launchCodexLogin(ctx context.Context, basePath string, homePath string) error {
	cmdPath, err := resolveCodexLoginCommand()
	if err != nil {
		return err
	}

	fmt.Println("Starting `codex login`...")
	if err := os.MkdirAll(filepath.Join(basePath, "tmp"), 0o700); err != nil {
		return fmt.Errorf("create managed Codex temp dir: %w", err)
	}

	cmd := exec.CommandContext(ctx, "cmd.exe", "/d", "/c", cmdPath, "login")
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

func launchClaudeLogin(ctx context.Context, homePath string) error {
	cmdPath, err := resolveClaudeLoginCommand()
	if err != nil {
		return err
	}

	fmt.Println("Starting `claude auth login`...")
	cmd := exec.CommandContext(ctx, "cmd.exe", "/d", "/c", cmdPath, "auth", "login", "--claudeai")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = claudeLoginEnv(homePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run claude auth login: %w", err)
	}
	if _, err := os.Stat(filepath.Join(homePath, ".credentials.json")); err != nil {
		return fmt.Errorf("claude auth login finished but %s was not created", filepath.Join(homePath, ".credentials.json"))
	}
	return nil
}

func resolveCodexLoginCommand() (string, error) {
	for _, candidate := range []string{"codex.cmd", "codex"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", errors.New("could not locate Codex CLI")
}

func resolveClaudeLoginCommand() (string, error) {
	for _, candidate := range []string{"claude.exe", "claude"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", errors.New("could not locate Claude CLI")
}

func addKimiAccount(ctx context.Context, svc *service.Service, st *store.Store) (model.Profile, error) {
	homePath, err := prepareManagedKimiLoginHome(st.Root())
	if err != nil {
		return model.Profile{}, err
	}
	defer func() {
		_ = os.RemoveAll(homePath)
	}()

	fmt.Println("Add Kimi account")
	fmt.Printf("Managed home: %s\n", homePath)

	if err := launchKimiLogin(ctx, homePath); err != nil {
		return model.Profile{}, err
	}
	profile, err := svc.AddManagedKimiAccount(homePath)
	if err != nil {
		return model.Profile{}, fmt.Errorf("login finished but account import failed: %w", err)
	}
	if _, err := svc.RefreshAll(); err != nil {
		return model.Profile{}, err
	}
	return profile, nil
}

func prepareManagedKimiLoginHome(storeRoot string) (string, error) {
	root := filepath.Join(storeRoot, "homes", "kimi")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create managed Kimi homes root: %w", err)
	}

	var homePath string
	for attempt := 0; attempt < 100; attempt++ {
		name := time.Now().UTC().Format("20060102-150405")
		if attempt > 0 {
			name += "-" + strconv.Itoa(attempt+1)
		}
		candidate := filepath.Join(root, name)
		if _, err := os.Stat(candidate); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if err := os.MkdirAll(candidate, 0o700); err != nil {
			return "", fmt.Errorf("create managed Kimi login home: %w", err)
		}
		homePath = candidate
		break
	}
	if homePath == "" {
		return "", errors.New("could not allocate managed Kimi login home")
	}
	return homePath, nil
}

func launchKimiLogin(ctx context.Context, homePath string) error {
	cmdPath, err := resolveKimiLoginCommand()
	if err != nil {
		return err
	}

	fmt.Println("Starting `kimi login`...")
	cmd := exec.CommandContext(ctx, "cmd.exe", "/d", "/c", cmdPath, "login")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = kimiLoginEnv(homePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run kimi login: %w", err)
	}
	if _, err := os.Stat(filepath.Join(homePath, "credentials", "kimi-code.json")); err != nil {
		return fmt.Errorf("kimi login finished but %s was not created", filepath.Join(homePath, "credentials", "kimi-code.json"))
	}
	return nil
}

func resolveKimiLoginCommand() (string, error) {
	for _, candidate := range []string{"kimi.exe", "kimi"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", errors.New("could not locate Kimi CLI")
}

func kimiLoginEnv(homePath string) []string {
	env := os.Environ()
	env = setEnvValue(env, "HOME", homePath)
	env = setEnvValue(env, "USERPROFILE", homePath)
	return env
}

func codexLoginEnv(basePath string, homePath string) []string {
	env := os.Environ()
	env = setEnvValue(env, "HOME", basePath)
	env = setEnvValue(env, "USERPROFILE", basePath)
	env = setEnvValue(env, "CODEX_HOME", homePath)
	env = setEnvValue(env, "TEMP", filepath.Join(basePath, "tmp"))
	env = setEnvValue(env, "TMP", filepath.Join(basePath, "tmp"))

	volume := filepath.VolumeName(basePath)
	rest := strings.TrimPrefix(basePath, volume)
	if volume != "" {
		env = setEnvValue(env, "HOMEDRIVE", volume)
	}
	if rest != "" {
		env = setEnvValue(env, "HOMEPATH", rest)
	}
	return env
}

func claudeLoginEnv(homePath string) []string {
	env := os.Environ()
	env = setEnvValue(env, "CLAUDE_CONFIG_DIR", homePath)
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
	if len(statuses) == 0 {
		fmt.Println("No profiles imported yet.")
		return
	}

	now := time.Now()
	for i, item := range statuses {
		if i > 0 {
			fmt.Println()
		}
		fmt.Println(profileLabelOrID(item.Profile))
		fmt.Printf("  email: %s\n", emptyDash(item.Profile.Email))
		fmt.Printf("  plan: %s\n", emptyDash(profileStatusPlan(item)))
		fmt.Printf("  auth: %s\n", emptyDash(item.State.AuthStatus))
		fmt.Printf("  freshness: %s\n", profileFreshness(item))
		fmt.Printf("  5h: %s\n", formatWindowText(profileStatusPrimary(item), item.State.AuthStatus, now))
		fmt.Printf("  weekly: %s\n", formatWindowText(profileStatusSecondary(item), item.State.AuthStatus, now))
		fmt.Printf("  credits: %s\n", service.FormatCredits(profileStatusCredits(item)))
		if item.State.LastRefreshedAt != nil {
			fmt.Printf("  refreshed: %s\n", item.State.LastRefreshedAt.Local().Format("2006-01-02 15:04:05"))
		}
		if strings.TrimSpace(item.State.LastError) != "" {
			fmt.Printf("  error: %s\n", item.State.LastError)
		}
	}
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
	fmt.Println("Commands:")
	fmt.Println("  run")
	fmt.Println("  watch")
	fmt.Println("  account add [codex|claude|kimi]")
	fmt.Println("  profile import <codex|claude|kimi> --label NAME --home PATH")
	fmt.Println("  profile list")
	fmt.Println("  refresh")
	fmt.Println("  status")
	fmt.Println("  daemon")
	fmt.Println("  daemon-status")
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
