package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/kakaovx/cursor-slack-server/docs" // Swagger docs
	"github.com/kakaovx/cursor-slack-server/internal/database"
	"github.com/kakaovx/cursor-slack-server/internal/ngrok"
	"github.com/kakaovx/cursor-slack-server/internal/server"
	"github.com/kakaovx/cursor-slack-server/internal/setup"
	"github.com/kakaovx/cursor-slack-server/internal/worker"
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

	// 로그 파일 설정 (실행 파일과 같은 디렉토리)
	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		logsDir := filepath.Join(exeDir, "logs")
		
		// logs 디렉토리 생성 (없으면)
		if err := os.MkdirAll(logsDir, 0755); err == nil {
			logFile := filepath.Join(logsDir, "server.log")
			
			// 로그 파일 열기 (append 모드)
			f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				// stdout과 파일 둘 다에 로그 출력
				mw := io.MultiWriter(os.Stdout, f)
				log.SetOutput(mw)
				log.Printf("📝 로그 파일: %s", logFile)
			}
		}
	}

	// .env 파일 로드 (실행 파일과 같은 디렉토리 또는 현재 디렉토리)
	// 1. 실행 파일과 같은 디렉토리에서 .env 찾기
	exePath, exeErr := os.Executable()
	if exeErr == nil {
		exeDir := filepath.Dir(exePath)
		envPath := filepath.Join(exeDir, ".env")
		
		if loadErr := godotenv.Load(envPath); loadErr == nil {
			log.Printf("✅ .env 파일을 로드했습니다: %s", envPath)
		} else {
			// 2. 현재 작업 디렉토리에서 .env 찾기
			if loadErr := godotenv.Load(); loadErr != nil {
				log.Println("⚠️  .env 파일을 찾을 수 없습니다. 시스템 환경변수를 사용합니다.")
			} else {
				log.Println("✅ .env 파일을 로드했습니다: ./.env")
			}
		}
	} else {
		// 실행 파일 경로를 찾을 수 없는 경우
		if loadErr := godotenv.Load(); loadErr != nil {
			log.Println("⚠️  .env 파일을 찾을 수 없습니다. 시스템 환경변수를 사용합니다.")
		} else {
			log.Println("✅ .env 파일을 로드했습니다.")
		}
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
		// cursor-agent 설치 확인
		cursorResult := setup.CheckCursorAgent()
		if cursorResult.Installed {
			cursorCLIPath = cursorResult.Path
			log.Printf("✅ cursor-agent 발견: %s", cursorResult.Path)
			if cursorResult.Version != "" {
				log.Printf("   버전: %s", cursorResult.Version)
			}
			if cursorResult.Message != "" {
				log.Printf("   참고: %s", cursorResult.Message)
			}
		} else {
			// 기본값: PATH에서 cursor-agent 검색
			cursorCLIPath = "cursor-agent"
			log.Println("⚠️  cursor-agent가 설치되지 않았습니다.")
			log.Println("   기본값 사용: 'cursor-agent' (PATH에서 검색)")
			log.Println()
			log.Println("💡 cursor-agent 설치 방법:")
			
			osName := setup.GetOS()
			if osName == "windows" {
				log.Println("   Git Bash에서 실행:")
				log.Println("   curl https://cursor.com/install -fsS | bash")
			} else {
				log.Println("   curl https://cursor.com/install -fsS | bash")
			}
			log.Println()
			log.Println("   또는 .env에 CURSOR_CLI_PATH를 직접 설정:")
			if osName == "windows" {
				log.Println("   CURSOR_CLI_PATH=C:\\path\\to\\cursor-agent.exe")
			} else {
				log.Println("   CURSOR_CLI_PATH=/path/to/cursor-agent")
			}
		}
	} else {
		log.Printf("ℹ️  CURSOR_CLI_PATH 사용: %s", cursorCLIPath)
	}

	// v1.1: SSRF 방어용 허용 도메인 설정
	allowedDomains := []string{"hooks.slack.com"}
	log.Printf("ℹ️  SSRF 방어: 허용 도메인 = %v", allowedDomains)

	// v1.3: SQLite 데이터베이스 초기화
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		// 실행 파일 기준으로 DB 경로 설정
		if exePath, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exePath)
			dbPath = filepath.Join(exeDir, "data", "jobs.db")
		} else {
			dbPath = "./data/jobs.db" // Fallback
		}
	}
	
	// 데이터베이스 디렉토리 생성 (없으면)
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		log.Fatalf("데이터베이스 디렉토리 생성 실패: %v", err)
	}
	
	// 절대 경로로 변환하여 표시
	absDbPath, _ := filepath.Abs(dbPath)
	
	db, dbErr := database.NewDB(dbPath)
	if dbErr != nil {
		log.Fatalf("데이터베이스 초기화 실패: %v", dbErr)
	}
	// defer db.Close() 제거 - graceful shutdown에서 명시적으로 닫음
	
	log.Println()
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("📦 데이터베이스 위치: %s", absDbPath)
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println()

	// v1.4: Worker Pool 설정
	maxWorkers := 3 // 기본값: 3개의 동시 작업
	if maxWorkersEnv := os.Getenv("MAX_WORKERS"); maxWorkersEnv != "" {
		if parsed, err := strconv.Atoi(maxWorkersEnv); err == nil && parsed > 0 {
			maxWorkers = parsed
		}
	}
	
	// 작업 큐 생성 (버퍼 크기: maxWorkers * 2)
	jobQueue := make(chan worker.Job, maxWorkers*2)
	
	// TaskExecutor 생성
	taskExecutor := worker.NewTaskExecutor(allowedDomains)
	
	// Dispatcher 생성 및 시작
	dispatcher := worker.NewDispatcher(jobQueue, maxWorkers)
	dispatcher.Start(taskExecutor)
	
	log.Printf("🔧 Worker Pool 초기화 완료: %d개 작업자, 큐 크기: %d", maxWorkers, maxWorkers*2)
	log.Println()

	// 설정 정보를 담은 구조체 (v1.2: 동적 경로 관리, v1.3: DB 추가, v1.4: Worker Pool 추가)
	config := &server.Config{
		SigningSecret:          signingSecret,
		Port:                   port,
		CursorCLIPath:          cursorCLIPath,
		AllowedResponseDomains: allowedDomains,
		DB:                     db,
		Dispatcher:             dispatcher,
		JobQueue:               jobQueue,
	}

	// 환경 변수로 초기 프로젝트 경로 설정 (있는 경우)
	if projectPath != "" {
		config.SetProjectPath(projectPath)
	}

	// v1.4: 포트 사용 가능 여부 확인 및 정리
	log.Printf("🔍 포트 %s 사용 가능 여부 확인 중...", port)
	autoKill := os.Getenv("AUTO_KILL_PORT") == "true" // 환경변수로 자동 종료 설정
	if err := server.EnsurePortAvailable(port, autoKill); err != nil {
		log.Printf("❌ 포트 사용 불가: %v", err)
		log.Println()
		log.Println("💡 해결 방법:")
		log.Println("   1. 기존 서버를 종료하세요")
		log.Println("   2. 또는 다른 포트를 사용하세요 (환경변수 PORT 설정)")
		if !autoKill {
			log.Println("   3. AUTO_KILL_PORT=true로 설정하면 자동으로 기존 프로세스를 종료합니다")
		}
		os.Exit(1)
	}
	log.Println("✅ 포트 사용 가능")
	log.Println()

	// 라우터 설정
	router := server.SetupRouter(config)

	// ngrok 시작 (선택사항)
	var ngrokManager *ngrok.Manager
	if ngrok.IsInstalled() {
		ngrokManager = ngrok.NewManager(port)
		log.Println("🌐 ngrok 터널 생성 중...")
		
		if err := ngrokManager.Start(); err != nil {
			log.Printf("⚠️  ngrok 시작 실패: %v", err)
			log.Println("서버는 로컬에서만 실행됩니다.")
		} else {
			ngrokManager.PrintInstructions()
		}
	} else {
		ngrok.PrintNotInstalledWarning(port)
	}

	// 서버를 별도 goroutine에서 시작
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           router,
		ReadTimeout:       15 * time.Minute, // 작업 타임아웃과 동일
		WriteTimeout:      15 * time.Minute,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("🚀 서버를 포트 %s에서 시작합니다...", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("서버 시작 실패: %v", err)
		}
	}()

	// Graceful shutdown 처리
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 서버를 종료합니다...")
	log.Println("1️⃣ 새로운 HTTP 요청 차단 중...")

	// 1. HTTP 서버 graceful shutdown (새 요청 차단, 기존 요청은 처리)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("⚠️  HTTP 서버 종료 중 오류: %v", err)
	} else {
		log.Println("✅ HTTP 서버 종료 완료")
	}

	// 2. JobQueue 닫기 (새 작업 수신 중단)
	log.Println("2️⃣ 작업 큐 닫는 중...")
	close(config.JobQueue)
	log.Println("✅ 작업 큐 닫힘 (새 작업 수신 중단)")

	// 3. Worker Pool 종료 (진행 중인 작업 완료 대기)
	log.Println("3️⃣ 진행 중인 작업 완료 대기 중...")
	if config.Dispatcher != nil {
		// 별도 goroutine에서 종료 대기 (타임아웃 적용)
		workerDone := make(chan struct{})
		go func() {
			config.Dispatcher.Stop()
			close(workerDone)
		}()

		// 최대 30초 대기 (작업이 길 수 있으므로)
		select {
		case <-workerDone:
			log.Println("✅ 모든 작업자 종료 완료")
		case <-time.After(30 * time.Second):
			log.Println("⚠️  작업자 종료 시간 초과 (30초) - 강제 종료")
		}
	}

	// 4. ngrok 종료
	log.Println("4️⃣ ngrok 터널 종료 중...")
	if ngrokManager != nil {
		if err := ngrokManager.Stop(); err != nil {
			log.Printf("⚠️  ngrok 종료 중 오류: %v", err)
		} else {
			log.Println("✅ ngrok 터널 종료 완료")
		}
	}

	// 5. DB 연결 닫기 (defer 대신 명시적으로)
	log.Println("5️⃣ 데이터베이스 연결 종료 중...")
	if err := db.Close(); err != nil {
		log.Printf("⚠️  데이터베이스 종료 중 오류: %v", err)
	} else {
		log.Println("✅ 데이터베이스 연결 종료 완료")
	}

	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("✅ 모든 리소스 정리 완료 - 서버 종료")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

