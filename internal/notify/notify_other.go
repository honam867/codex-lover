//go:build !windows

package notify

import "os/exec"

// notifySendArgs builds the notify-send argument list for an event, mapping the
// event level onto notify-send's urgency levels.
func notifySendArgs(event Event) []string {
	urgency := "normal"
	switch event.Level {
	case LevelError:
		urgency = "critical"
	case LevelInfo:
		urgency = "low"
	}
	title := event.Title
	if title == "" {
		title = "codex-lover"
	}
	return []string{"-a", "codex-lover", "-u", urgency, title, event.Message}
}

func send(event Event) error {
	path, err := exec.LookPath("notify-send")
	if err != nil {
		// notify-send (libnotify-bin) is not installed; skip silently.
		return nil
	}
	cmd := exec.Command(path, notifySendArgs(event)...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
