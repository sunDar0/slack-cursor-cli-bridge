package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/kakaovx/cursor-slack-server/internal/database"
	"github.com/kakaovx/cursor-slack-server/internal/server"
)

// TaskExecutor는 실제 cursor-agent 작업을 실행하고 모든 보안 검증을 수행합니다.
type TaskExecutor struct {
	allowedResponseDomains []string // (SSRF 방어) 허용 도메인
}

// NewTaskExecutor는 TaskExecutor의 인스턴스를 생성합니다.
func NewTaskExecutor(allowedDomains []string) *TaskExecutor {
	return &TaskExecutor{
		allowedResponseDomains: allowedDomains,
	}
}

// Run은 Job을 받아 (1)검증 -> (2)실행 -> (3)응답의 전체 파이프라인을 수행합니다.
func (te *TaskExecutor) Run(job Job) {
	payload := job.Payload
	responseURL := payload.ResponseURL
	jobID := job.ID
	cfg := job.Config

	// 1. 프롬프트 추출 (v1.1: 단순화)
	prompt := strings.TrimSpace(payload.Text)

	if prompt == "" {
		errMsg := "❌ 프롬프트가 비어있습니다. 사용법: /cursor \"자연어 프롬프트\""
		log.Printf("[%s] %s", jobID, errMsg)
		te.sendDelayedResponse(responseURL, errMsg)
		return
	}

	// 1.5. 프로젝트 경로 확인 (v1.2)
	projectPath, isSet := cfg.GetProjectPath()
	if !isSet {
		errMsg := "❌ 프로젝트 경로가 설정되지 않았습니다.\n" +
			"먼저 `/cursor set-path <프로젝트_경로>` 명령어로 경로를 설정해주세요.\n" +
			"예시: `/cursor set-path /Users/username/projects/my-project`"
		log.Printf("[%s] %s", jobID, errMsg)
		te.sendDelayedResponse(responseURL, errMsg)
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

	// 진행 상황 업데이트를 위한 channel
	progressDone := make(chan struct{})
	
	// 주기적으로 진행 상황 전송 (2분마다, 최대 4회)
	go te.sendProgressUpdates(jobID, responseURL, progressDone)

	// 2. cursor-agent 실행 (v1.1: --force 추가, --files 제거)
	log.Printf("[%s] 작업자 실행 시작: prompt='%s'", jobID, prompt)
	output, err := te.executeCursorCommand(jobID, prompt, projectPath, cfg.CursorCLIPath)
	
	// 진행 상황 업데이트 중지
	close(progressDone)

	// 3. 결과 포맷팅
	rawOutput := string(output)
	if err != nil {
		log.Printf("[%s] 작업자 실행 오류: %v, output: %s", jobID, err, rawOutput)

		// v1.3: 실패 결과 저장
		cfg.DB.UpdateJobResult(jobID, rawOutput, err.Error())
		cfg.DB.UpdateJobStatus(jobID, database.JobStatusFailed)
		
		// 에러 메시지 포맷팅 (마크다운 적용)
		messages := te.formatErrorOutput(jobID, err, rawOutput)
		te.sendMultipleMessages(responseURL, messages, jobID)
	} else {
		log.Printf("[%s] 작업자 실행 완료.", jobID)

		// v1.3: 성공 결과 저장
		cfg.DB.UpdateJobResult(jobID, rawOutput, "")
		cfg.DB.UpdateJobStatus(jobID, database.JobStatusCompleted)
		
		// 성공 메시지 포맷팅 (마크다운 적용, before/after 표시)
		messages := te.formatSuccessOutput(jobID, rawOutput, prompt)
		te.sendMultipleMessages(responseURL, messages, jobID)
	}
}

// executeCursorCommand는 context.WithTimeout과 process group kill을 사용하여
// cursor-agent를 안전하게 실행합니다.
func (te *TaskExecutor) executeCursorCommand(jobID string, prompt string, projectPath string, cursorCLIPath string) ([]byte, error) {
	// 1. 타임아웃 컨텍스트 생성 (15분)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// 2. 명령어 인자 생성 (v1.1: --force 필수, --files 제거)
	args := []string{
		"-p", prompt,              // 자연어 프롬프트 (파일명 포함 가능)
		"--force",                 // 파일 수정 허용 (필수!)
		"--output-format", "text", // 텍스트 출력
	}

	cmd := exec.CommandContext(ctx, cursorCLIPath, args...)

	// 3. (보안) 작업 디렉토리 격리
	cmd.Dir = projectPath

	// 4. (보안 핵심) 자식 프로세스까지 함께 종료하기 위해 Process Group 설정
	// 타임아웃 시 좀비 프로세스 방지
	server.SetupProcessGroup(cmd)

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
		if err := server.KillProcessGroup(cmd); err != nil {
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

// sendProgressUpdates는 작업 진행 중 주기적으로 상태를 Slack에 전송합니다
func (te *TaskExecutor) sendProgressUpdates(jobID string, responseURL string, done <-chan struct{}) {
	ticker := time.NewTicker(2 * time.Minute) // 2분마다 업데이트
	defer ticker.Stop()
	
	elapsed := 0
	maxUpdates := 4 // Slack 제한(5번) - 1 (최종 결과용)
	updateCount := 0
	
	for {
		select {
		case <-done:
			// 작업 완료, goroutine 종료
			log.Printf("[%s] 진행 상황 업데이트 종료 (총 %d회 전송)", jobID, updateCount)
			return
		case <-ticker.C:
			elapsed += 120 // 2분 = 120초
			updateCount++
			
			// 최대 업데이트 횟수 제한
			if updateCount > maxUpdates {
				log.Printf("[%s] 최대 업데이트 횟수 도달", jobID)
				return
			}
			
			// 분/초 표시
			minutes := elapsed / 60
			seconds := elapsed % 60
			var timeStr string
			if seconds == 0 {
				timeStr = fmt.Sprintf("%d분", minutes)
			} else {
				timeStr = fmt.Sprintf("%d분 %d초", minutes, seconds)
			}
			
			message := fmt.Sprintf("⏳ 작업이 %s 경과되었습니다... (처리 중)", timeStr)
			log.Printf("[%s] 진행 상황 업데이트: %s", jobID, timeStr)
			
			// 진행 상황 메시지 전송
			te.sendProgressMessage(responseURL, message)
		}
	}
}

// sendProgressMessage는 진행 상황 메시지를 전송합니다 (SSRF 검증 포함)
func (te *TaskExecutor) sendProgressMessage(responseURL string, message string) {
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
	for _, allowedDomain := range te.allowedResponseDomains {
		if parsedURL.Hostname() == allowedDomain || strings.HasSuffix(parsedURL.Hostname(), "."+allowedDomain) {
			isAllowed = true
			break
		}
	}

	if !isAllowed {
		log.Printf("SSRF 방어: 허용되지 않는 도메인으로의 응답 시도 차단: %s", responseURL)
		return
	}

	// 4. Slack 메시지 전송 (새 메시지 추가)
	payload := server.SlackDelayedResponse{
		Text:         message,
		ResponseType: "in_channel", // 채널에 공개
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshaling progress message: %v", err)
		return
	}

	resp, err := http.Post(responseURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		log.Printf("Error sending progress message to %s: %v", responseURL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Slack progress message returned non-200 status: %d", resp.StatusCode)
	}
}

// formatSuccessOutput은 cursor-agent 성공 출력을 Slack 마크다운으로 포맷팅합니다.
// 반환값: 메시지 배열 (40,000자씩 분할)
func (te *TaskExecutor) formatSuccessOutput(jobID string, rawOutput string, prompt string) []string {
	var result strings.Builder
	result.WriteString("✅ *Cursor AI 작업 완료*\n\n")
	result.WriteString(fmt.Sprintf("📝 *요청 프롬프트*\n> %s\n\n", prompt))
	
	// cursor-agent 출력 파싱
	lines := strings.Split(rawOutput, "\n")
	
	// 변경된 파일 목록 추출
	modifiedFiles := te.extractModifiedFiles(lines)
	if len(modifiedFiles) > 0 {
		result.WriteString("📁 *변경된 파일*\n")
		for _, file := range modifiedFiles {
			result.WriteString(fmt.Sprintf("• `%s`\n", file))
		}
		result.WriteString("\n")
	}
	
	// 주요 변경 사항 추출 (diff가 있으면 표시)
	changes := te.extractChangeSummary(lines)
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
	return te.splitMessage(result.String())
}

// formatErrorOutput은 에러 출력을 Slack 마크다운으로 포맷팅합니다.
// 반환값: 메시지 배열 (40,000자씩 분할)
func (te *TaskExecutor) formatErrorOutput(jobID string, err error, rawOutput string) []string {
	var result strings.Builder
	result.WriteString("❌ *Cursor AI 실행 중 오류 발생*\n\n")
	result.WriteString(fmt.Sprintf("🚨 *오류 메시지*\n> %s\n\n", err.Error()))
	
	if rawOutput != "" {
		result.WriteString("📄 *출력 내용*\n")
		// 에러 출력도 마크다운 렌더링 가능하도록 코드블록 제거
		result.WriteString(rawOutput)
		result.WriteString("\n")
	}
	
	result.WriteString(fmt.Sprintf("\n💡 자세한 정보: `/cursor show %s`", jobID[:8]))
	
	// 메시지를 40,000자 단위로 분할
	return te.splitMessage(result.String())
}

// extractModifiedFiles는 cursor-agent 출력에서 변경된 파일 목록을 추출합니다.
func (te *TaskExecutor) extractModifiedFiles(lines []string) []string {
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
						if file != "" && !contains(files, file) {
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
func (te *TaskExecutor) extractChangeSummary(lines []string) string {
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

// contains는 문자열 슬라이스에 특정 문자열이 있는지 확인합니다.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// splitMessage는 메시지를 Slack 최대 크기(40,000자)로 분할합니다.
func (te *TaskExecutor) splitMessage(message string) []string {
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
func (te *TaskExecutor) sendMultipleMessages(responseURL string, messages []string, jobID string) {
	for i, message := range messages {
		log.Printf("[%s] 메시지 전송 (%d/%d): %d자", jobID, i+1, len(messages), len(message))
		te.sendDelayedResponse(responseURL, message)
		
		// 메시지 간 짧은 대기 (Slack rate limit 방지)
		if i < len(messages)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// sendDelayedResponse는 SSRF 공격을 방지하기 위해 ResponseURL을 검증한 후 전송합니다.
func (te *TaskExecutor) sendDelayedResponse(responseURL string, message string) {
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
	for _, allowedDomain := range te.allowedResponseDomains {
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
	payload := server.SlackDelayedResponse{
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
