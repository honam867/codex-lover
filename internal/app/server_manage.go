package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"codex-lover/internal/store"
)

const daemonReadyTimeout = 10 * time.Second

func startManagedServer(st *store.Store) error {
	address, err := daemonAddress(st)
	if err != nil {
		return err
	}
	if _, err := fetchDaemonStatuses(address); err == nil {
		if pid, lookupErr := managedServerPID(st); lookupErr != nil || pid <= 0 || !processRunning(pid) {
			if pid, findErr := findManagedServerPID(); findErr == nil && pid > 0 {
				_ = writeManagedServerPID(st, pid)
			}
		}
		fmt.Printf("codex-lover server already running on http://%s\n", address)
		return nil
	} else if err != nil && !isDaemonUnavailable(err) {
		return err
	}

	pid, _ := managedServerPID(st)
	if pid > 0 && processRunning(pid) {
		if err := waitForDaemonReady(address, 2*time.Second); err == nil {
			fmt.Printf("codex-lover server already running on http://%s\n", address)
			return nil
		}
	}
	_ = os.Remove(serverPIDPath(st))

	if err := os.MkdirAll(filepath.Dir(serverLogPath(st)), 0o755); err != nil {
		return err
	}
	logFile, err := os.OpenFile(serverLogPath(st), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open server log: %w", err)
	}
	defer logFile.Close()

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return fmt.Errorf("open /dev/null: %w", err)
	}
	defer devNull.Close()

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve codex-lover executable: %w", err)
	}
	cmd := exec.Command(exePath, "server", "run")
	cmd.Stdin = devNull
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start managed server: %w", err)
	}
	if err := writeManagedServerPID(st, cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("write server pid file: %w", err)
	}
	if err := waitForDaemonReady(address, daemonReadyTimeout); err != nil {
		return err
	}
	fmt.Printf("Started codex-lover server on http://%s (pid %d)\n", address, cmd.Process.Pid)
	return nil
}

func stopManagedServer(st *store.Store) error {
	pid, err := managedServerPID(st)
	if err != nil || pid <= 0 {
		if discoveredPID, findErr := findManagedServerPID(); findErr == nil && discoveredPID > 0 {
			pid = discoveredPID
			_ = writeManagedServerPID(st, pid)
		} else {
			_ = os.Remove(serverPIDPath(st))
			fmt.Println("codex-lover server is not running")
			return nil
		}
	}
	if !processRunning(pid) {
		_ = os.Remove(serverPIDPath(st))
		fmt.Println("codex-lover server is not running")
		return nil
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("stop managed server: %w", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !processRunning(pid) {
			_ = os.Remove(serverPIDPath(st))
			fmt.Println("Stopped codex-lover server")
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out stopping server pid %d", pid)
}

func printServerStatus(st *store.Store) error {
	address, err := daemonAddress(st)
	if err != nil {
		return err
	}
	pid, _ := managedServerPID(st)
	running := false
	if _, err := fetchDaemonStatuses(address); err == nil {
		running = true
	}
	status := "stopped"
	if running {
		status = "running"
		if pid <= 0 || !processRunning(pid) {
			if discoveredPID, findErr := findManagedServerPID(); findErr == nil && discoveredPID > 0 {
				pid = discoveredPID
				_ = writeManagedServerPID(st, pid)
			}
		}
	}
	fmt.Printf("server: %s\n", status)
	fmt.Printf("address: http://%s\n", address)
	if pid > 0 && processRunning(pid) {
		fmt.Printf("pid: %d\n", pid)
	}
	fmt.Printf("log: %s\n", serverLogPath(st))
	return nil
}

func waitForDaemonReady(address string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if _, err := fetchDaemonStatuses(address); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(200 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("wait for daemon ready: %w", lastErr)
	}
	return fmt.Errorf("wait for daemon ready timed out")
}

func managedServerPID(st *store.Store) (int, error) {
	data, err := os.ReadFile(serverPIDPath(st))
	if err != nil {
		return 0, err
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return 0, fmt.Errorf("empty server pid file")
	}
	return strconv.Atoi(value)
}

func registerManagedServerPID(st *store.Store) (func(), error) {
	pid := os.Getpid()
	if err := writeManagedServerPID(st, pid); err != nil {
		return nil, err
	}
	return func() {
		current, err := managedServerPID(st)
		if err == nil && current == pid {
			_ = os.Remove(serverPIDPath(st))
		}
	}, nil
}

func writeManagedServerPID(st *store.Store, pid int) error {
	return os.WriteFile(serverPIDPath(st), []byte(strconv.Itoa(pid)+"\n"), 0o644)
}

func findManagedServerPID() (int, error) {
	cmd := exec.Command("pgrep", "-f", "codex-lover server run")
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil || pid <= 0 {
			continue
		}
		if pid == os.Getpid() {
			continue
		}
		if processRunning(pid) {
			return pid, nil
		}
	}
	return 0, fmt.Errorf("managed server process not found")
}

func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil
}

func serverPIDPath(st *store.Store) string {
	return filepath.Join(st.Root(), "server.pid")
}

func serverLogPath(st *store.Store) string {
	return filepath.Join(st.Root(), "logs", "server.log")
}

func isDaemonUnavailable(err error) bool {
	return err != nil && strings.Contains(err.Error(), ErrDaemonUnavailable.Error())
}
