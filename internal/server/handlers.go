package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kakaovx/cursor-slack-server/internal/database"
	"github.com/kakaovx/cursor-slack-server/internal/server/middleware"
	"github.com/kakaovx/cursor-slack-server/internal/types"
	"github.com/kakaovx/cursor-slack-server/internal/worker"
)

// SlackImmediateResponse는 Slack 즉시 응답용 JSON 구조체입니다.
type SlackImmediateResponse struct {
	ResponseType string `json:"response_type" example:"ephemeral"`
	Text         string `json:"text" example:"⏳ 요청을 접수했습니다. 작업을 처리 중입니다..."`
}

// ErrorResponse는 에러 응답 구조체입니다.
type ErrorResponse struct {
	Error string `json:"error" example:"Invalid request payload"`
}

// APICursorRequest는 일반 API용 cursor 실행 요청 구조체입니다.
// v1.1: 자연어 프롬프트 방식 (파일명을 프롬프트에 포함)
type APICursorRequest struct {
	Prompt string `json:"prompt" example:"main.go의 버그를 수정해줘" binding:"required"`
	Async  bool   `json:"async" example:"false"`
}

// APICursorResponse는 일반 API용 cursor 실행 응답 구조체입니다.
type APICursorResponse struct {
	Status  string `json:"status" example:"success"`
	Message string `json:"message" example:"Cursor AI 작업이 완료되었습니다."`
	Output  string `json:"output,omitempty" example:"// 실행 결과 출력"`
	JobID   string `json:"job_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000"`
}

// ProjectPathRequest는 프로젝트 경로 설정 요청 구조체입니다 (v1.2)
type ProjectPathRequest struct {
	Path string `json:"path" example:"/Users/username/projects/my-project" binding:"required"`
}

// ProjectPathResponse는 프로젝트 경로 응답 구조체입니다 (v1.2)
type ProjectPathResponse struct {
	Path    string `json:"path" example:"/Users/username/projects/my-project"`
	IsSet   bool   `json:"is_set" example:"true"`
	Message string `json:"message,omitempty" example:"프로젝트 경로가 설정되었습니다."`
}

