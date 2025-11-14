package ngrok

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// Tunnel represents an ngrok tunnel
type Tunnel struct {
	PublicURL string `json:"public_url"`
	Proto     string `json:"proto"`
	Config    struct {
		Addr string `json:"addr"`
	} `json:"config"`
}

// APIResponse represents the ngrok API response
type APIResponse struct {
	Tunnels []Tunnel `json:"tunnels"`
}

// Manager handles ngrok tunnel lifecycle
type Manager struct {
	cmd       *exec.Cmd
	port      string
	publicURL string
}

// NewManager creates a new ngrok manager
func NewManager(port string) *Manager {
	return &Manager{
		port: port,
	}
}

// Start starts the ngrok tunnel
func (m *Manager) Start() error {
	// Check if ngrok is installed
	if _, err := exec.LookPath("ngrok"); err != nil {
		return fmt.Errorf("ngrok이 설치되어 있지 않습니다. 설치 방법: https://ngrok.com/download")
	}

	// Start ngrok in background
	m.cmd = exec.Command("ngrok", "http", m.port, "--log=stdout")
	m.cmd.Stdout = nil // Suppress ngrok output
	m.cmd.Stderr = nil

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("ngrok 시작 실패: %w", err)
	}

	// Wait for ngrok to be ready and get the public URL
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		
		url, err := m.GetPublicURL()
		if err == nil && url != "" {
			m.publicURL = url
			return nil
		}
	}

	return fmt.Errorf("ngrok URL을 가져올 수 없습니다 (타임아웃)")
}

// GetPublicURL retrieves the public URL from ngrok API
func (m *Manager) GetPublicURL() (string, error) {
	resp, err := http.Get("http://localhost:4040/api/tunnels")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", err
	}

	// Find HTTPS tunnel
	for _, tunnel := range apiResp.Tunnels {
		if tunnel.Proto == "https" {
			return tunnel.PublicURL, nil
		}
	}

	return "", fmt.Errorf("HTTPS 터널을 찾을 수 없습니다")
}

// GetURL returns the stored public URL
func (m *Manager) GetURL() string {
	return m.publicURL
}

// Stop stops the ngrok tunnel
func (m *Manager) Stop() error {
	if m.cmd != nil && m.cmd.Process != nil {
		if err := m.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("ngrok 종료 실패: %w", err)
		}
	}
	return nil
}

// PrintInstructions prints usage instructions with the ngrok URL
func (m *Manager) PrintInstructions() {
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("✅ 서버가 시작되었습니다!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println("🌐 ngrok 공개 URL:")
	fmt.Printf("   %s\n", m.publicURL)
	fmt.Println()
	fmt.Println("📝 Slack App 설정:")
	fmt.Println("   1. https://api.slack.com/apps 접속")
	fmt.Println("   2. 앱 선택 → Slash Commands → /cursor 편집")
	fmt.Println("   3. Request URL에 다음을 입력:")
	fmt.Printf("      %s/slack/cursor\n", m.publicURL)
	fmt.Println()
	fmt.Println("🔗 유용한 링크:")
	fmt.Printf("   • Swagger UI:    http://localhost:%s/swagger/index.html\n", m.port)
	fmt.Println("   • ngrok 대시보드: http://localhost:4040")
	fmt.Printf("   • Health Check:  http://localhost:%s/health\n", m.port)
	fmt.Println()
	fmt.Println("📋 사용 가능한 Slack 명령어:")
	fmt.Println("   /cursor help                       - 도움말 및 전체 명령어 목록")
	fmt.Println("   /cursor set-path <경로>            - 프로젝트 경로 설정")
	fmt.Println("   /cursor path                       - 현재 경로 확인")
	fmt.Println("   /cursor list                       - 최근 작업 목록")
	fmt.Println("   /cursor show <job-id>              - 작업 결과 보기")
	fmt.Println("   /cursor \"프롬프트\"                  - 코드 작업 요청")
	fmt.Println()
	fmt.Println("⚠️  종료하려면 Ctrl+C를 누르세요")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
}

// IsInstalled checks if ngrok is installed
func IsInstalled() bool {
	_, err := exec.LookPath("ngrok")
	return err == nil
}

// PrintNotInstalledWarning prints a warning if ngrok is not installed
func PrintNotInstalledWarning(port string) {
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("⚠️  ngrok이 설치되어 있지 않습니다")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println("서버는 로컬에서만 실행됩니다:")
	fmt.Printf("   http://localhost:%s\n", port)
	fmt.Println()
	fmt.Println("💡 Slack 연동을 위해서는 ngrok 설치가 필요합니다:")
	
	// Detect OS and show appropriate install command
	switch os.Getenv("GOOS") {
	case "darwin":
		fmt.Println("   brew install ngrok")
	case "linux":
		fmt.Println("   sudo snap install ngrok")
	case "windows":
		fmt.Println("   https://ngrok.com/download")
	default:
		fmt.Println("   https://ngrok.com/download")
	}
	
	fmt.Println()
	fmt.Println("또는 설정 마법사를 실행하세요:")
	fmt.Println("   ./실행파일 --setup")
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
}

