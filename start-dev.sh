#!/bin/bash

# 색상 정의
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# 로그 디렉토리 및 파일 경로
LOGS_DIR="logs"
PIDS_FILE="$LOGS_DIR/.dev-pids"
SERVER_LOG="$LOGS_DIR/server.log"
NGROK_LOG="$LOGS_DIR/ngrok.log"

# 로그 디렉토리 생성
mkdir -p "$LOGS_DIR"

# 종료 시 정리 함수
cleanup() {
    echo ""
    echo -e "${YELLOW}🛑 서버를 종료합니다...${NC}"
    
    if [ -f "$PIDS_FILE" ]; then
        while IFS= read -r pid; do
            if ps -p "$pid" > /dev/null 2>&1; then
                echo "  종료: PID $pid"
                kill "$pid" 2>/dev/null
            fi
        done < "$PIDS_FILE"
        rm "$PIDS_FILE"
    fi
    
    # ngrok 프로세스 정리
    pkill -f "ngrok http" 2>/dev/null
    
    echo -e "${GREEN}✅ 정리 완료${NC}"
    exit 0
}

# Ctrl+C 시그널 처리
trap cleanup SIGINT SIGTERM

echo -e "${BLUE}🚀 Slack-Cursor-Hook 개발 환경 시작${NC}"
echo ""

# 1. Go 서버 시작
echo -e "${YELLOW}📦 Go 서버 시작 중...${NC}"
go run cmd/server/main.go > "$SERVER_LOG" 2>&1 &
SERVER_PID=$!
echo $SERVER_PID > "$PIDS_FILE"
echo -e "${GREEN}✅ Go 서버 시작됨 (PID: $SERVER_PID)${NC}"

# 서버가 시작될 때까지 대기
sleep 3

# 서버 헬스 체크
if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo -e "${GREEN}✅ Go 서버 정상 작동 확인${NC}"
else
    echo -e "${RED}❌ Go 서버 시작 실패. $SERVER_LOG를 확인하세요.${NC}"
    cleanup
fi

echo ""

# 2. ngrok 시작
echo -e "${YELLOW}🌐 ngrok 터널 생성 중...${NC}"
ngrok http 8080 --log=stdout --log-level=error > "$NGROK_LOG" 2>&1 &
NGROK_PID=$!
echo $NGROK_PID >> "$PIDS_FILE"
echo -e "${GREEN}✅ ngrok 시작됨 (PID: $NGROK_PID)${NC}"

# ngrok이 시작될 때까지 대기
echo -e "${YELLOW}⏳ ngrok URL 생성 중 (5초)...${NC}"
sleep 5

# ngrok URL 가져오기
NGROK_URL=$(curl -s http://localhost:4040/api/tunnels | grep -o '"public_url":"https://[^"]*' | head -1 | cut -d'"' -f4)

if [ -z "$NGROK_URL" ]; then
    echo -e "${RED}❌ ngrok URL을 가져올 수 없습니다. $NGROK_LOG를 확인하세요.${NC}"
    cleanup
fi

echo ""
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}✅ 개발 환경이 준비되었습니다!${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "${BLUE}🌐 ngrok 공개 URL:${NC}"
echo -e "   ${YELLOW}$NGROK_URL${NC}"
echo ""
echo -e "${BLUE}📝 Slack App 설정:${NC}"
echo -e "   1. https://api.slack.com/apps 접속"
echo -e "   2. 앱 선택 → Slash Commands → /cursor 편집"
echo -e "   3. Request URL에 다음을 입력:"
echo -e "      ${GREEN}$NGROK_URL/slack/cursor${NC}"
echo ""
echo -e "${BLUE}🔗 유용한 링크:${NC}"
echo -e "   • Swagger UI:    http://localhost:8080/swagger/index.html"
echo -e "   • ngrok 대시보드: http://localhost:4040"
echo -e "   • Health Check:  http://localhost:8080/health"
echo ""
echo -e "${BLUE}📋 사용 가능한 Slack 명령어:${NC}"
echo -e "   ${YELLOW}/cursor set-path /path/to/project${NC}  - 프로젝트 경로 설정"
echo -e "   ${YELLOW}/cursor 자연어 프롬프트${NC}            - 코드 작업 요청"
echo ""
echo -e "${RED}⚠️  종료하려면 Ctrl+C를 누르세요${NC}"
echo ""
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# 로그 출력 (tail -f 스타일)
echo -e "${BLUE}📄 실시간 로그 ($SERVER_LOG):${NC}"
echo ""
tail -f "$SERVER_LOG"

# 이 지점에는 도달하지 않지만, 안전을 위해 cleanup 호출
cleanup

