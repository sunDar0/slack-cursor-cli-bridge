package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kakaovx/cursor-slack-server/internal/database"
	"github.com/kakaovx/cursor-slack-server/internal/server/middleware"
)

// SlackCommandPayload는 Slack이 보내는 폼 데이터를 바인딩합니다.
// v1.1: 자연어 프롬프트 방식 (파일명을 프롬프트에 포함)
type SlackCommandPayload struct {
	Text        string `form:"text" example:"main.go의 버그를 수정해줘"`
	UserName    string `form:"user_name" example:"john_doe"`
	UserID      string `form:"user_id" example:"U1234567890"`
	ResponseURL string `form:"response_url" example:"https://hooks.slack.com/commands/1234567890/1234567890/abcdefghijklmnopqrstuvwxyz"`
	TriggerID   string `form:"trigger_id" example:"1234567890.1234567890.abcdefghijklmnopqrstuvwxyz"`
}

// SlackDelayedResponse는 Slack 지연 응답용 JSON 구조체입니다.
type SlackDelayedResponse struct {
	Text         string `json:"text" example:"✅ Cursor AI 작업 완료"`
	ResponseType string `json:"response_type" example:"in_channel"` // "in_channel" 또는 "ephemeral"
}

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
		var payload SlackCommandPayload

		if err := c.ShouldBind(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}

		// v1.2: set-path 커맨드 처리
		text := strings.TrimSpace(payload.Text)
		if strings.HasPrefix(text, "set-path ") {
			// set-path 커맨드 처리
			path := strings.TrimSpace(strings.TrimPrefix(text, "set-path "))
			if path == "" {
				c.JSON(http.StatusOK, gin.H{
					"response_type": "ephemeral",
					"text":          "❌ 경로를 입력해주세요.\n사용법: `/cursor set-path /path/to/project`",
				})
				return
			}

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
			"text":          "⏳ " + payload.UserName + "님의 요청을 접수했습니다. 작업을 처리 중입니다...",
		})

		// 2. 비동기로 cursor-agent 실행
		reqID, exists := c.Get(middleware.RequestIDKey)
		if !exists {
			reqID = uuid.NewString()
		}

		go executeCursorTask(reqID.(string), payload, cfg)
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

		// 비동기 모드
		if req.Async {
			// 비동기로 실행하고 job_id만 즉시 반환
			go func() {
				// 작업 시작 상태 업데이트
				cfg.DB.UpdateJobStatus(jobID, database.JobStatusRunning)

				output, err := executeCursorCLI(jobID, req.Prompt, projectPath, cfg.CursorCLIPath)
				
				// v1.3: 결과 저장
				if err != nil {
					log.Printf("[%s] API 비동기 실행 오류: %v, output: %s", jobID, err, string(output))
					cfg.DB.UpdateJobResult(jobID, string(output), err.Error())
					cfg.DB.UpdateJobStatus(jobID, database.JobStatusFailed)
				} else {
					log.Printf("[%s] API 비동기 실행 완료", jobID)
					cfg.DB.UpdateJobResult(jobID, string(output), "")
					cfg.DB.UpdateJobStatus(jobID, database.JobStatusCompleted)
				}
			}()

			c.JSON(http.StatusOK, APICursorResponse{
				Status:  "accepted",
				Message: "작업이 비동기로 시작되었습니다. (현재 점 단계에서는 결과 조회 미지원)",
				JobID:   jobID,
			})
			return
		}

		// 동기 모드 - 실행 결과를 즉시 반환
		cfg.DB.UpdateJobStatus(jobID, database.JobStatusRunning)
		output, err := executeCursorCLI(jobID, req.Prompt, projectPath, cfg.CursorCLIPath)

		if err != nil {
			log.Printf("[%s] API 동기 실행 오류: %v", jobID, err)
			// v1.3: 실패 결과 저장
			cfg.DB.UpdateJobResult(jobID, string(output), err.Error())
			cfg.DB.UpdateJobStatus(jobID, database.JobStatusFailed)
			
			c.JSON(http.StatusInternalServerError, APICursorResponse{
				Status:  "error",
				Message: fmt.Sprintf("Cursor AI 실행 중 오류 발생: %v", err),
				Output:  string(output),
				JobID:   jobID,
			})
			return
		}

		log.Printf("[%s] API 동기 실행 완료", jobID)
		// v1.3: 성공 결과 저장
		cfg.DB.UpdateJobResult(jobID, string(output), "")
		cfg.DB.UpdateJobStatus(jobID, database.JobStatusCompleted)
		
		c.JSON(http.StatusOK, APICursorResponse{
			Status:  "success",
			Message: "Cursor AI 작업이 완료되었습니다.",
			Output:  string(output),
			JobID:   jobID,
		})
	}
}

