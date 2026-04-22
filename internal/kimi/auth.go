package kimi

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"codex-lover/internal/model"
)

const (
	authFileName     = "credentials/kimi-code.json"
	defaultUsageURL  = "https://api.kimi.com/coding/v1/usages"
	defaultUserAgent = "codex-lover"
)

type AuthFile struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	ExpiresAt    float64 `json:"expires_at"`
	Scope        string  `json:"scope"`
	TokenType    string  `json:"token_type"`
	ExpiresIn    float64 `json:"expires_in"`
}

type ProfileAuth struct {
	HomePath     string
	AccessToken  string
	RefreshToken string
	AccountID    string
	Email        string
	Plan         string
}

func LoadProfileAuth(homePath string) (*ProfileAuth, error) {
	authPath := filepath.Join(homePath, authFileName)
	data, err := os.ReadFile(authPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", authPath, err)
	}

	var authFile AuthFile
	if err := json.Unmarshal(data, &authFile); err != nil {
		return nil, fmt.Errorf("parse %s: %w", authPath, err)
	}
	if authFile.AccessToken == "" {
		return nil, fmt.Errorf("%s does not contain Kimi access token", authPath)
	}

	accountID := extractAccountIDFromToken(authFile.AccessToken)

	return &ProfileAuth{
		HomePath:     homePath,
		AccessToken:  authFile.AccessToken,
		RefreshToken: authFile.RefreshToken,
		AccountID:    accountID,
		Email:        "", // Kimi token doesn't contain email in JWT
		Plan:         "", // Will be filled from usage response
	}, nil
}

func extractAccountIDFromToken(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := decodeBase64(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		UserID string `json:"user_id"`
		Sub    string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	if claims.UserID != "" {
		return claims.UserID
	}
	return claims.Sub
}

func decodeBase64(s string) ([]byte, error) {
	// JWT uses base64url encoding without padding
	padding := 4 - len(s)%4
	if padding != 4 {
		s += strings.Repeat("=", padding)
	}
	// Try base64url first (standard JWT encoding)
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	return base64.StdEncoding.DecodeString(s)
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
		auth.AccessToken,
		auth.RefreshToken,
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func ProfileFromAuth(label string, homePath string, auth *ProfileAuth) model.Profile {
	now := time.Now().UTC()
	return model.Profile{
		ID:        profileID(model.ToolKimi, label, auth.AccountID, homePath),
		Label:     label,
		Tool:      model.ToolKimi,
		Provider:  model.ToolKimi,
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
	label := observedProfileLabel(auth)
	return model.Profile{
		ID:             observedProfileID(homePath, auth),
		Label:          label,
		Tool:           model.ToolKimi,
		Provider:       model.ToolKimi,
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

func profileID(tool string, label string, accountID string, homePath string) string {
	base := label
	if base == "" {
		base = accountID
	}
	if base == "" {
		base = filepath.Base(homePath)
	}
	base = strings.ToLower(strings.TrimSpace(base))
	base = strings.ReplaceAll(base, " ", "-")
	base = strings.ReplaceAll(base, "_", "-")
	base = strings.ReplaceAll(base, string(os.PathSeparator), "-")
	return tool + "-" + base
}

func observedProfileID(homePath string, auth *ProfileAuth) string {
	if auth != nil {
		if auth.AccountID != "" {
			return profileID(model.ToolKimi, "", normalizeObservedLabel(auth.AccountID), homePath)
		}
		if auth.Email != "" {
			return profileID(model.ToolKimi, "", normalizeObservedLabel(auth.Email), homePath)
		}
	}
	return profileID(model.ToolKimi, "", "", homePath)
}

func observedProfileLabel(auth *ProfileAuth) string {
	if auth == nil {
		return "observed-account"
	}
	if auth.Email != "" {
		return normalizeObservedLabel(auth.Email)
	}
	if auth.AccountID != "" {
		return normalizeObservedLabel(auth.AccountID)
	}
	return "observed-account"
}

func normalizeObservedLabel(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "observed-account"
	}
	re := regexp.MustCompile(`[^a-z0-9]+`)
	value = re.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "observed-account"
	}
	return value
}