// HandleSlashCursor godoc
// @Summary      Slack 슬래시 커맨드 처리 (v1.1)
// @Description  Slack의 /cursor 슬래시 커맨드를 받아 cursor-agent를 비동기로 실행합니다.
// @Description  v1.1: 자연어 프롬프트 방식 사용 (파일명을 프롬프트에 직접 포함)
// @Description  HMAC-SHA256 서명 검증 및 타임스탬프 검증이 필요합니다.
// @Tags         slack
// @Accept       x-www-form-urlencoded
// @Produce      json
// @Param        text          formData  string  true   "자연어 프롬프트 (예: 'main.go의 버그를 수정해줘')"
// @Param        user_name     formData  string  true   "Slack 사용자명"
// @Param        user_id       formData  string  true   "Slack 사용자 ID"
// @Param        response_url  formData  string  true   "지연 응답을 보낼 Slack Webhook URL"
// @Param        trigger_id    formData  string  true   "Slack 트리거 ID"
// @Success      200  {object}  SlackImmediateResponse  "즉시 ACK 응답"
// @Failure      400  {object}  ErrorResponse           "잘못된 요청"
// @Failure      401  {object}  ErrorResponse           "인증 실패"
// @Security     SlackSignature
// @Security     SlackTimestamp
// @Router       /slack/cursor [post]
func HandleSlashCursor(cfg *Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload types.SlackCommandPayload

		if err := c.ShouldBind(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}

		// 명령어 처리
		text := strings.TrimSpace(payload.Text)
		
		// 명령어 파싱
		parts := strings.Fields(text)
		if len(parts) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"response_type": "ephemeral",
				"text":          "❌ 명령어 또는 프롬프트를 입력해주세요.\n💡 도움말: `/cursor help`",
			})
			return
		}
		
		command := parts[0]
		
		// 명령어별 처리
		switch command {
		case "help", "?":
			handleHelpCommand(c)
			return
			
		case "list", "jobs":
			handleListCommand(c, cfg, payload.UserID)
			return
			
		case "show", "result":
			if len(parts) < 2 {
				c.JSON(http.StatusOK, gin.H{
					"response_type": "ephemeral",
					"text":          "❌ Job ID를 입력해주세요.\n사용법: `/cursor show <job-id>`",
				})
				return
			}
			handleShowCommand(c, cfg, parts[1])
			return
			
		case "path", "get-path":
			handlePathCommand(c, cfg)
			return
			
		case "set-path":
			if len(parts) < 2 {
				c.JSON(http.StatusOK, gin.H{
					"response_type": "ephemeral",
					"text":          "❌ 경로를 입력해주세요.\n사용법: `/cursor set-path /path/to/project`",
				})
				return
			}
			path := strings.TrimSpace(strings.TrimPrefix(text, "set-path "))
			cfg.SetProjectPath(path)
			log.Printf("[%s] Slack을 통해 프로젝트 경로 설정: %s", payload.UserID, path)
			c.JSON(http.StatusOK, gin.H{
				"response_type": "ephemeral",
				"text":          fmt.Sprintf("✅ 프로젝트 경로가 설정되었습니다:\n`%s`\n\n이제 `/cursor \"프롬프트\"` 명령어를 사용할 수 있습니다.", path),
			})
			return
		}

		// 1. 즉시 응답 (ACK) - 3초 룰 준수
		c.JSON(http.StatusOK, gin.H{
			"response_type": "ephemeral",
			"text":          fmt.Sprintf("⏳ %s님의 요청을 접수했습니다. 작업을 처리 중입니다...\n💡 최대 대기시간: 15분", payload.UserName),
		})

		// 2. Worker Pool을 통해 작업 제출 (v1.4)
		reqID, exists := c.Get(middleware.RequestIDKey)
		if !exists {
			reqID = uuid.NewString()
		}
		jobID := reqID.(string)

		// Job 생성 및 큐에 제출 (ConfigFull wrapper 생성)
		job := worker.Job{
			ID:         jobID,
			Payload:    payload,
			ReceivedAt: time.Now(),
			Config:     cfg.ToWorkerConfig(),
		}

		// 비동기로 큐에 제출 (큐가 가득 차면 블록될 수 있음)
		go func() {
			// 서버 종료 시 JobQueue가 닫힐 수 있으므로 panic 방지
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[%s] ⚠️ 작업 큐가 닫혀서 제출 실패: %v", jobID, r)
				}
			}()
			
			cfg.JobQueue <- job
			log.Printf("[%s] 작업이 큐에 제출되었습니다.", jobID)
		}()
	}
}

