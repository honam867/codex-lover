package claude

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"codex-lover/internal/model"
)

const (
	authFileName          = ".credentials.json"
	defaultUsageURL       = "https://api.anthropic.com/api/oauth/usage"
	defaultProfileURL     = "https://api.anthropic.com/api/oauth/profile"
	defaultOAuthBeta      = "oauth-2025-04-20"
	defaultUserAgent      = "codex-lover"
	defaultWindowMinutes5 = 5 * 60
	defaultWindowMinutes7 = 7 * 24 * 60
)

type authFile struct {
	ClaudeAIOAuth *oauthTokenData `json:"claudeAiOauth"`
}

type oauthTokenData struct {
	AccessToken      string   `json:"accessToken"`
	RefreshToken     string   `json:"refreshToken"`
	ExpiresAt        int64    `json:"expiresAt"`
	Scopes           []string `json:"scopes"`
	SubscriptionType string   `json:"subscriptionType"`
	RateLimitTier    string   `json:"rateLimitTier"`
}

type profilePayload struct {
	Account      *profileAccount      `json:"account"`
	Organization *profileOrganization `json:"organization"`
	Application  *profileApplication  `json:"application"`
}

type profileAccount struct {
	UUID        string `json:"uuid"`
	FullName    string `json:"full_name"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	HasClaudeMax bool  `json:"has_claude_max"`
	HasClaudePro bool  `json:"has_claude_pro"`
}

type profileOrganization struct {
	UUID          string `json:"uuid"`
	Name          string `json:"name"`
	OrganizationType string `json:"organization_type"`
	BillingType   string `json:"billing_type"`
	RateLimitTier string `json:"rate_limit_tier"`
}

type profileApplication struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type ProfileAuth struct {
	HomePath       string
	AccessToken    string
	RefreshToken   string
	AccountID      string
	Email          string
	Plan           string
	RateLimitTier  string
	AuthMethod     string
	APIProvider    string
	OAuthExpiresAt *time.Time
}

func LoadProfileAuth(homePath string) (*ProfileAuth, error) {
	authPath := filepath.Join(homePath, authFileName)
	data, err := os.ReadFile(authPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", authPath, err)
	}

	var parsed authFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse %s: %w", authPath, err)
	}
	if parsed.ClaudeAIOAuth == nil || strings.TrimSpace(parsed.ClaudeAIOAuth.AccessToken) == "" {
		return nil, fmt.Errorf("%s does not contain Claude OAuth tokens", authPath)
	}

	profileInfo, err := loadProfile(parsed.ClaudeAIOAuth.AccessToken)
	if err != nil {
		profileInfo = profilePayload{}
	}

	profile := profileAuthFromFile(homePath, parsed.ClaudeAIOAuth, profileInfo)
	return &profile, nil
}

func DeleteHomeAuth(homePath string) error {
	authPath := filepath.Join(homePath, authFileName)
	if err := os.Remove(authPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete %s: %w", authPath, err)
	}
	return nil
}

func AuthFingerprint(auth *ProfileAuth) string {
	if auth == nil {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{
		auth.AccountID,
		auth.Email,
		auth.AccessToken,
		auth.RefreshToken,
		auth.Plan,
		auth.RateLimitTier,
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func ProfileFromAuth(label string, homePath string, auth *ProfileAuth) model.Profile {
	now := time.Now().UTC()
	return model.Profile{
		ID:        profileID(model.ToolClaude, label, auth.AccountID, auth.Email, homePath),
		Label:     label,
		Tool:      model.ToolClaude,
		Provider:  model.ToolClaude,
		HomePath:  homePath,
		Email:     auth.Email,
		AccountID: auth.AccountID,
		Plan:      auth.Plan,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func ObservedProfileFromAuth(homePath string, auth *ProfileAuth) model.Profile {
	now := time.Now().UTC()
	return model.Profile{
		ID:             observedProfileID(homePath, auth),
		Label:          observedProfileLabel(auth),
		Tool:           model.ToolClaude,
		Provider:       model.ToolClaude,
		HomePath:       homePath,
		Email:          auth.Email,
		AccountID:      auth.AccountID,
		Plan:           auth.Plan,
		Enabled:        true,
		AutoDiscovered: true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func LoadCachedProfileAuth(cacheRoot string, profileID string) (*ProfileAuth, error) {
	authPath := cachedAuthPath(cacheRoot, profileID)
	data, err := os.ReadFile(authPath)
	if err != nil {
		return nil, fmt.Errorf("read cached Claude auth: %w", err)
	}

	var parsed authFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse cached Claude auth: %w", err)
	}
	if parsed.ClaudeAIOAuth == nil || strings.TrimSpace(parsed.ClaudeAIOAuth.AccessToken) == "" {
		return nil, fmt.Errorf("%s does not contain Claude OAuth tokens", authPath)
	}

	profile := profileAuthFromFile(filepath.Dir(authPath), parsed.ClaudeAIOAuth, profilePayload{})
	return &profile, nil
}

func profileAuthFromFile(homePath string, oauth *oauthTokenData, profile profilePayload) ProfileAuth {
	var expiresAt *time.Time
	if oauth.ExpiresAt > 0 {
		value := time.UnixMilli(oauth.ExpiresAt).UTC()
		expiresAt = &value
	}

	accountID := ""
	email := ""
	authMethod := "claude.ai"
	apiProvider := "firstParty"
	plan := strings.TrimSpace(oauth.SubscriptionType)
	rateLimitTier := strings.TrimSpace(oauth.RateLimitTier)
	if profile.Account != nil {
		email = strings.TrimSpace(profile.Account.Email)
	}
	if profile.Organization != nil {
		accountID = strings.TrimSpace(profile.Organization.UUID)
		rateLimitTier = chooseNonEmpty(strings.TrimSpace(profile.Organization.RateLimitTier), rateLimitTier)
		if plan == "" {
			switch {
			case profile.Account != nil && profile.Account.HasClaudeMax:
				plan = "max"
			case profile.Account != nil && profile.Account.HasClaudePro:
				plan = "pro"
			case strings.TrimSpace(profile.Organization.OrganizationType) != "":
				plan = strings.TrimSpace(profile.Organization.OrganizationType)
			}
		}
	}
	return ProfileAuth{
		HomePath:       homePath,
		AccessToken:    strings.TrimSpace(oauth.AccessToken),
		RefreshToken:   strings.TrimSpace(oauth.RefreshToken),
		AccountID:      accountID,
		Email:          email,
		Plan:           plan,
		RateLimitTier:  rateLimitTier,
		AuthMethod:     authMethod,
		APIProvider:    apiProvider,
		OAuthExpiresAt: expiresAt,
	}
}

func loadProfile(accessToken string) (profilePayload, error) {
	req, err := http.NewRequest(http.MethodGet, defaultProfileURL, nil)
	if err != nil {
		return profilePayload{}, fmt.Errorf("build Claude profile request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("anthropic-beta", defaultOAuthBeta)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return profilePayload{}, fmt.Errorf("request Claude profile: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return profilePayload{}, fmt.Errorf("read Claude profile response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return profilePayload{}, fmt.Errorf("Claude profile request failed with %d: %s", resp.StatusCode, string(body))
	}

	var payload profilePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return profilePayload{}, fmt.Errorf("parse Claude profile payload: %w", err)
	}
	return payload, nil
}

func resolveClaudeCLI() (string, error) {
	for _, candidate := range []string{"claude.exe", "claude"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("could not locate Claude CLI")
}

func claudeConfigEnv(homePath string) []string {
	env := os.Environ()
	return setEnvValue(env, "CLAUDE_CONFIG_DIR", homePath)
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

func profileID(tool string, label string, accountID string, email string, homePath string) string {
	base := label
	if strings.TrimSpace(base) == "" {
		base = accountID
	}
	if strings.TrimSpace(base) == "" {
		base = email
	}
	if strings.TrimSpace(base) == "" {
		base = filepath.Base(homePath)
	}
	base = normalizeObservedLabel(base)
	return tool + "-" + base
}

func observedProfileID(homePath string, auth *ProfileAuth) string {
	if auth != nil {
		if strings.TrimSpace(auth.AccountID) != "" {
			return profileID(model.ToolClaude, "", auth.AccountID, "", homePath)
		}
		if strings.TrimSpace(auth.Email) != "" {
			return profileID(model.ToolClaude, "", "", auth.Email, homePath)
		}
	}
	return profileID(model.ToolClaude, "", "", "", homePath)
}

func observedProfileLabel(auth *ProfileAuth) string {
	if auth == nil {
		return "observed-claude-account"
	}
	if strings.TrimSpace(auth.Email) != "" {
		return normalizeObservedLabel(auth.Email)
	}
	if strings.TrimSpace(auth.AccountID) != "" {
		return normalizeObservedLabel(auth.AccountID)
	}
	return "observed-claude-account"
}

func normalizeObservedLabel(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "observed-claude-account"
	}
	re := regexp.MustCompile(`[^a-z0-9]+`)
	value = re.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "observed-claude-account"
	}
	return value
}

func chooseNonEmpty(primary string, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}
