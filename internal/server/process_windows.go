//go:build windows
// +build windows

package server

import (
	"os/exec"
	"syscall"
)

// setupProcessGroup configures the process for Windows
// Windows uses Job Objects instead of process groups
func setupProcessGroup(cmd *exec.Cmd) {
	// Windows: Create a new process group
	// This is similar to Unix Setpgid but Windows-specific
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// killProcessGroup kills the process on Windows
// Windows doesn't have process groups like Unix, so we just kill the main process
// Note: Child processes may not be killed automatically
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	
	// On Windows, cmd.Process.Kill() sends SIGTERM
	return cmd.Process.Kill()
}

