package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"codex-lover/internal/service"
)

const desktopExecutableName = "codex-lover-desktop.exe"

func Run(ctx context.Context, svc *service.Service) error {
	_ = ctx
	_ = svc

	exePath, err := resolveDesktopExecutable()
	if err != nil {
		return err
	}

	cmd := exec.Command(exePath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch desktop app: %w", err)
	}
	return nil
}

func resolveDesktopExecutable() (string, error) {
	candidates := []string{}

	if currentExe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(currentExe), desktopExecutableName))
	}

	if _, file, _, ok := runtime.Caller(0); ok {
		repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
		candidates = append(candidates,
			filepath.Join(repoRoot, "desktop-app", "build", "bin", desktopExecutableName),
			filepath.Join(repoRoot, "build", "bin", desktopExecutableName),
		)
	}

	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "desktop-app", "build", "bin", desktopExecutableName),
			filepath.Join(cwd, "build", "bin", desktopExecutableName),
		)
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", errors.New("desktop app is not built yet. Run .\\install.ps1 first, then try `codex-lover run` again")
}