// HandleAPICursor godoc
// @Summary      일반 API를 통한 Cursor Agent 실행 (v1.1)
// @Description  JSON 형식으로 cursor-agent를 실행합니다. Slack 인증이 필요 없습니다.
// @Description  v1.1: 자연어 프롬프트 방식 (파일명을 프롬프트에 직접 포함)
// @Description  async=false: 동기 실행 (결과를 즉시 반환)
// @Description  async=true: 비동기 실행 (job_id만 반환, 결과는 별도 조회 필요 - 원 단계에서 구현)
// @Tags         api
// @Accept       json
// @Produce      json
// @Param        request  body      APICursorRequest  true  "Cursor 실행 요청"
// @Success      200      {object}  APICursorResponse "실행 성공"
// @Failure      400      {object}  ErrorResponse     "잘못된 요청"
// @Failure      500      {object}  ErrorResponse     "서버 오류"
// @Router       /api/cursor [post]
func HandleAPICursor(cfg *Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req APICursorRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid JSON payload: " + err.Error()})
			return
		}

		jobID := uuid.NewString()
		log.Printf("[%s] API 요청: prompt='%s', async=%v", jobID, req.Prompt, req.Async)

		// 프로젝트 경로 확인 (v1.2)
		projectPath, isSet := cfg.GetProjectPath()
		if !isSet {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: "프로젝트 경로가 설정되지 않았습니다. POST /api/config/project-path로 경로를 먼저 설정해주세요.",
			})
			return
		}

		// v1.3: DB에 작업 저장
		jobRecord := &database.JobRecord{
			ID:          jobID,
			Prompt:      req.Prompt,
			ProjectPath: projectPath,
			Status:      database.JobStatusPending,
			CreatedAt:   time.Now(),
		}
		if err := cfg.DB.CreateJob(jobRecord); err != nil {
			log.Printf("[%s] DB 작업 생성 실패: %v", jobID, err)
		}

		// v1.4: Worker Pool을 통해 작업 제출
		// API는 항상 비동기로 처리 (동시 실행 제어를 위해)
		// 동기 모드 요청도 Worker Pool을 통해 처리하되, 결과는 DB에서 조회해야 함
		
		// SlackCommandPayload 형식으로 변환 (API 요청용)
		slackPayload := types.SlackCommandPayload{
			Text:        req.Prompt,
			UserName:    "api-user",
			UserID:      "api",
			ResponseURL: "", // API는 response_url이 없음
		}

		// Job 생성 및 큐에 제출 (ConfigFull wrapper 생성)
		job := worker.Job{
			ID:         jobID,
			Payload:    slackPayload,
			ReceivedAt: time.Now(),
			Config:     cfg.ToWorkerConfig(),
		}

		// 비동기로 큐에 제출
		go func() {
			// 서버 종료 시 JobQueue가 닫힐 수 있으므로 panic 방지
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[%s] ⚠️ 작업 큐가 닫혀서 제출 실패: %v", jobID, r)
				}
			}()
			
			cfg.JobQueue <- job
			log.Printf("[%s] API 작업이 큐에 제출되었습니다.", jobID)
		}()

		// 비동기 모드: job_id만 즉시 반환
		if req.Async {
			c.JSON(http.StatusOK, APICursorResponse{
				Status:  "accepted",
				Message: "작업이 비동기로 시작되었습니다. GET /api/jobs/{id}로 결과를 조회하세요.",
				JobID:   jobID,
			})
			return
		}

		// 동기 모드: 작업 완료까지 대기 (최대 15분)
		// 주의: 이 방식은 HTTP 연결을 오래 유지하므로 권장하지 않지만,
		// 기존 API 호환성을 위해 유지
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()

		// DB에서 작업 완료 대기 (폴링)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				c.JSON(http.StatusRequestTimeout, APICursorResponse{
					Status:  "timeout",
					Message: "작업이 시간 내에 완료되지 않았습니다. GET /api/jobs/{id}로 결과를 조회하세요.",
					JobID:   jobID,
				})
				return

			case <-ticker.C:
				jobRecord, err := cfg.DB.GetJob(jobID)
				if err != nil {
					log.Printf("[%s] 작업 조회 오류: %v", jobID, err)
					continue
				}

				if jobRecord == nil {
					continue
				}

				// 작업 완료 확인
				if jobRecord.Status == database.JobStatusCompleted {
					c.JSON(http.StatusOK, APICursorResponse{
						Status:  "success",
						Message: "Cursor AI 작업이 완료되었습니다.",
						Output:  jobRecord.Output,
						JobID:   jobID,
					})
					return
				}

				if jobRecord.Status == database.JobStatusFailed {
					c.JSON(http.StatusInternalServerError, APICursorResponse{
						Status:  "error",
						Message: fmt.Sprintf("Cursor AI 실행 중 오류 발생: %s", jobRecord.Error),
						Output:  jobRecord.Output,
						JobID:   jobID,
					})
					return
				}

				// pending 또는 running 상태면 계속 대기
			}
		}
	}
}

// v1.4: executeCursorTask 함수는 제거됨
// 이제 Worker Pool의 TaskExecutor가 작업을 처리합니다.
// executeCursorCLI 함수도 TaskExecutor로 이동되었습니다.
// formatSuccessOutput, formatErrorOutput, sendMultipleMessages, sendDelayedResponse 등
// 관련 함수들도 TaskExecutor로 이동되어 더 이상 사용되지 않습니다.

// HandleGetProjectPath godoc
// @Summary      프로젝트 경로 조회 (v1.2)
// @Description  현재 설정된 프로젝트 경로를 조회합니다.
// @Tags         config
// @Produce      json
// @Success      200  {object}  ProjectPathResponse  "프로젝트 경로 정보"
// @Router       /api/config/project-path [get]
func HandleGetProjectPath(cfg *Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		path, isSet := cfg.GetProjectPath()

		if !isSet {
			c.JSON(http.StatusOK, ProjectPathResponse{
				Path:    "",
				IsSet:   false,
				Message: "프로젝트 경로가 설정되지 않았습니다. POST /api/config/project-path로 경로를 설정하세요.",
			})
			return
		}

		c.JSON(http.StatusOK, ProjectPathResponse{
			Path:    path,
			IsSet:   true,
			Message: "프로젝트 경로가 설정되어 있습니다.",
		})
	}
}

