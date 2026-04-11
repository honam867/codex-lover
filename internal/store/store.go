package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"codex-lover/internal/model"
)

type Store struct {
	root       string
	configPath string
	statePath  string
	mu         sync.Mutex
}

func New() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user home: %w", err)
	}
	root := filepath.Join(home, ".codex-lover")
	return &Store{
		root:       root,
		configPath: filepath.Join(root, "config.json"),
		statePath:  filepath.Join(root, "state.json"),
	}, nil
}

func (s *Store) Ensure() error {
	return os.MkdirAll(s.root, 0o755)
}

func (s *Store) Root() string {
	return s.root
}

func DefaultConfig() model.Config {
	return model.Config{
		Version:             1,
		PollIntervalSeconds: 30,
		Daemon: model.DaemonConfig{
			ListenAddress: "127.0.0.1:47070",
		},
		Profiles: []model.Profile{},
	}
}

func DefaultState() model.State {
	return model.State{
		Version:   1,
		UpdatedAt: time.Now().UTC(),
		Profiles:  map[string]model.ProfileState{},
		Sessions:  []model.Session{},
	}
}

func (s *Store) LoadConfig() (model.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadConfigUnlocked()
}

func (s *Store) SaveConfig(cfg model.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveJSONUnlocked(s.configPath, cfg)
}

func (s *Store) LoadState() (model.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadStateUnlocked()
}

func (s *Store) SaveState(state model.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state.UpdatedAt = time.Now().UTC()
	return s.saveJSONUnlocked(s.statePath, state)
}

func (s *Store) UpsertProfile(profile model.Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.loadConfigUnlocked()
	if err != nil {
		return err
	}
	found := false
	for i := range cfg.Profiles {
		if cfg.Profiles[i].ID == profile.ID {
			cfg.Profiles[i] = profile
			found = true
			break
		}
	}
	if !found {
		cfg.Profiles = append(cfg.Profiles, profile)
	}
	return s.saveJSONUnlocked(s.configPath, cfg)
}

func (s *Store) UpdateProfileState(profileID string, state model.ProfileState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.loadStateUnlocked()
	if err != nil {
		return err
	}
	if current.Profiles == nil {
		current.Profiles = map[string]model.ProfileState{}
	}
	current.Profiles[profileID] = state
	current.UpdatedAt = time.Now().UTC()
	return s.saveJSONUnlocked(s.statePath, current)
}

func (s *Store) ProfileStatuses() ([]model.ProfileStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.loadConfigUnlocked()
	if err != nil {
		return nil, err
	}
	state, err := s.loadStateUnlocked()
	if err != nil {
		return nil, err
	}

	out := make([]model.ProfileStatus, 0, len(cfg.Profiles))
	for _, profile := range cfg.Profiles {
		out = append(out, model.ProfileStatus{
			Profile: profile,
			State:   state.Profiles[profile.ID],
		})
	}
	sort.SliceStable(out, func(i int, j int) bool {
		left := out[i]
		right := out[j]
		if left.Profile.HomePath != right.Profile.HomePath {
			return left.Profile.HomePath < right.Profile.HomePath
		}
		leftRank := authStatusRank(left.State.AuthStatus)
		rightRank := authStatusRank(right.State.AuthStatus)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		leftLabel := strings.ToLower(strings.TrimSpace(left.Profile.Email + " " + left.Profile.Label))
		rightLabel := strings.ToLower(strings.TrimSpace(right.Profile.Email + " " + right.Profile.Label))
		return leftLabel < rightLabel
	})
	return out, nil
}

func (s *Store) loadConfigUnlocked() (model.Config, error) {
	var cfg model.Config
	err := s.loadJSONUnlocked(s.configPath, &cfg)
	if errors.Is(err, os.ErrNotExist) {
		cfg = DefaultConfig()
		if err := s.saveJSONUnlocked(s.configPath, cfg); err != nil {
			return model.Config{}, err
		}
		return cfg, nil
	}
	return cfg, err
}

func (s *Store) loadStateUnlocked() (model.State, error) {
	var state model.State
	err := s.loadJSONUnlocked(s.statePath, &state)
	if errors.Is(err, os.ErrNotExist) {
		state = DefaultState()
		if err := s.saveJSONUnlocked(s.statePath, state); err != nil {
			return model.State{}, err
		}
		return state, nil
	}
	if state.Profiles == nil {
		state.Profiles = map[string]model.ProfileState{}
	}
	return state, err
}

func (s *Store) loadJSONUnlocked(path string, target any) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, target)
}

func (s *Store) saveJSONUnlocked(path string, value any) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	return os.WriteFile(path, data, 0o644)
}

func authStatusRank(value string) int {
	switch value {
	case model.AuthStatusActive:
		return 0
	case model.AuthStatusLoggedOut:
		return 1
	case model.AuthStatusError:
		return 2
	default:
		return 3
	}
}
