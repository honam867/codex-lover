package app

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

var errNoTTY = errors.New("not an interactive terminal")

// isInteractiveTerminal reports whether stdin is a usable interactive TTY that
// we can drive with stty. Non-TTY input (pipes, redirects) returns false so the
// caller falls back to the read-only dashboard.
func isInteractiveTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	return sttyState() != ""
}

// sttyState returns the current terminal settings (`stty -g`), or "" if stty is
// unavailable or stdin is not a terminal.
func sttyState() string {
	cmd := exec.Command("stty", "-g")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func setStty(args ...string) error {
	cmd := exec.Command("stty", args...)
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// rawArgs puts the terminal into cbreak/no-echo mode with a 0.1s read timeout so
// a single-threaded loop can poll for keys and still tick on a timer.
var rawArgs = []string{"-echo", "-icanon", "-isig", "min", "0", "time", "1"}

// rawTerminal remembers the original terminal settings so they can be restored.
type rawTerminal struct {
	original string
}

// enterRawTerminal switches the terminal into raw polling mode and returns a
// handle used to toggle and restore it. It errors when there is no usable TTY.
func enterRawTerminal() (*rawTerminal, error) {
	original := sttyState()
	if original == "" {
		return nil, errNoTTY
	}
	if err := setStty(rawArgs...); err != nil {
		return nil, err
	}
	// Bypass Go's poller and read raw; ensure stdin blocks for the VTIME window.
	_ = syscall.SetNonblock(int(os.Stdin.Fd()), false)
	return &rawTerminal{original: original}, nil
}

// raw re-applies raw polling mode (used after temporarily restoring for add).
func (r *rawTerminal) raw() error {
	return setStty(rawArgs...)
}

// restore puts the terminal back to its original (cooked) settings.
func (r *rawTerminal) restore() {
	if r == nil {
		return
	}
	if r.original != "" {
		_ = setStty(r.original)
		return
	}
	_ = setStty("sane")
}

// readStdin performs a single raw read of stdin. With the raw VMIN=0/VTIME
// settings it returns after at most ~0.1s, possibly with zero bytes.
func readStdin(buf []byte) (int, error) {
	return syscall.Read(int(os.Stdin.Fd()), buf)
}