// HandleSetProjectPath godoc
// @Summary      프로젝트 경로 설정 (v1.2)
// @Description  cursor-agent가 실행될 프로젝트 경로를 설정합니다.
// @Description  이 경로는 런타임에 동적으로 변경 가능합니다.
// @Tags         config
// @Accept       json
// @Produce      json
// @Param        request  body      ProjectPathRequest   true  "프로젝트 경로"
// @Success      200      {object}  ProjectPathResponse  "경로 설정 성공"
// @Failure      400      {object}  ErrorResponse        "잘못된 요청"
// @Router       /api/config/project-path [post]
func HandleSetProjectPath(cfg *Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ProjectPathRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid JSON payload: " + err.Error()})
			return
		}

		// 경로 유효성 검사 (간단히 비어있지 않은지만 확인)
		if strings.TrimSpace(req.Path) == "" {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "프로젝트 경로는 비어있을 수 없습니다."})
			return
		}

		// 경로 설정
		cfg.SetProjectPath(req.Path)
		log.Printf("프로젝트 경로가 설정되었습니다: %s", req.Path)

		c.JSON(http.StatusOK, ProjectPathResponse{
			Path:    req.Path,
			IsSet:   true,
			Message: "프로젝트 경로가 성공적으로 설정되었습니다.",
		})
	}
}

// HandleGetJob godoc
// @Summary      작업 결과 조회 (v1.3)
// @Description  Job ID로 작업 실행 결과를 조회합니다.
// @Tags         jobs
// @Produce      json
// @Param        id   path      string  true  "Job ID"
// @Success      200  {object}  database.JobRecord  "작업 결과"
// @Failure      404  {object}  ErrorResponse       "작업을 찾을 수 없음"
// @Router       /api/jobs/{id} [get]
func HandleGetJob(cfg *Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		jobID := c.Param("id")

		job, err := cfg.DB.GetJob(jobID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "작업 조회 실패: " + err.Error()})
			return
		}

		if job == nil {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "작업을 찾을 수 없습니다."})
			return
		}

		c.JSON(http.StatusOK, job)
	}
}

// JobListQuery는 작업 목록 조회 쿼리 파라미터입니다
type JobListQuery struct {
	Limit  int                `form:"limit" example:"10"`
	Offset int                `form:"offset" example:"0"`
	Status database.JobStatus `form:"status" example:"completed"`
}

// HandleListJobs godoc
// @Summary      작업 목록 조회 (v1.3)
// @Description  작업 목록을 조회합니다. 상태별 필터링과 페이지네이션을 지원합니다.
// @Tags         jobs
// @Produce      json
// @Param        limit   query     int     false  "조회할 개수 (기본값: 10)"
// @Param        offset  query     int     false  "건너뛸 개수 (기본값: 0)"
// @Param        status  query     string  false  "작업 상태 필터 (pending/running/completed/failed)"
// @Success      200     {array}   database.JobRecord  "작업 목록"
// @Failure      400     {object}  ErrorResponse       "잘못된 요청"
// @Router       /api/jobs [get]
func HandleListJobs(cfg *Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 쿼리 파라미터 파싱
		limit := 10
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}

		offset := 0
		if o := c.Query("offset"); o != "" {
			if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
				offset = parsed
			}
		}

		status := database.JobStatus(c.Query("status"))

		jobs, err := cfg.DB.ListJobs(limit, offset, status)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "작업 목록 조회 실패: " + err.Error()})
			return
		}

		if jobs == nil {
			jobs = []*database.JobRecord{}
		}

		c.JSON(http.StatusOK, jobs)
	}
}

// Slack 명령어 핸들러 함수들

// handleHelpCommand shows available commands
func handleHelpCommand(c *gin.Context) {
	helpText := "📚 *Cursor AI 사용 가이드*\n\n" +
		"*🎯 코드 작업 요청:*\n" +
		"`/cursor \"프롬프트\"`\n" +
		"예: `/cursor \"main.go의 버그를 수정해줘\"`\n\n" +
		"*🔧 설정 명령어:*\n" +
		"• `/cursor set-path <경로>` - 프로젝트 경로 설정\n" +
		"• `/cursor path` - 현재 프로젝트 경로 확인\n\n" +
		"*📋 작업 조회:*\n" +
		"• `/cursor list` - 최근 작업 목록 보기 (최근 10개)\n" +
		"• `/cursor show <job-id>` - 특정 작업 결과 상세 보기\n\n" +
		"*❓ 도움말:*\n" +
		"• `/cursor help` - 이 도움말 표시\n\n" +
		"💡 *사용 팁:*\n" +
		"1. 처음 사용 시 `set-path`로 프로젝트 경로 설정\n" +
		"2. 자연어로 편하게 요청하세요\n" +
		"3. 작업 ID는 `list` 명령어로 확인 가능"

	c.JSON(http.StatusOK, gin.H{
		"response_type": "ephemeral",
		"text":          helpText,
	})
}

