//go:build !windows
// +build !windows

package server

import (
	"os/exec"
	"syscall"
)

// setupProcessGroup configures the process to be in its own process group
// This allows killing the entire process tree on timeout
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// killProcessGroup kills the entire process group
// On Unix, we use negative PID to target the process group
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	
	// Kill the process group (negative PID)
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

