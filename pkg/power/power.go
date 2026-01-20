package power

import (
	"context"
	"log/slog"
	"os/exec"
	"runtime"
)

// Inhibitor prevents the system from going to sleep
type Inhibitor struct {
	logger  *slog.Logger
	cancel  context.CancelFunc
	cmd     *exec.Cmd
	running bool
}

// NewInhibitor creates a new sleep inhibitor
func NewInhibitor(logger *slog.Logger) *Inhibitor {
	return &Inhibitor{
		logger: logger,
	}
}

// Start prevents the system from sleeping
func (i *Inhibitor) Start() error {
	if i.running {
		return nil
	}

	switch runtime.GOOS {
	case "darwin":
		return i.startMacOS()
	case "windows":
		return i.startWindows()
	case "linux":
		return i.startLinux()
	default:
		i.logger.Warn("Sleep inhibition not supported on this OS", slog.String("os", runtime.GOOS))
		return nil
	}
}

// Stop allows the system to sleep again
func (i *Inhibitor) Stop() {
	if !i.running {
		return
	}

	if i.cancel != nil {
		i.cancel()
	}

	if i.cmd != nil && i.cmd.Process != nil {
		i.cmd.Process.Kill()
		i.cmd.Wait()
	}

	i.running = false
	i.logger.Info("Sleep inhibition stopped")
}

// startMacOS uses caffeinate to prevent sleep
func (i *Inhibitor) startMacOS() error {
	// caffeinate -i: prevent idle sleep
	// caffeinate -d: prevent display sleep
	ctx, cancel := context.WithCancel(context.Background())
	i.cancel = cancel

	i.cmd = exec.CommandContext(ctx, "caffeinate", "-i", "-d")
	if err := i.cmd.Start(); err != nil {
		cancel()
		return err
	}

	i.running = true
	i.logger.Info("Sleep inhibition started", slog.String("os", "macOS"), slog.String("method", "caffeinate"))
	return nil
}

// startWindows uses PowerShell to prevent sleep
func (i *Inhibitor) startWindows() error {
	ctx, cancel := context.WithCancel(context.Background())
	i.cancel = cancel

	// PowerShell script that keeps running and prevents sleep
	script := `
Add-Type -TypeDefinition @"
using System;
using System.Runtime.InteropServices;
public class PowerState {
    [DllImport("kernel32.dll")]
    public static extern uint SetThreadExecutionState(uint esFlags);
}
"@
[PowerState]::SetThreadExecutionState(0x80000003)
while ($true) {
    Start-Sleep -Seconds 30
    [PowerState]::SetThreadExecutionState(0x80000003)
}
`

	i.cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	if err := i.cmd.Start(); err != nil {
		cancel()
		return err
	}

	i.running = true
	i.logger.Info("Sleep inhibition started", slog.String("os", "windows"), slog.String("method", "SetThreadExecutionState"))
	return nil
}

// startLinux tries multiple methods to prevent sleep
func (i *Inhibitor) startLinux() error {
	ctx, cancel := context.WithCancel(context.Background())
	i.cancel = cancel

	// Try systemd-inhibit first (most common on modern Linux)
	if path, err := exec.LookPath("systemd-inhibit"); err == nil {
		i.cmd = exec.CommandContext(ctx, path, "--what=idle:sleep", "--who=instagram-unfollow", "--why=Unfollow automation running", "sleep", "infinity")
		if err := i.cmd.Start(); err == nil {
			i.running = true
			i.logger.Info("Sleep inhibition started", slog.String("os", "linux"), slog.String("method", "systemd-inhibit"))
			return nil
		}
	}

	// Try gnome-session-inhibit
	if path, err := exec.LookPath("gnome-session-inhibit"); err == nil {
		i.cmd = exec.CommandContext(ctx, path, "--inhibit=idle:suspend", "--reason=Unfollow automation running", "sleep", "infinity")
		if err := i.cmd.Start(); err == nil {
			i.running = true
			i.logger.Info("Sleep inhibition started", slog.String("os", "linux"), slog.String("method", "gnome-session-inhibit"))
			return nil
		}
	}

	cancel()
	i.logger.Warn("No sleep inhibition method available on this Linux system")
	return nil
}