// handleListCommand shows recent jobs
func handleListCommand(c *gin.Context, cfg *Config, userID string) {
	if cfg.DB == nil {
		c.JSON(http.StatusOK, gin.H{
			"response_type": "ephemeral",
			"text":          "❌ 데이터베이스가 초기화되지 않았습니다.",
		})
		return
	}

	// Get user's recent jobs (최근 10개)
	jobs, err := cfg.DB.ListJobs(10, 0, "")
	if err != nil {
		log.Printf("작업 목록 조회 실패: %v", err)
		c.JSON(http.StatusOK, gin.H{
			"response_type": "ephemeral",
			"text":          "❌ 작업 목록을 가져오는 중 오류가 발생했습니다.",
		})
		return
	}

	if len(jobs) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"response_type": "ephemeral",
			"text":          "📋 아직 실행된 작업이 없습니다.\n\n💡 사용법: `/cursor \"프롬프트\"`",
		})
		return
	}

	// Build response
	var response strings.Builder
	response.WriteString("📋 *최근 작업 목록* (최근 10개)\n\n")

	for _, job := range jobs {
		// Status emoji
		var statusEmoji string
		switch job.Status {
		case "completed":
			statusEmoji = "✅"
		case "failed":
			statusEmoji = "❌"
		case "running":
			statusEmoji = "⏳"
		case "pending":
			statusEmoji = "🕐"
		default:
			statusEmoji = "❓"
		}

		// Time ago
		timeAgo := timeAgoString(job.CreatedAt)
		
		// Truncate prompt if too long
		prompt := job.Prompt
		if len(prompt) > 50 {
			prompt = prompt[:47] + "..."
		}

		response.WriteString(fmt.Sprintf("%s `%s` - \"%s\" (%s)\n", 
			statusEmoji, job.ID[:8], prompt, timeAgo))
	}

	response.WriteString("\n💡 *결과 확인:* `/cursor show <job-id>`")

	c.JSON(http.StatusOK, gin.H{
		"response_type": "ephemeral",
		"text":          response.String(),
	})
}