// executeCursorTask는 비동기적으로 cursor-agent를 실행하고 결과를 Slack에 전송합니다.
// v1.1: 자연어 프롬프트 방식으로 단순화
// v1.2: 동적 프로젝트 경로 지원
// v1.3: DB에 작업 결과 저장
func executeCursorTask(jobID string, payload SlackCommandPayload, cfg *Config) {
	log.Printf("[%s] 작업 시작: user=%s, text=%s", jobID, payload.UserName, payload.Text)

	// 1. 프롬프트 추출 (v1.1: 단순화)
	prompt := strings.TrimSpace(payload.Text)

	if prompt == "" {
		errMsg := "❌ 프롬프트가 비어있습니다. 사용법: /cursor \"자연어 프롬프트\"\n예시: /cursor \"main.go의 버그를 수정해줘\""
		log.Printf("[%s] %s", jobID, errMsg)
		sendDelayedResponse(payload.ResponseURL, errMsg, cfg.AllowedResponseDomains)
		return
	}

	// 1.5. 프로젝트 경로 확인 (v1.2)
	projectPath, isSet := cfg.GetProjectPath()
	if !isSet {
		errMsg := "❌ 프로젝트 경로가 설정되지 않았습니다.\n" +
			"먼저 `/cursor set-path <프로젝트_경로>` 명령어로 경로를 설정해주세요.\n" +
			"예시: `/cursor set-path /Users/username/projects/my-project`"
		log.Printf("[%s] %s", jobID, errMsg)
		sendDelayedResponse(payload.ResponseURL, errMsg, cfg.AllowedResponseDomains)
		return
	}

	// v1.3: DB에 작업 생성
	jobRecord := &database.JobRecord{
		ID:          jobID,
		Prompt:      prompt,
		ProjectPath: projectPath,
		Status:      database.JobStatusPending,
		UserID:      payload.UserID,
		UserName:    payload.UserName,
		CreatedAt:   time.Now(),
	}
	if err := cfg.DB.CreateJob(jobRecord); err != nil {
		log.Printf("[%s] DB 작업 생성 실패: %v", jobID, err)
	}

	// 작업 시작
	cfg.DB.UpdateJobStatus(jobID, database.JobStatusRunning)

	// 2. cursor-agent 실행 (v1.1: --force 추가, --files 제거)
	output, err := executeCursorCLI(jobID, prompt, projectPath, cfg.CursorCLIPath)

	// 3. 결과 포맷팅
	resultMessage := string(output)
	if err != nil {
		log.Printf("[%s] 실행 오류: %v, output: %s", jobID, err, resultMessage)
		resultMessage = fmt.Sprintf("❌ Cursor AI 실행 중 에러 발생:\n%v\n\n출력:\n%s", err, resultMessage)
		
		// v1.3: 실패 결과 저장
		cfg.DB.UpdateJobResult(jobID, string(output), err.Error())
		cfg.DB.UpdateJobStatus(jobID, database.JobStatusFailed)
	} else {
		log.Printf("[%s] 실행 완료", jobID)
		resultMessage = fmt.Sprintf("✅ Cursor AI 작업 완료:\n\n%s", resultMessage)
		
		// v1.3: 성공 결과 저장
		cfg.DB.UpdateJobResult(jobID, string(output), "")
		cfg.DB.UpdateJobStatus(jobID, database.JobStatusCompleted)
	}

	// 4. 결과 전송 (SSRF 방어 추가)
	sendDelayedResponse(payload.ResponseURL, "```\n"+resultMessage+"\n```", cfg.AllowedResponseDomains)
}


