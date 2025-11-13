package setup

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// InstallCursorAgent installs cursor-agent CLI
func InstallCursorAgent() error {
	fmt.Println("📦 cursor-agent 설치 중...")
	fmt.Println("   curl https://cursor.com/install -fsS | bash")
	fmt.Println()

	// Run installation script
	cmd := exec.Command("bash", "-c", "curl https://cursor.com/install -fsS | bash")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cursor-agent 설치 실패: %v", err)
	}

	// Wait for installation to complete
	fmt.Println()
	fmt.Println("⏳ 설치 완료를 확인하는 중...")
	
	homeDir, _ := os.UserHomeDir()
	defaultPath := fmt.Sprintf("%s/.local/bin/cursor-agent", homeDir)
	
	// Wait up to 10 seconds for the binary to appear
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(defaultPath); err == nil {
			fmt.Println("✅ cursor-agent 설치 완료!")
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("cursor-agent 파일을 찾을 수 없습니다: %s", defaultPath)
}

// InstallNgrok installs ngrok based on the OS
func InstallNgrok() error {
	osName := GetOS()
	
	fmt.Println("📦 ngrok 설치 중...")
	
	var cmd *exec.Cmd
	
	switch osName {
	case "darwin": // macOS
		fmt.Println("   brew install ngrok")
		cmd = exec.Command("brew", "install", "ngrok")
		
	case "linux":
		fmt.Println("   자동 설치 스크립트 실행 중...")
		// Use snap for Linux
		cmd = exec.Command("sudo", "snap", "install", "ngrok")
		
	default:
		return fmt.Errorf("지원하지 않는 OS입니다: %s", osName)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ngrok 설치 실패: %v", err)
	}

	fmt.Println("✅ ngrok 설치 완료!")
	return nil
}

// AddToPATH adds a directory to the shell RC file
func AddToPATH(dir string) error {
	rcPath := GetShellRCPath()
	if rcPath == "" {
		return fmt.Errorf("지원하지 않는 셸입니다")
	}

	shell := DetectShell()
	
	fmt.Printf("📝 %s에 PATH 추가 중...\n", rcPath)
	fmt.Printf("   export PATH=\"$HOME/.local/bin:$PATH\"\n")
	fmt.Println()

	// Check if already exists
	content, err := os.ReadFile(rcPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	pathExport := fmt.Sprintf("export PATH=\"$HOME/.local/bin:$PATH\"")
	
	// Check if already added
	if strings.Contains(string(content), pathExport) {
		fmt.Println("✅ PATH가 이미 설정되어 있습니다.")
		return nil
	}

	// Append to RC file
	f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(fmt.Sprintf("\n# Added by slack-cursor-hook\n%s\n", pathExport))
	if err != nil {
		return err
	}

	fmt.Println("✅ PATH 설정 완료!")
	fmt.Println()
	fmt.Println("⚠️  새로운 터미널을 열거나 다음을 실행하세요:")
	fmt.Printf("   source %s\n", rcPath)
	
	if shell == "zsh" {
		fmt.Println("   또는: source ~/.zshrc")
	} else {
		fmt.Println("   또는: source ~/.bashrc")
	}

	return nil
}

// AskYesNo prompts the user for a yes/no question
func AskYesNo(question string) bool {
	reader := bufio.NewReader(os.Stdin)
	
	for {
		fmt.Printf("%s (y/n): ", question)
		response, err := reader.ReadString('\n')
		if err != nil {
			return false
		}

		response = strings.TrimSpace(strings.ToLower(response))
		
		if response == "y" || response == "yes" {
			return true
		}
		if response == "n" || response == "no" {
			return false
		}
		
		fmt.Println("'y' 또는 'n'을 입력해주세요.")
	}
}

// AskString prompts the user for a string input
func AskString(prompt string) string {
	reader := bufio.NewReader(os.Stdin)
	
	fmt.Print(prompt)
	response, err := reader.ReadString('\n')
	if err != nil {
		return ""
	}

	return strings.TrimSpace(response)
}