// handleShowCommand shows job details
func handleShowCommand(c *gin.Context, cfg *Config, jobID string) {
	if cfg.DB == nil {
		c.JSON(http.StatusOK, gin.H{
			"response_type": "ephemeral",
			"text":          "❌ 데이터베이스가 초기화되지 않았습니다.",
		})
		return
	}

	job, err := cfg.DB.GetJob(jobID)
	if err != nil {
		log.Printf("작업 조회 실패 (%s): %v", jobID, err)
		c.JSON(http.StatusOK, gin.H{
			"response_type": "ephemeral",
			"text":          fmt.Sprintf("❌ 작업을 찾을 수 없습니다: `%s`", jobID),
		})
		return
	}

	// nil 체크 추가 (v1.4.1: panic 방지)
	if job == nil {
		log.Printf("작업이 nil 반환됨 (%s)", jobID)
		c.JSON(http.StatusOK, gin.H{
			"response_type": "ephemeral",
			"text":          fmt.Sprintf("❌ 작업을 찾을 수 없습니다: `%s`\n\n💡 `/cursor list` 명령어로 최근 작업 목록을 확인하세요.", jobID),
		})
		return
	}

	// Status emoji and text
	var statusEmoji, statusText string
	switch job.Status {
	case "completed":
		statusEmoji = "✅"
		statusText = "완료"
	case "failed":
		statusEmoji = "❌"
		statusText = "실패"
	case "running":
		statusEmoji = "⏳"
		statusText = "실행 중"
	case "pending":
		statusEmoji = "🕐"
		statusText = "대기 중"
	default:
		statusEmoji = "❓"
		statusText = "알 수 없음"
	}

	// Build response
	var response strings.Builder
	response.WriteString(fmt.Sprintf("📦 *작업 결과* (ID: `%s`)\n\n", job.ID[:8]))
	response.WriteString(fmt.Sprintf("*프롬프트:* \"%s\"\n", job.Prompt))
	response.WriteString(fmt.Sprintf("*상태:* %s %s\n", statusEmoji, statusText))
	response.WriteString(fmt.Sprintf("*생성 시간:* %s\n", job.CreatedAt.Format("2006-01-02 15:04:05")))
	
	// v1.4.1: 올바른 소요 시간 계산 (completed_at - started_at)
	if job.StartedAt != nil && !job.StartedAt.IsZero() {
		if job.CompletedAt != nil && !job.CompletedAt.IsZero() {
			// 완료된 작업: completed_at - started_at
			duration := job.CompletedAt.Sub(*job.StartedAt)
			response.WriteString(fmt.Sprintf("*소요 시간:* %s\n", duration.Round(time.Second)))
		} else if job.Status == "running" {
			// 실행 중인 작업: 현재 시간 - started_at
			duration := time.Since(*job.StartedAt)
			response.WriteString(fmt.Sprintf("*실행 중:* %s 경과\n", duration.Round(time.Second)))
		} else {
			// failed/pending 등 completed_at이 없는 경우
			response.WriteString("*소요 시간:* -\n")
		}
	}

	// Output or error
	if job.Status == "completed" && job.Output != "" {
		output := job.Output
		if len(output) > 1000 {
			output = output[:997] + "..."
		}
		// 마크다운 렌더링을 위해 코드블록 제거
		response.WriteString(fmt.Sprintf("\n📝 *출력:*\n%s", output))
	} else if job.Status == "failed" && job.Error != "" {
		// 에러는 코드블록 유지 (에러 메시지는 일반 텍스트)
		response.WriteString(fmt.Sprintf("\n❌ *오류:*\n```\n%s\n```", job.Error))
	}

	c.JSON(http.StatusOK, gin.H{
		"response_type": "ephemeral",
		"text":          response.String(),
	})
}

// handlePathCommand shows current project path
func handlePathCommand(c *gin.Context, cfg *Config) {
	path, isSet := cfg.GetProjectPath()
	
	if !isSet || path == "" {
		c.JSON(http.StatusOK, gin.H{
			"response_type": "ephemeral",
			"text":          "❌ 프로젝트 경로가 설정되지 않았습니다.\n\n💡 설정하기: `/cursor set-path /path/to/project`",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"response_type": "ephemeral",
		"text":          fmt.Sprintf("📁 *현재 프로젝트 경로*\n`%s`\n\n💡 변경하기: `/cursor set-path <새경로>`", path),
	})
}

// timeAgoString returns a human-readable time ago string
func timeAgoString(t time.Time) string {
	duration := time.Since(t)
	
	if duration < time.Minute {
		return "방금 전"
	} else if duration < time.Hour {
		return fmt.Sprintf("%d분 전", int(duration.Minutes()))
	} else if duration < 24*time.Hour {
		return fmt.Sprintf("%d시간 전", int(duration.Hours()))
	} else {
		return fmt.Sprintf("%d일 전", int(duration.Hours()/24))
	}
}

// Slack Options API for autocomplete

type SlackOption struct {
	Text  string `json:"text"`
	Value string `json:"value"`
}

type SlackOptionsResponse struct {
	Options []SlackOption `json:"options"`
}

// HandleSlackOptions provides autocomplete options for Slack commands
func HandleSlackOptions(cfg *Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Parse the payload
		var payload struct {
			Value string `form:"value" json:"value"`
		}
		
		if err := c.ShouldBind(&payload); err != nil {
			c.JSON(http.StatusOK, SlackOptionsResponse{Options: []SlackOption{}})
			return
		}

		// Provide command suggestions based on current input
		options := []SlackOption{
			{Text: "help - 도움말 보기", Value: "help"},
			{Text: "list - 최근 작업 목록", Value: "list"},
			{Text: "path - 현재 경로 확인", Value: "path"},
			{Text: "set-path <경로> - 프로젝트 경로 설정", Value: "set-path "},
			{Text: "show <job-id> - 작업 결과 보기", Value: "show "},
		}

		c.JSON(http.StatusOK, SlackOptionsResponse{Options: options})
	}
}

