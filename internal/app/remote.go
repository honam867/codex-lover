package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"codex-lover/internal/model"
	"codex-lover/internal/store"
)

var ErrDaemonUnavailable = errors.New("daemon unavailable")

var daemonHTTPClient = &http.Client{Timeout: 5 * time.Second}

func daemonAddress(st *store.Store) (string, error) {
	cfg, err := st.LoadConfig()
	if err != nil {
		return "", err
	}
	address := strings.TrimSpace(cfg.Daemon.ListenAddress)
	if address == "" {
		return "", errors.New("daemon listen address is not configured")
	}
	return address, nil
}

func ensureDaemonRunning(st *store.Store) (string, error) {
	address, err := daemonAddress(st)
	if err != nil {
		return "", err
	}
	if _, err := fetchDaemonStatuses(address); err == nil {
		return address, nil
	} else if !isDaemonUnavailable(err) {
		return "", err
	}
	if err := startManagedServer(st); err != nil {
		return "", err
	}
	return address, nil
}

func fetchDaemonStatuses(address string) ([]model.ProfileStatus, error) {
	var statuses []model.ProfileStatus
	err := doDaemonJSONRequest(http.MethodGet, daemonURL(address, "/v1/status"), nil, &statuses)
	return statuses, err
}

func postDaemonRefresh(address string) ([]model.ProfileStatus, error) {
	var statuses []model.ProfileStatus
	err := doDaemonJSONRequest(http.MethodPost, daemonURL(address, "/v1/refresh"), map[string]any{}, &statuses)
	return statuses, err
}

func postDaemonSwitch(address string, profileID string) ([]model.ProfileStatus, error) {
	var statuses []model.ProfileStatus
	err := doDaemonJSONRequest(http.MethodPost, daemonURL(address, "/v1/accounts/switch"), map[string]string{"profile_id": profileID}, &statuses)
	return statuses, err
}

func postDaemonRemove(address string, profileID string) ([]model.ProfileStatus, error) {
	var statuses []model.ProfileStatus
	err := doDaemonJSONRequest(http.MethodPost, daemonURL(address, "/v1/accounts/remove"), map[string]string{"profile_id": profileID}, &statuses)
	return statuses, err
}

func tryDaemonRefresh(st *store.Store) error {
	address, err := ensureDaemonRunning(st)
	if err != nil {
		return err
	}
	_, err = postDaemonRefresh(address)
	return err
}

func daemonURL(address string, path string) string {
	return "http://" + address + path
}

func doDaemonJSONRequest(method string, url string, payload any, target any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := daemonHTTPClient.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDaemonUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		body, _ := io.ReadAll(response.Body)
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = response.Status
		}
		return fmt.Errorf("daemon returned %s: %s", response.Status, message)
	}
	if target == nil {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return err
	}
	return nil
}
