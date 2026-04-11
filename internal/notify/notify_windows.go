//go:build windows

package notify

import (
	"encoding/base64"
	"os/exec"
	"strings"
	"unicode/utf16"
)

func send(event Event) error {
	script := buildWindowsNotifyScript(event)
	encoded := encodePowerShell(script)
	cmd := exec.Command(
		"powershell",
		"-NoProfile",
		"-NonInteractive",
		"-WindowStyle", "Hidden",
		"-ExecutionPolicy", "Bypass",
		"-EncodedCommand", encoded,
	)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}

func buildWindowsNotifyScript(event Event) string {
	icon := "Info"
	switch event.Level {
	case LevelError:
		icon = "Error"
	case LevelWarning:
		icon = "Warning"
	}

	return strings.Join([]string{
		"Add-Type -AssemblyName System.Windows.Forms",
		"Add-Type -AssemblyName System.Drawing",
		"$notify = New-Object System.Windows.Forms.NotifyIcon",
		"$notify.Icon = [System.Drawing.SystemIcons]::Information",
		"$notify.BalloonTipTitle = '" + escapePowerShellSingleQuoted(event.Title) + "'",
		"$notify.BalloonTipText = '" + escapePowerShellSingleQuoted(event.Message) + "'",
		"$notify.BalloonTipIcon = [System.Windows.Forms.ToolTipIcon]::" + icon,
		"$notify.Visible = $true",
		"$notify.ShowBalloonTip(5000)",
		"Start-Sleep -Milliseconds 5500",
		"$notify.Dispose()",
	}, "\n")
}

func escapePowerShellSingleQuoted(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func encodePowerShell(script string) string {
	encoded := utf16.Encode([]rune(script))
	bytes := make([]byte, 0, len(encoded)*2)
	for _, value := range encoded {
		bytes = append(bytes, byte(value), byte(value>>8))
	}
	return base64.StdEncoding.EncodeToString(bytes)
}
