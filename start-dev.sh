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
    
    # v1.4.1: 강력한 프로세스 종료 (SIGTERM → SIGKILL)
    if [ -f "$PIDS_FILE" ]; then
        while IFS= read -r pid; do
            if ps -p "$pid" > /dev/null 2>&1; then
                echo "  종료 시도: PID $pid"
                # SIGTERM 전송 (graceful shutdown 시도)
                kill -TERM "$pid" 2>/dev/null
                sleep 1
                
                # 여전히 살아있으면 SIGKILL
                if ps -p "$pid" > /dev/null 2>&1; then
                    echo "  강제 종료: PID $pid"
                    kill -9 "$pid" 2>/dev/null
                    sleep 0.5
                fi
            fi
        done < "$PIDS_FILE"
        rm "$PIDS_FILE"
    fi
    
    # ngrok 프로세스 정리
    pkill -9 -f "ngrok http" 2>/dev/null
    
    # 포트 정리 확인
    echo -e "${YELLOW}🔍 포트 8080 정리 확인 중...${NC}"
    PORT_PIDS=$(lsof -ti :8080 2>/dev/null)
    if [ -n "$PORT_PIDS" ]; then
        echo "  포트 8080 사용 프로세스 강제 종료: $PORT_PIDS"
        echo "$PORT_PIDS" | xargs kill -9 2>/dev/null
    fi
    
    echo -e "${GREEN}✅ 정리 완료${NC}"
    exit 0
}

# 시작 전 포트 정리 함수 (v1.4.1)
cleanup_port() {
    local port=$1
    echo -e "${YELLOW}🔍 포트 $port 정리 중...${NC}"
    
    # lsof로 포트 사용 프로세스 찾기
    local pids=$(lsof -ti ":$port" 2>/dev/null)
    
    if [ -n "$pids" ]; then
        echo -e "${YELLOW}⚠️  포트 $port가 이미 사용 중입니다.${NC}"
        echo "  사용 중인 프로세스: $pids"
        echo "  기존 프로세스를 종료합니다..."
        
        for pid in $pids; do
            # 프로세스 정보 출력
            ps -p "$pid" -o pid,command 2>/dev/null || true
            
            # SIGTERM 시도
            kill -TERM "$pid" 2>/dev/null
            sleep 1
            
            # 여전히 살아있으면 SIGKILL
            if ps -p "$pid" > /dev/null 2>&1; then
                echo "  강제 종료: PID $pid"
                kill -9 "$pid" 2>/dev/null
                sleep 0.5
            fi
        done
        
        # 최종 확인
        sleep 1
        local remaining=$(lsof -ti ":$port" 2>/dev/null)
        if [ -n "$remaining" ]; then
            echo -e "${RED}❌ 포트 정리 실패. 수동으로 종료하세요: kill -9 $remaining${NC}"
            exit 1
        fi
        
        echo -e "${GREEN}✅ 포트 $port 정리 완료${NC}"
    else
        echo -e "${GREEN}✅ 포트 $port 사용 가능${NC}"
    fi
}

# Ctrl+C 시그널 처리
trap cleanup SIGINT SIGTERM

echo -e "${BLUE}🚀 Slack-Cursor-Hook 개발 환경 시작${NC}"
echo ""

# v1.4.1: 시작 전 포트 정리
cleanup_port 8080
cleanup_port 4040  # ngrok 대시보드
echo ""

# 1. Go 서버 시작
echo -e "${YELLOW}📦 Go 서버 시작 중...${NC}"
# go run은 임시 디렉토리에서 실행되므로 DB_PATH를 명시적으로 지정
DB_PATH="./data/jobs.db" go run cmd/server/main.go > "$SERVER_LOG" 2>&1 &
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
echo -e "   ${YELLOW}/cursor help${NC}  - 도움말 및 전체 명령어 목록"
echo -e "   ${YELLOW}/cursor set-path /path/to/project${NC}  - 프로젝트 경로 설정"
echo -e "   ${YELLOW}/cursor path${NC}                       - 현재 경로 확인"
echo -e "   ${YELLOW}/cursor list${NC}                       - 최근 작업 목록"
echo -e "   ${YELLOW}/cursor show <job-id>${NC}              - 작업 결과 보기"
echo -e "   ${YELLOW}/cursor \"프롬프트\"${NC}                  - 코드 작업 요청"
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

