package worker

import (
	"time"

	"github.com/kakaovx/cursor-slack-server/internal/types"
)

// Job은 cursor-agent 실행 작업을 정의합니다.
// Dispatcher의 JobQueue를 통해 Worker에게 전달됩니다.
//
// 순환 참조를 피하기 위해 Config는 interface{} 타입을 사용합니다.
type Job struct {
	ID          string                      // 로깅 및 추적을 위한 고유 ID (예: UUID)
	Payload     types.SlackCommandPayload   // Slack 페이로드
	ReceivedAt  time.Time                   // 요청 수신 시간 (큐 대기 시간 측정용)
	Config      interface{}                 // 서버 설정 (*server.Config)
}
