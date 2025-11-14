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

	// 3. 결과 포맷팅 (마크다운 적용)
	rawOutput := string(output)
	var messages []string
	
	if err != nil {
		log.Printf("[%s] 실행 오류: %v, output: %s", jobID, err, rawOutput)
		
		// v1.3: 실패 결과 저장
		cfg.DB.UpdateJobResult(jobID, rawOutput, err.Error())
		cfg.DB.UpdateJobStatus(jobID, database.JobStatusFailed)
		
		// 에러 메시지 포맷팅
		messages = formatErrorOutput(jobID, err, rawOutput)
	} else {
		log.Printf("[%s] 실행 완료", jobID)
		
		// v1.3: 성공 결과 저장
		cfg.DB.UpdateJobResult(jobID, rawOutput, "")
		cfg.DB.UpdateJobStatus(jobID, database.JobStatusCompleted)
		
		// 성공 메시지 포맷팅
		messages = formatSuccessOutput(jobID, rawOutput, prompt)
	}

	// 4. 결과 전송 (SSRF 방어 추가, 마크다운 적용, 분할 전송)
	sendMultipleMessages(payload.ResponseURL, messages, jobID, cfg.AllowedResponseDomains)
}


// executeCursorCLI는 cursor-agent를 안전하게 실행합니다.
// v1.1: --force 플래그 추가, --files 제거, Process Group 관리
func executeCursorCLI(jobID string, prompt string, projectPath string, cursorCLIPath string) ([]byte, error) {
	// 1. 타임아웃 컨텍스트 생성 (15분)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
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
	SetupProcessGroup(cmd)

	log.Printf("[%s] Executing: %s %s (in %s)", jobID, cursorCLIPath, strings.Join(args, " "), cmd.Dir)

	// 5. 실행 및 결과 수집 (stdout + stderr)
	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb

	err := cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("명령어 시작 실패: %w", err)
	}

	// 5.5. cmd.Wait()를 별도 goroutine에서 실행하고 타임아웃과 동시에 처리
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	// 타임아웃 또는 완료 대기
	select {
	case <-ctx.Done():
		// 타임아웃 발생 - 프로세스 그룹 강제 종료
		log.Printf("[%s] 작업 시간 초과 (15분). 프로세스 그룹 강제 종료 시도...", jobID)
		if err := KillProcessGroup(cmd); err != nil {
			log.Printf("[%s] 프로세스 종료 실패: %v", jobID, err)
		}
		// cmd.Wait()가 완료될 때까지 잠시 대기 (최대 2초)
		select {
		case <-done:
			// 프로세스가 종료됨
		case <-time.After(2 * time.Second):
			// 강제 종료 후에도 종료되지 않으면 로그만 남김
			log.Printf("[%s] 프로세스 종료 대기 시간 초과", jobID)
		}
		// 출력 결합
		combinedOutput := append(outb.Bytes(), errb.Bytes()...)
		return combinedOutput, fmt.Errorf("명령어 실행 시간 초과 (15분)")

	case err = <-done:
		// 정상 완료 또는 에러
		// 출력 결합
		combinedOutput := append(outb.Bytes(), errb.Bytes()...)
		if err != nil {
			return combinedOutput, fmt.Errorf("cursor-agent 실행 실패: %w", err)
		}
		return combinedOutput, nil
	}
}

// formatSuccessOutput은 cursor-agent 성공 출력을 Slack 마크다운으로 포맷팅합니다.
// 반환값: 메시지 배열 (40,000자씩 분할)
func formatSuccessOutput(jobID string, rawOutput string, prompt string) []string {
	var result strings.Builder
	result.WriteString("✅ *Cursor AI 작업 완료*\n\n")
	result.WriteString(fmt.Sprintf("📝 *요청 프롬프트*\n> %s\n\n", prompt))
	
	// cursor-agent 출력 파싱
	lines := strings.Split(rawOutput, "\n")
	
	// 변경된 파일 목록 추출
	modifiedFiles := extractModifiedFiles(lines)
	if len(modifiedFiles) > 0 {
		result.WriteString("📁 *변경된 파일*\n")
		for _, file := range modifiedFiles {
			result.WriteString(fmt.Sprintf("• `%s`\n", file))
		}
		result.WriteString("\n")
	}
	
	// 주요 변경 사항 추출 (diff가 있으면 표시)
	changes := extractChangeSummary(lines)
	if changes != "" {
		result.WriteString("🔧 *주요 변경 사항*\n")
		result.WriteString(changes)
		result.WriteString("\n")
	}
	
	// 원본 출력 (마크다운 렌더링을 위해 코드블록 제거)
	result.WriteString("📄 *실행 결과*\n")
	result.WriteString(rawOutput)
	result.WriteString(fmt.Sprintf("\n\n🆔 Job ID: `%s`", jobID[:8]))
	
	// 메시지를 40,000자 단위로 분할
	return splitMessage(result.String())
}

// formatErrorOutput은 에러 출력을 Slack 마크다운으로 포맷팅합니다.
// 반환값: 메시지 배열 (40,000자씩 분할)
func formatErrorOutput(jobID string, err error, rawOutput string) []string {
	var result strings.Builder
	result.WriteString("❌ *Cursor AI 실행 중 오류 발생*\n\n")
	result.WriteString(fmt.Sprintf("🚨 *오류 메시지*\n> %s\n\n", err.Error()))
	
	if rawOutput != "" {
		result.WriteString("📄 *출력 내용*\n")
		result.WriteString("```\n")
		result.WriteString(rawOutput)
		result.WriteString("\n```\n")
	}
	
	result.WriteString(fmt.Sprintf("\n💡 자세한 정보: `/cursor show %s`", jobID[:8]))
	
	// 메시지를 40,000자 단위로 분할
	return splitMessage(result.String())
}

