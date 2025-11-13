package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestIDKey는 Gin Context에서 요청 ID를 식별하기 위한 키입니다.
const RequestIDKey = "requestID"

const maxTimestampAge = 5 * time.Minute

// SlackAuthMiddleware는 Slack 요청의 서명과 타임스탬프를 검증합니다.
func SlackAuthMiddleware(signingSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 모든 요청에 고유 ID 부여
		c.Set(RequestIDKey, uuid.NewString())

		// 1. 요청 본문 읽기 (이중 읽기 문제 해결)
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			log.Printf("[%s] Failed to read body: %v", c.GetString(RequestIDKey), err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to read body"})
			return
		}
		// 핸들러가 다시 읽을 수 있도록 본문 복원
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// 2. 타임스탬프 검증 (Replay Attack 방어)
		timestampStr := c.GetHeader("X-Slack-Request-Timestamp")
		timestampInt, err := strconv.ParseInt(timestampStr, 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid timestamp"})
			return
		}
		timestamp := time.Unix(timestampInt, 0)

		// 시간차가 5분을 초과하면 요청 거부
		if time.Since(timestamp) > maxTimestampAge {
			log.Printf("[%s] Timestamp too old (Replay Attack?): %s", c.GetString(RequestIDKey), timestampStr)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Timestamp too old"})
			return
		}

		// 3. HMAC 서명 검증
		slackSignature := c.GetHeader("X-Slack-Signature")
		if slackSignature == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Signature missing"})
			return
		}

		// v0=<timestamp>:<raw_body> 형태의 basestring 생성
		baseString := fmt.Sprintf("v0:%s:%s", timestampStr, string(bodyBytes))

		// HMAC-SHA256 계산
		h := hmac.New(sha256.New, []byte(signingSecret))
		h.Write([]byte(baseString))
		expectedSignature := "v0=" + hex.EncodeToString(h.Sum(nil))

		// 서명 비교 (Timing Attack 방지를 위해 hmac.Equal 사용)
		if !hmac.Equal([]byte(slackSignature), []byte(expectedSignature)) {
			log.Printf("[%s] Signature mismatch", c.GetString(RequestIDKey))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Signature mismatch"})
			return
		}

		// 4. 모든 검증 통과
		c.Next()
	}
}