// executeCursorCLI는 cursor-agent를 안전하게 실행합니다.
// v1.1: --force 플래그 추가, --files 제거, Process Group 관리
func executeCursorCLI(jobID string, prompt string, projectPath string, cursorCLIPath string) ([]byte, error) {
	// 1. 타임아웃 컨텍스트 생성 (120초)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 2. 명령어 인자 생성 (v1.1: --force 필수, --files 제거)
	args := []string{
		"-p", prompt,              // 자연어 프롬프트 (파일명 포함)
		"--force",                 // 파일 수정 허용 (필수!)
		"--output-format", "text", // 텍스트 출력
	}

	cmd := exec.CommandContext(ctx, cursorCLIPath, args...)

	// 3. (보안) 작업 디렉토리 격리
	cmd.Dir = projectPath

	// 4. (보안 핵심) 자식 프로세스까지 함께 종료하기 위해 Process Group 설정
	// 타임아웃 시 좀비 프로세스 방지
	setupProcessGroup(cmd)

	log.Printf("[%s] Executing: %s %s (in %s)", jobID, cursorCLIPath, strings.Join(args, " "), cmd.Dir)

	// 5. 실행 및 결과 수집 (stdout + stderr)
	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb

	err := cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("명령어 시작 실패: %w", err)
	}

	err = cmd.Wait()

	// 6. 출력 결합
	combinedOutput := append(outb.Bytes(), errb.Bytes()...)

	// 7. 에러 처리 (타임아웃 확인)
	if ctx.Err() == context.DeadlineExceeded {
		log.Printf("[%s] 작업 시간 초과 (120초). 프로세스 그룹 강제 종료 시도...", jobID)
		// (보안 핵심) 프로세스 그룹 전체를 강제 종료
		if err := killProcessGroup(cmd); err != nil {
			log.Printf("[%s] 프로세스 종료 실패: %v", jobID, err)
		}
		return combinedOutput, fmt.Errorf("명령어 실행 시간 초과 (120초)")
	}

	if err != nil {
		return combinedOutput, fmt.Errorf("cursor-agent 실행 실패: %w", err)
	}

	return combinedOutput, nil
}

// sendDelayedResponse는 SSRF 공격을 방지하기 위해 ResponseURL을 검증한 후 전송합니다.
// v1.1: SSRF 방어 추가
func sendDelayedResponse(responseURL string, message string, allowedDomains []string) {
	// 1. (보안 핵심) SSRF 방어를 위한 URL 검증
	parsedURL, err := url.Parse(responseURL)
	if err != nil {
		log.Printf("SSRF 방어: 유효하지 않은 ResponseURL: %s", responseURL)
		return
	}

	// 2. 스킴(Scheme) 검증
	if parsedURL.Scheme != "https" {
		log.Printf("SSRF 방어: 'https'가 아닌 스킴 차단: %s", parsedURL.Scheme)
		return
	}

	// 3. 허용 목록(Allow-list) 기반 도메인 검증
	isAllowed := false
	for _, allowedDomain := range allowedDomains {
		if parsedURL.Hostname() == allowedDomain || strings.HasSuffix(parsedURL.Hostname(), "."+allowedDomain) {
			isAllowed = true
			break
		}
	}

	if !isAllowed {
		log.Printf("SSRF 방어: 허용되지 않는 도메인으로의 응답 시도 차단: %s", responseURL)
		return
	}

	// 4. Slack 응답 전송
	payload := SlackDelayedResponse{
		Text:         message,
		ResponseType: "in_channel", // 채널에 공개
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshaling delayed response: %v", err)
		return
	}

	resp, err := http.Post(responseURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		log.Printf("Error sending delayed response to %s: %v", responseURL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Slack delayed response returned non-200 status: %d", resp.StatusCode)
	}
}

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

