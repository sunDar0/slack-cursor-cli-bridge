package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	ColorReset  = "\033[0m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorRed    = "\033[31m"
)

// RunSetup runs the interactive setup process
func RunSetup() error {
	printHeader()
	
	// Step 1: Check system
	if err := checkSystem(); err != nil {
		return err
	}

	// Step 2: Check and install cursor-agent
	if err := setupCursorAgent(); err != nil {
		return err
	}

	// Step 3: Check and install ngrok
	if err := setupNgrok(); err != nil {
		return err
	}

	// Step 4: Setup environment variables
	if err := setupEnv(); err != nil {
		return err
	}

	// Step 5: Initialize project
	if err := initializeProject(); err != nil {
		return err
	}

	printSuccess()
	return nil
}

func printHeader() {
	fmt.Println()
	fmt.Println(ColorBlue + "🚀 Slack-Cursor-Hook 설정 마법사" + ColorReset)
	fmt.Println(ColorBlue + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" + ColorReset)
	fmt.Println()
}

func checkSystem() error {
	fmt.Println(ColorYellow + "🔍 시스템 환경을 확인하는 중..." + ColorReset)
	
	os := GetOS()
	arch := GetArch()
	
	fmt.Printf("   OS: %s\n", os)
	fmt.Printf("   Architecture: %s\n", arch)
	
	if os == "windows" {
		printWindowsGuide()
		return fmt.Errorf("setup wizard is not supported on Windows")
	}
	
	fmt.Println(ColorGreen + "✅ 시스템 확인 완료" + ColorReset)
	fmt.Println()
	return nil
}

func printWindowsGuide() {
	fmt.Println()
	fmt.Println(ColorYellow + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" + ColorReset)
	fmt.Println(ColorYellow + "⚠️  --setup 기능은 macOS/Linux 전용입니다" + ColorReset)
	fmt.Println(ColorYellow + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" + ColorReset)
	fmt.Println()
	fmt.Println(ColorBlue + "Windows에서 사용하려면 다음을 수동으로 설정하세요:" + ColorReset)
	fmt.Println()
	
	fmt.Println(ColorGreen + "1. cursor-agent 설치 (Git Bash 사용):" + ColorReset)
	fmt.Println("   curl https://cursor.com/install -fsS | bash")
	fmt.Println()
	
	fmt.Println(ColorGreen + "2. ngrok 설치:" + ColorReset)
	fmt.Println("   https://ngrok.com/download 에서 다운로드")
	fmt.Println("   설치 후 PATH에 추가")
	fmt.Println()
	
	fmt.Println(ColorGreen + "3. .env 파일 생성:" + ColorReset)
	fmt.Println("   실행파일과 같은 위치에 .env 파일 생성")
	fmt.Println("   내용:")
	fmt.Println("   SLACK_SIGNING_SECRET=your_signing_secret_here")
	fmt.Println()
	
	fmt.Println(ColorGreen + "4. 서버 실행:" + ColorReset)
	fmt.Println("   .\\slack-cursor-hook-windows-amd64.exe")
	fmt.Println()
	
	fmt.Println(ColorBlue + "💡 자세한 내용은 SETUP.md를 참고하세요." + ColorReset)
	fmt.Println()
}

func setupCursorAgent() error {
	fmt.Println(ColorYellow + "🔍 cursor-agent 확인 중..." + ColorReset)
	
	result := CheckCursorAgent()
	
	if result.Installed {
		fmt.Println(ColorGreen + "✅ cursor-agent 설치됨" + ColorReset)
		if result.Version != "" {
			fmt.Printf("   버전: %s\n", result.Version)
		}
		fmt.Printf("   경로: %s\n", result.Path)
		
		// Check if PATH needs to be updated
		homeDir, _ := os.UserHomeDir()
		localBin := fmt.Sprintf("%s/.local/bin", homeDir)
		
		if !CheckPATH(localBin) && strings.Contains(result.Path, ".local/bin") {
			fmt.Println()
			fmt.Println(ColorYellow + "⚠️  ~/.local/bin이 PATH에 없습니다." + ColorReset)
			
			if AskYesNo("💡 PATH를 자동으로 설정하시겠습니까?") {
				if err := AddToPATH(localBin); err != nil {
					fmt.Println(ColorRed + "❌ PATH 설정 실패: " + err.Error() + ColorReset)
				}
			}
		}
		
		fmt.Println()
		return nil
	}

	fmt.Println(ColorRed + "❌ cursor-agent가 설치되지 않았습니다." + ColorReset)
	fmt.Println()
	
	if !AskYesNo("💡 자동 설치하시겠습니까?") {
		fmt.Println()
		fmt.Println(ColorYellow + "💡 수동 설치 방법:" + ColorReset)
		fmt.Println("   curl https://cursor.com/install -fsS | bash")
		fmt.Println()
		return fmt.Errorf("cursor-agent 설치가 필요합니다")
	}

	fmt.Println()
	if err := InstallCursorAgent(); err != nil {
		return err
	}

	// Add to PATH
	homeDir, _ := os.UserHomeDir()
	localBin := fmt.Sprintf("%s/.local/bin", homeDir)
	
	if !CheckPATH(localBin) {
		fmt.Println()
		if err := AddToPATH(localBin); err != nil {
			fmt.Println(ColorYellow + "⚠️  PATH 설정 실패. 수동으로 설정해주세요:" + ColorReset)
			fmt.Printf("   export PATH=\"$HOME/.local/bin:$PATH\"\n")
		}
	}

	fmt.Println()
	return nil
}

func setupNgrok() error {
	fmt.Println(ColorYellow + "🔍 ngrok 확인 중..." + ColorReset)
	
	result := CheckNgrok()
	
	if result.Installed {
		fmt.Println(ColorGreen + "✅ ngrok 설치됨" + ColorReset)
		if result.Version != "" {
			fmt.Printf("   버전: %s\n", result.Version)
		}
		fmt.Printf("   경로: %s\n", result.Path)
		fmt.Println()
		return nil
	}

	fmt.Println(ColorRed + "❌ ngrok이 설치되지 않았습니다." + ColorReset)
	fmt.Println()
	
	if !AskYesNo("💡 자동 설치하시겠습니까?") {
		fmt.Println()
		fmt.Println(ColorYellow + "💡 수동 설치 방법:" + ColorReset)
		
		os := GetOS()
		if os == "darwin" {
			fmt.Println("   brew install ngrok")
		} else if os == "linux" {
			fmt.Println("   sudo snap install ngrok")
			fmt.Println("   또는: https://ngrok.com/download")
		}
		fmt.Println()
		
		fmt.Println(ColorYellow + "⚠️  ngrok 없이도 서버는 실행되지만, Slack 연동이 불가능합니다." + ColorReset)
		fmt.Println()
		return nil // ngrok is optional, don't fail
	}

	fmt.Println()
	if err := InstallNgrok(); err != nil {
		fmt.Println(ColorYellow + "⚠️  ngrok 자동 설치 실패: " + err.Error() + ColorReset)
		fmt.Println(ColorYellow + "💡 수동으로 설치해주세요: brew install ngrok" + ColorReset)
	}

	fmt.Println()
	return nil
}

func setupEnv() error {
	fmt.Println(ColorYellow + "📝 환경 변수를 설정합니다..." + ColorReset)
	fmt.Println()

	// Get executable directory for .env location
	exePath, err := os.Executable()
	var envPath string
	if err == nil {
		exeDir := filepath.Dir(exePath)
		envPath = filepath.Join(exeDir, ".env")
	} else {
		// Fallback to current directory
		envPath = ".env"
	}
	
	// Check if .env already exists
	if _, err := os.Stat(envPath); err == nil {
		fmt.Println(ColorYellow + ".env 파일을 발견했습니다." + ColorReset)
		fmt.Printf("   위치: %s\n", envPath)
		if AskYesNo("기존 설정을 사용하시겠습니까?") {
			fmt.Println(ColorGreen + "✅ 기존 .env 파일 사용" + ColorReset)
			fmt.Println()
			return nil
		}
		fmt.Println()
	}

	fmt.Println("Slack Signing Secret을 입력하세요:")
	fmt.Println(ColorBlue + "(https://api.slack.com/apps 에서 확인)" + ColorReset)
	signingSecret := AskString("> ")
	
	if signingSecret == "" {
		return fmt.Errorf("SLACK_SIGNING_SECRET이 필요합니다")
	}

	// Create .env file in executable directory
	envContent := fmt.Sprintf("# Slack Configuration\nSLACK_SIGNING_SECRET=%s\n\n# Optional Settings\n# CURSOR_CLI_PATH=cursor-agent\n# CURSOR_PROJECT_PATH=/path/to/project\n# DB_PATH=./data/jobs.db\n# PORT=8080\n", signingSecret)
	
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		return fmt.Errorf(".env 파일 생성 실패: %v", err)
	}

	fmt.Println()
	fmt.Printf(ColorGreen + "✅ .env 파일 저장 완료: %s\n" + ColorReset, envPath)
	fmt.Println()
	return nil
}

func initializeProject() error {
	fmt.Println(ColorYellow + "🔧 프로젝트를 초기화하는 중..." + ColorReset)

	// Get executable directory
	exePath, err := os.Executable()
	var baseDir string
	if err == nil {
		baseDir = filepath.Dir(exePath)
	} else {
		// Fallback to current directory
		baseDir = "."
	}

	// Create data directory in executable directory
	dataDir := filepath.Join(baseDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("data/ 디렉토리 생성 실패: %v", err)
	}
	fmt.Printf("   ✅ data/ 디렉토리 생성: %s\n", dataDir)

	// Create logs directory in executable directory
	logsDir := filepath.Join(baseDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("logs/ 디렉토리 생성 실패: %v", err)
	}
	fmt.Printf("   ✅ logs/ 디렉토리 생성: %s\n", logsDir)

	fmt.Println()
	fmt.Println(ColorGreen + "✅ 프로젝트 초기화 완료!" + ColorReset)
	fmt.Println()
	return nil
}

func printSuccess() {
	fmt.Println(ColorGreen + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" + ColorReset)
	fmt.Println(ColorGreen + "✅ 모든 설정이 완료되었습니다!" + ColorReset)
	fmt.Println(ColorGreen + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" + ColorReset)
	fmt.Println()
	fmt.Println(ColorBlue + "🚀 다음 단계:" + ColorReset)
	fmt.Println()
	
	// Get current executable name
	exePath, err := os.Executable()
	var exeName string
	if err == nil {
		exeName = filepath.Base(exePath)
	} else {
		exeName = "실행파일"
	}
	
	// Check if we need to source RC file
	homeDir, _ := os.UserHomeDir()
	localBin := fmt.Sprintf("%s/.local/bin", homeDir)
	
	if !CheckPATH(localBin) {
		rcPath := filepath.Base(GetShellRCPath())
		fmt.Println(ColorYellow + "1. 터미널을 재시작하거나 다음을 실행:" + ColorReset)
		fmt.Printf("   source ~/%s\n", rcPath)
		fmt.Println()
		fmt.Println(ColorYellow + "2. 서버 시작:" + ColorReset)
	} else {
		fmt.Println(ColorYellow + "서버를 시작하세요:" + ColorReset)
	}
	
	fmt.Printf("   ./%s\n", exeName)
	fmt.Println()
	fmt.Println(ColorBlue + "💡 개발 모드 (서버 + ngrok + 로그):" + ColorReset)
	fmt.Println("   ./start-dev.sh")
	fmt.Println()
}

