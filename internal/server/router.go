package server

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/kakaovx/cursor-slack-server/internal/database"
	"github.com/kakaovx/cursor-slack-server/internal/server/middleware"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Config는 서버 설정을 담는 구조체입니다.
// v1.1: AllowedResponseDomains 추가 (SSRF 방어)
// v1.2: ProjectPath를 동적으로 관리 (런타임 설정/변경 가능)
// v1.3: SQLite DB 추가 (작업 결과 저장)
type Config struct {
	SigningSecret          string
	projectPath            string           // private: 동적 설정
	Port                   string
	CursorCLIPath          string
	AllowedResponseDomains []string         // SSRF 방어용 허용 도메인 목록
	DB                     *database.DB     // SQLite 데이터베이스
	mu                     sync.RWMutex
}

// SetProjectPath는 프로젝트 경로를 설정합니다 (thread-safe)
func (c *Config) SetProjectPath(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projectPath = path
}

// GetProjectPath는 프로젝트 경로를 반환합니다 (thread-safe)
func (c *Config) GetProjectPath() (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.projectPath == "" {
		return "", false
	}
	return c.projectPath, true
}

// SetupRouter는 Gin 라우터와 미들웨어, 핸들러를 설정합니다.
func SetupRouter(cfg *Config) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// Slack API 엔드포인트 그룹 (HMAC 인증 필요)
	slackApi := r.Group("/slack")
	{
		// Slack 요청 인증 미들웨어 적용
		authMiddleware := middleware.SlackAuthMiddleware(cfg.SigningSecret)
		slackApi.Use(authMiddleware)

		// 핸들러 바인딩
		slackApi.POST("/cursor", HandleSlashCursor(cfg))
		
		// Options API for autocomplete
		slackApi.POST("/cursor/options", HandleSlackOptions(cfg))
	}

	// 일반 API 엔드포인트 그룹 (인증 불필요 - 테스트/개발용)
	api := r.Group("/api")
	{
		// Cursor 실행 API
		api.POST("/cursor", HandleAPICursor(cfg))

		// 설정 API (v1.2: 동적 프로젝트 경로 관리)
		config := api.Group("/config")
		{
			config.GET("/project-path", HandleGetProjectPath(cfg))
			config.POST("/project-path", HandleSetProjectPath(cfg))
		}

		// 작업 관리 API (v1.3: 작업 결과 조회)
		jobs := api.Group("/jobs")
		{
			jobs.GET("/:id", HandleGetJob(cfg))
			jobs.GET("", HandleListJobs(cfg))
		}
	}

	// Health check 엔드포인트
	r.GET("/health", HealthCheck)

	// Swagger UI 엔드포인트
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return r
}

// HealthResponse는 Health check 응답 구조체입니다.
type HealthResponse struct {
	Status string `json:"status" example:"ok"`
}

// HealthCheck godoc
// @Summary      서버 상태 확인
// @Description  서버가 정상 동작 중인지 확인합니다.
// @Tags         health
// @Produce      json
// @Success      200  {object}  HealthResponse
// @Router       /health [get]
func HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResponse{Status: "ok"})
}

