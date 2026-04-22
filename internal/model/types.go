package model

import "time"

const (
	ToolCodex = "codex"
	ToolClaude = "claude"
	ToolKimi   = "kimi"

	AuthStatusUnknown   = "unknown"
	AuthStatusActive    = "active"
	AuthStatusLoggedOut = "logged_out"
	AuthStatusError     = "error"
)

type Config struct {
	Version             int          `json:"version"`
	PollIntervalSeconds int          `json:"poll_interval_seconds"`
	Daemon              DaemonConfig `json:"daemon"`
	Profiles            []Profile    `json:"profiles"`
	AutoRotateCodex     bool         `json:"auto_rotate_codex"`
	AutoRotateThreshold float64      `json:"auto_rotate_threshold"`
}

type DaemonConfig struct {
	ListenAddress string `json:"listen_address"`
}

type Profile struct {
	ID             string    `json:"id"`
	Label          string    `json:"label"`
	Tool           string    `json:"tool"`
	Provider       string    `json:"provider,omitempty"`
	HomePath       string    `json:"home_path"`
	Email          string    `json:"email,omitempty"`
	AccountID      string    `json:"account_id,omitempty"`
	Plan           string    `json:"plan,omitempty"`
	Enabled        bool      `json:"enabled"`
	AutoDiscovered bool      `json:"auto_discovered,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type State struct {
	Version   int                     `json:"version"`
	UpdatedAt time.Time               `json:"updated_at"`
	Profiles  map[string]ProfileState `json:"profiles"`
	Sessions  []Session               `json:"sessions"`
}

type ProfileState struct {
	ProfileID           string         `json:"profile_id"`
	LastRefreshedAt     *time.Time     `json:"last_refreshed_at,omitempty"`
	LastError           string         `json:"last_error,omitempty"`
	Usage               *UsageSnapshot `json:"usage,omitempty"`
	AuthStatus          string         `json:"auth_status,omitempty"`
	AuthFingerprint     string         `json:"-"`
	LastSeenActiveAt    *time.Time     `json:"last_seen_active_at,omitempty"`
	LastSeenLoggedOutAt *time.Time     `json:"last_seen_logged_out_at,omitempty"`
}

type UsageSnapshot struct {
	PlanType         string            `json:"plan_type,omitempty"`
	Primary          *UsageWindow      `json:"primary,omitempty"`
	Secondary        *UsageWindow      `json:"secondary,omitempty"`
	Credits          *CreditsSnapshot  `json:"credits,omitempty"`
	AdditionalLimits []AdditionalLimit `json:"additional_limits,omitempty"`
	CapturedAt       time.Time         `json:"captured_at"`
}

type UsageWindow struct {
	UsedPercent      float64    `json:"used_percent"`
	RemainingPercent float64    `json:"remaining_percent"`
	WindowMinutes    int        `json:"window_minutes"`
	ResetsAt         *time.Time `json:"resets_at,omitempty"`
}

type CreditsSnapshot struct {
	HasCredits bool   `json:"has_credits"`
	Unlimited  bool   `json:"unlimited"`
	Balance    string `json:"balance,omitempty"`
}

type AdditionalLimit struct {
	LimitName string       `json:"limit_name"`
	Primary   *UsageWindow `json:"primary,omitempty"`
	Secondary *UsageWindow `json:"secondary,omitempty"`
}

type Session struct {
	ID        string     `json:"id"`
	Tool      string     `json:"tool"`
	ProfileID string     `json:"profile_id"`
	PID       int        `json:"pid"`
	Command   []string   `json:"command,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	ExitedAt  *time.Time `json:"exited_at,omitempty"`
	Status    string     `json:"status"`
}

type ProfileStatus struct {
	Profile Profile      `json:"profile"`
	State   ProfileState `json:"state"`
}

func (s ProfileState) UsageWindowPrimary() *UsageWindow {
	if s.Usage == nil {
		return nil
	}
	return s.Usage.Primary
}

func (s ProfileState) UsageWindowSecondary() *UsageWindow {
	if s.Usage == nil {
		return nil
	}
	return s.Usage.Secondary
}
