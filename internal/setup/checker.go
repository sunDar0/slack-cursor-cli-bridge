package setup

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// CheckResult represents the result of a dependency check
type CheckResult struct {
	Name      string
	Installed bool
	Version   string
	Path      string
	Message   string
}

// CheckCursorAgent checks if cursor-agent is installed
func CheckCursorAgent() CheckResult {
	result := CheckResult{
		Name: "cursor-agent",
	}

	// Try to find cursor-agent in PATH
	path, err := exec.LookPath("cursor-agent")
	if err == nil {
		result.Installed = true
		result.Path = path

		// Get version
		cmd := exec.Command("cursor-agent", "--version")
		output, err := cmd.Output()
		if err == nil {
			result.Version = strings.TrimSpace(string(output))
		}
		return result
	}

	// Check default installation paths by OS
	homeDir, _ := os.UserHomeDir()
	var defaultPaths []string
	
	osName := runtime.GOOS
	switch osName {
	case "windows":
		// Windows 기본 경로들
		defaultPaths = []string{
			fmt.Sprintf("%s\\.local\\bin\\cursor-agent.exe", homeDir),
			fmt.Sprintf("%s\\AppData\\Local\\Programs\\cursor-agent\\cursor-agent.exe", homeDir),
			fmt.Sprintf("%s\\AppData\\Roaming\\cursor-agent\\cursor-agent.exe", homeDir),
		}
	case "darwin", "linux":
		// macOS/Linux 기본 경로들
		defaultPaths = []string{
			fmt.Sprintf("%s/.local/bin/cursor-agent", homeDir),
			"/usr/local/bin/cursor-agent",
			"/opt/cursor-agent/cursor-agent",
		}
	default:
		// 기타 Unix 계열
		defaultPaths = []string{
			fmt.Sprintf("%s/.local/bin/cursor-agent", homeDir),
		}
	}
	
	// 각 기본 경로 확인
	for _, defaultPath := range defaultPaths {
		if _, err := os.Stat(defaultPath); err == nil {
			result.Installed = true
			result.Path = defaultPath
			result.Message = "설치되어 있지만 PATH에 없습니다"
			return result
		}
	}

	result.Message = "설치되지 않았습니다"
	return result
}

// CheckNgrok checks if ngrok is installed
func CheckNgrok() CheckResult {
	result := CheckResult{
		Name: "ngrok",
	}

	path, err := exec.LookPath("ngrok")
	if err == nil {
		result.Installed = true
		result.Path = path

		// Get version
		cmd := exec.Command("ngrok", "version")
		output, err := cmd.Output()
		if err == nil {
			result.Version = strings.TrimSpace(string(output))
		}
		return result
	}

	result.Message = "설치되지 않았습니다"
	return result
}

// DetectShell detects the current shell
func DetectShell() string {
	shell := os.Getenv("SHELL")
	if strings.Contains(shell, "zsh") {
		return "zsh"
	}
	if strings.Contains(shell, "bash") {
		return "bash"
	}
	return "unknown"
}

// GetShellRCPath returns the RC file path for the current shell
func GetShellRCPath() string {
	homeDir, _ := os.UserHomeDir()
	shell := DetectShell()
	
	switch shell {
	case "zsh":
		return fmt.Sprintf("%s/.zshrc", homeDir)
	case "bash":
		return fmt.Sprintf("%s/.bashrc", homeDir)
	default:
		return ""
	}
}

// CheckPATH checks if a directory is in PATH
func CheckPATH(dir string) bool {
	pathEnv := os.Getenv("PATH")
	paths := strings.Split(pathEnv, ":")
	
	for _, p := range paths {
		if p == dir {
			return true
		}
	}
	return false
}

// GetOS returns the current operating system
func GetOS() string {
	return runtime.GOOS
}

// GetArch returns the current architecture
func GetArch() string {
	return runtime.GOARCH
}

