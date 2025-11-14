package types

// SlackCommandPayload는 Slack이 보내는 폼 데이터를 바인딩합니다.
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

