package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// LoginLinkResult carries the Codex login URL for the "copy link" flow.
type LoginLinkResult struct {
	URL   string `json:"url"`
	Error string `json:"error"`
}

var codexLoginURLPattern = regexp.MustCompile(`https://auth\.openai\.com/\S+`)

// StartCodexLoginLink runs the Codex add-account login hidden in the background
// and returns its login URL, so it can be opened in any browser (an incognito
// tab, another profile). The login server keeps waiting on localhost until the
// login finishes; the CLI then imports the account like the console flow.
func (a *App) StartCodexLoginLink() LoginLinkResult {
	cliPath, err := resolveCLIExecutable()
	if err != nil {
		return LoginLinkResult{Error: err.Error()}
	}

	// Only one login server can hold its localhost port at a time.
	a.mu.Lock()
	previous := a.loginLinkCmd
	a.loginLinkCmd = nil
	a.mu.Unlock()
	killProcessTree(previous)

	reader, writer, err := os.Pipe()
	if err != nil {
		return LoginLinkResult{Error: err.Error()}
	}
	cmd := exec.Command(cliPath, "account", "add", "codex")
	cmd.Stdout = writer
	cmd.Stderr = writer
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if err := cmd.Start(); err != nil {
		reader.Close()
		writer.Close()
		return LoginLinkResult{Error: fmt.Sprintf("start Codex login: %v", err)}
	}
	writer.Close()

	a.mu.Lock()
	a.loginLinkCmd = cmd
	a.mu.Unlock()

	found := make(chan string, 1)
	go func() {
		defer reader.Close()
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		sent := false
		for scanner.Scan() { // keep draining so the login never blocks on a full pipe
			if url := codexLoginURLPattern.FindString(scanner.Text()); url != "" && !sent {
				found <- url
				sent = true
			}
		}
	}()
	go func() {
		_ = cmd.Wait()
		a.mu.Lock()
		if a.loginLinkCmd == cmd {
			a.loginLinkCmd = nil
		}
		a.mu.Unlock()
		_, _ = a.snapshot(false) // pick up the imported account
	}()

	select {
	case url := <-found:
		return LoginLinkResult{URL: url}
	case <-time.After(30 * time.Second):
		killProcessTree(cmd)
		return LoginLinkResult{Error: "Codex login did not print a login link within 30s"}
	}
}

// killProcessTree stops the CLI and the codex/node children it started, which
// would otherwise keep the login port busy.
func killProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	_ = kill.Run()
}