// extractModifiedFiles는 cursor-agent 출력에서 변경된 파일 목록을 추출합니다.
func extractModifiedFiles(lines []string) []string {
	var files []string
	filePattern := []string{
		"Modified:",
		"Created:",
		"Deleted:",
		"Updated:",
		"File:",
		"✓",
		"modified:",
		"created:",
	}
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		for _, pattern := range filePattern {
			if strings.Contains(strings.ToLower(line), strings.ToLower(pattern)) {
				// 파일명 추출 시도
				parts := strings.Fields(line)
				for _, part := range parts {
					// .go, .js, .ts, .py 등 파일 확장자가 있는 경우
					if strings.Contains(part, ".") && !strings.HasPrefix(part, ".") {
						// 특수 문자 제거
						file := strings.Trim(part, "`:,;\"'")
						if file != "" && !stringSliceContains(files, file) {
							files = append(files, file)
						}
					}
				}
			}
		}
	}
	
	return files
}

// extractChangeSummary는 변경 사항 요약을 추출합니다.
func extractChangeSummary(lines []string) string {
	var summary strings.Builder
	inDiff := false
	diffCount := 0
	maxDiffLines := 20 // 최대 20줄까지만 표시
	
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		// diff 시작 감지
		if strings.HasPrefix(trimmed, "diff --git") || 
		   strings.HasPrefix(trimmed, "---") || 
		   strings.HasPrefix(trimmed, "+++") {
			inDiff = true
			continue
		}
		
		// diff 내용 (+ or - 로 시작)
		if inDiff && (strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "-")) {
			if diffCount < maxDiffLines {
				if strings.HasPrefix(trimmed, "+") && !strings.HasPrefix(trimmed, "+++") {
					summary.WriteString(fmt.Sprintf("• ➕ %s\n", strings.TrimPrefix(trimmed, "+")))
					diffCount++
				} else if strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "---") {
					summary.WriteString(fmt.Sprintf("• ➖ %s\n", strings.TrimPrefix(trimmed, "-")))
					diffCount++
				}
			}
		}
		
		// Summary, Changes 등의 섹션 추출
		if strings.HasPrefix(strings.ToLower(trimmed), "summary:") ||
		   strings.HasPrefix(strings.ToLower(trimmed), "changes:") {
			summary.WriteString(fmt.Sprintf("%s\n", trimmed))
		}
	}
	
	if diffCount >= maxDiffLines {
		summary.WriteString("• ... (더 많은 변경 사항이 있습니다)\n")
	}
	
	return summary.String()
}

// stringSliceContains는 문자열 슬라이스에 특정 문자열이 있는지 확인합니다.
func stringSliceContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// splitMessage는 메시지를 Slack 최대 크기(40,000자)로 분할합니다.
func splitMessage(message string) []string {
	const maxSlackMessageSize = 40000
	const maxMessages = 5 // Slack response_url 최대 호출 횟수
	
	// 메시지가 최대 크기 이하면 그대로 반환
	if len(message) <= maxSlackMessageSize {
		return []string{message}
	}
	
	var messages []string
	remaining := message
	
	for len(remaining) > 0 && len(messages) < maxMessages {
		chunkSize := maxSlackMessageSize
		
		// 남은 메시지가 최대 크기보다 작으면 전부 추가
		if len(remaining) <= chunkSize {
			messages = append(messages, remaining)
			break
		}
		
		// 코드 블록(```)이 중간에 잘리지 않도록 조정
		chunk := remaining[:chunkSize]
		
		// 마지막 줄바꿈 위치 찾기 (자연스러운 분할)
		lastNewline := strings.LastIndex(chunk, "\n")
		if lastNewline > maxSlackMessageSize-1000 { // 너무 많이 자르지 않도록
			chunkSize = lastNewline + 1
			chunk = remaining[:chunkSize]
		}
		
		messages = append(messages, chunk)
		remaining = remaining[chunkSize:]
	}
	
	// 마지막 메시지가 너무 길면 경고 추가
	if len(remaining) > 0 {
		log.Printf("메시지가 너무 길어서 %d자가 잘렸습니다.", len(remaining))
		lastMsg := messages[len(messages)-1]
		messages[len(messages)-1] = lastMsg + fmt.Sprintf("\n\n⚠️ 메시지가 너무 길어서 %d자가 생략되었습니다.", len(remaining))
	}
	
	// 페이지 번호 추가 (여러 메시지인 경우)
	if len(messages) > 1 {
		for i := range messages {
			pageInfo := fmt.Sprintf("\n\n📄 페이지 %d/%d", i+1, len(messages))
			messages[i] = pageInfo + "\n" + messages[i]
		}
	}
	
	return messages
}

// sendMultipleMessages는 여러 메시지를 순차적으로 전송합니다.
func sendMultipleMessages(responseURL string, messages []string, jobID string, allowedDomains []string) {
	for i, message := range messages {
		log.Printf("[%s] 메시지 전송 (%d/%d): %d자", jobID, i+1, len(messages), len(message))
		sendDelayedResponse(responseURL, message, allowedDomains)
		
		// 메시지 간 짧은 대기 (Slack rate limit 방지)
		if i < len(messages)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
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
	response.WriteString(fmt.Sprintf("*실행 시간:* %s\n", job.CreatedAt.Format("2006-01-02 15:04:05")))
	
	if job.StartedAt != nil && !job.StartedAt.IsZero() {
		duration := time.Since(*job.StartedAt)
		response.WriteString(fmt.Sprintf("*소요 시간:* %s\n", duration.Round(time.Second)))
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

