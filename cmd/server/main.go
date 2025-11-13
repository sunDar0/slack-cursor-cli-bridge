package main

import (
	"flag"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/kakaovx/cursor-slack-server/docs" // Swagger docs
	"github.com/kakaovx/cursor-slack-server/internal/database"
	"github.com/kakaovx/cursor-slack-server/internal/server"
	"github.com/kakaovx/cursor-slack-server/internal/setup"
)

// @title           Slack-Cursor-CLI API (v1.3)
// @version         1.3
// @description     Slack 슬래시 커맨드를 통해 Cursor Agent를 실행하는 서버입니다.
// @description     v1.3: SQLite 작업 결과 저장 및 조회
// @description     v1.2: 동적 프로젝트 경로 관리 (런타임 설정/변경)
// @description     v1.1: 자연어 프롬프트 방식, Process Group 관리, SSRF 방어
// @description     주요 기능: HMAC 인증 + 비동기 실행 + 보안 강화 + 동적 경로 관리 + 작업 결과 저장
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  MIT
// @license.url   https://opensource.org/licenses/MIT

// @host      localhost:8080
// @BasePath  /

// @securityDefinitions.apikey SlackSignature
// @in header
// @name X-Slack-Signature
// @description Slack HMAC-SHA256 서명

// @securityDefinitions.apikey SlackTimestamp
// @in header
// @name X-Slack-Request-Timestamp
// @description Slack 요청 타임스탬프 (Unix timestamp)

func main() {
	// CLI 플래그 파싱
	setupMode := flag.Bool("setup", false, "대화형 설정 마법사 실행")
	flag.Parse()

	// 설정 모드인 경우 설정 마법사 실행
	if *setupMode {
		if err := setup.RunSetup(); err != nil {
			log.Fatalf("설정 실패: %v", err)
		}
		return
	}

	// .env 파일 로드 (파일이 없어도 에러는 무시)
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  .env 파일을 찾을 수 없습니다. 시스템 환경변수를 사용합니다.")
	} else {
		log.Println("✅ .env 파일을 로드했습니다.")
	}

	// 환경변수 로드
	signingSecret := os.Getenv("SLACK_SIGNING_SECRET")
	if signingSecret == "" {
		log.Fatal("SLACK_SIGNING_SECRET 환경변수가 설정되지 않았습니다.")
	}

	// v1.2: 프로젝트 경로는 런타임에 동적으로 설정
	// 환경 변수로 초기값 설정 가능 (선택사항)
	projectPath := os.Getenv("CURSOR_PROJECT_PATH")
	if projectPath != "" {
		log.Printf("ℹ️  환경변수로부터 초기 프로젝트 경로 설정: %s", projectPath)
	} else {
		log.Println("ℹ️  프로젝트 경로가 설정되지 않았습니다.")
		log.Println("   💡 다음 방법으로 경로를 설정할 수 있습니다:")
		log.Println("      - API: POST /api/config/project-path")
		log.Println("      - Slack: /cursor set-path <경로>")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	cursorCLIPath := os.Getenv("CURSOR_CLI_PATH")
	if cursorCLIPath == "" {
		// 기본값: PATH에서 cursor-agent 검색
		cursorCLIPath = "cursor-agent"
		log.Println("ℹ️  CURSOR_CLI_PATH가 설정되지 않았습니다. 기본값 사용: 'cursor-agent' (PATH에서 검색)")
		log.Println("   💡 cursor-agent가 PATH에 없다면 .env에 CURSOR_CLI_PATH를 설정하세요.")
		log.Println("      예: CURSOR_CLI_PATH=/Users/username/.local/bin/cursor-agent")
	} else {
		log.Printf("ℹ️  CURSOR_CLI_PATH 사용: %s", cursorCLIPath)
	}

	// v1.1: SSRF 방어용 허용 도메인 설정
	allowedDomains := []string{"hooks.slack.com"}
	log.Printf("ℹ️  SSRF 방어: 허용 도메인 = %v", allowedDomains)

	// v1.3: SQLite 데이터베이스 초기화
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/jobs.db" // 기본 경로
	}
	db, err := database.NewDB(dbPath)
	if err != nil {
		log.Fatalf("데이터베이스 초기화 실패: %v", err)
	}
	defer db.Close()

	// 설정 정보를 담은 구조체 (v1.2: 동적 경로 관리, v1.3: DB 추가)
	config := &server.Config{
		SigningSecret:          signingSecret,
		Port:                   port,
		CursorCLIPath:          cursorCLIPath,
		AllowedResponseDomains: allowedDomains,
		DB:                     db,
	}

	// 환경 변수로 초기 프로젝트 경로 설정 (있는 경우)
	if projectPath != "" {
		config.SetProjectPath(projectPath)
	}

	// 라우터 설정
	router := server.SetupRouter(config)

	// 서버 시작
	log.Printf("서버를 포트 %s에서 시작합니다...", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("서버 시작 실패: %v", err)
	}
}

