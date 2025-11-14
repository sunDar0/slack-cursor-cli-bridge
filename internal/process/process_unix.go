//go:build !windows
// +build !windows

package process

import (
	"os/exec"
	"syscall"
)

// SetupProcessGroup configures the process to be in its own process group
// This allows killing the entire process tree on timeout
func SetupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// KillProcessGroup kills the entire process group
// On Unix, we use negative PID to target the process group
func KillProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	
	// Kill the process group (negative PID)
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

