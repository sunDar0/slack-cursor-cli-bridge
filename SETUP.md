# Slack-Cursor-Hook 설치 가이드

이 문서는 **제3자가 이 프로젝트를 로컬에서 실행**하기 위한 완전한 설치 가이드입니다.

## 📋 목차

1. [빠른 시작 (추천!)](#빠른-시작-추천)
2. [사전 요구사항](#사전-요구사항)
3. [프로젝트 설치](#프로젝트-설치)
4. [Slack App 설정](#slack-app-설정)
5. [환경 변수 설정](#환경-변수-설정)
6. [서버 실행](#서버-실행)
7. [문제 해결](#문제-해결)

---

## 빠른 시작 (추천!)

**대화형 설정 마법사**를 사용하면 모든 설정을 자동으로 완료할 수 있습니다!

### 1단계: Go만 설치하세요

```bash
# macOS
brew install go

# Linux (Ubuntu/Debian)
sudo apt install golang-go
```

### 2단계: 프로젝트 클론

```bash
git clone <repository-url>
cd slack-cursor-hook
```

### 3단계: 설정 마법사 실행

```bash
go run cmd/server/main.go --setup
```

설정 마법사는 다음을 자동으로 수행합니다:
- ✅ **시스템 환경 확인** (OS, 아키텍처)
- ✅ **cursor-agent 설치 확인 및 자동 설치**
  - 설치되지 않은 경우 자동 설치 (`curl https://cursor.com/install -fsS | bash`)
  - PATH 설정 자동화
- ✅ **ngrok 설치 확인 및 자동 설치**
  - macOS: `brew install ngrok`
  - Linux: `snap install ngrok`
- ✅ **환경 변수 대화형 입력**
  - Slack Signing Secret 입력
  - `.env` 파일 자동 생성
- ✅ **프로젝트 초기화**
  - `data/` 디렉토리 생성 (SQLite DB용)
  - `logs/` 디렉토리 생성 (로그 파일용)

### 4단계: 서버 시작

```bash
조# 개발용 스크립트 (서버 + ngrok + 로그)
./start-dev.sh

# 또는 실행 파일 직접 실행 (ngrok 자동 실행!)
./실행파일
```

**자동으로 수행되는 작업:**
- ✅ 서버 시작 (포트 8080)
- ✅ ngrok 터널 생성 (설치되어 있는 경우)
- ✅ ngrok URL 자동 출력
- ✅ Slack App 설정 방법 안내

설정 마법사 실행 후 Slack App 설정만 완료하면 바로 사용할 수 있습니다!
([Slack App 설정](#slack-app-설정) 참조)

---

> 💡 **수동 설정을 원하시나요?** 아래의 상세 가이드를 따라주세요.

---

## 사전 요구사항

### ✅ 필수 설치

#### 1. Go (1.21 이상)

**macOS:**
```bash
brew install go
```

**Linux:**
```bash
# Ubuntu/Debian
sudo apt update
sudo apt install golang-go

# 또는 공식 사이트에서 다운로드
# https://go.dev/dl/
```

**Windows:**
https://go.dev/dl/ 에서 설치 프로그램 다운로드

**확인:**
```bash
go version
# 출력 예시: go version go1.21.0 darwin/arm64
```

#### 2. Cursor IDE 및 cursor-agent CLI

**Cursor IDE 설치:**
1. https://cursor.sh/ 접속
2. 운영체제에 맞는 버전 다운로드 및 설치
3. 설치 후 Cursor IDE를 최소 1회 실행

**cursor-agent 확인:**
```bash
which cursor-agent
# 출력 예시: /usr/local/bin/cursor-agent

cursor-agent --version
```

**문제 발생 시:**
- Cursor IDE가 설치되어 있지만 `cursor-agent`를 찾을 수 없는 경우
- Cursor IDE 설정에서 CLI 도구 설치 확인
- 또는 `.env` 파일에서 `CURSOR_CLI_PATH`를 절대 경로로 지정

#### 3. ngrok (로컬 테스트용)

**macOS:**
```bash
brew install ngrok
```

**Linux:**
```bash
# Ubuntu/Debian
curl -s https://ngrok-agent.s3.amazonaws.com/ngrok.asc | \
  sudo tee /etc/apt/trusted.gpg.d/ngrok.asc >/dev/null && \
  echo "deb https://ngrok-agent.s3.amazonaws.com buster main" | \
  sudo tee /etc/apt/sources.list.d/ngrok.list && \
  sudo apt update && sudo apt install ngrok
```

**Windows:**
https://ngrok.com/download 에서 다운로드

**확인:**
```bash
ngrok version
```

---

## 프로젝트 설치

### 1. 저장소 클론

```bash
git clone <repository-url>
cd slack-cursor-hook
```

### 2. Go 의존성 설치

```bash
go mod download
```

**예상 소요 시간:** 1-2분

---

## Slack App 설정

Slack에서 `/cursor` 명령어를 사용하려면 먼저 Slack App을 생성해야 합니다.

### 1. Slack App 생성

1. **[Slack API 페이지](https://api.slack.com/apps)** 접속
2. **"Create New App"** 클릭
3. **"From scratch"** 선택
4. **App Name** 입력 (예: "Cursor AI Assistant")
5. **워크스페이스** 선택
6. **"Create App"** 클릭

### 2. Slash Command 추가

1. 왼쪽 메뉴에서 **"Slash Commands"** 클릭
2. **"Create New Command"** 클릭
3. 다음 정보 입력:
   - **Command:** `/cursor`
   - **Request URL:** `https://your-ngrok-url/slack/cursor` (나중에 설정)
   - **Short Description:** `Cursor AI를 통한 코드 작업`
   - **Usage Hint:** `자연어 프롬프트 또는 set-path <경로>`
4. **"Save"** 클릭

### 3. Signing Secret 확보

1. 왼쪽 메뉴에서 **"Basic Information"** 클릭
2. **"App Credentials"** 섹션 찾기
3. **"Signing Secret"** 옆의 **"Show"** 클릭
4. 값을 복사 (이 값은 나중에 `.env` 파일에 사용)

### 4. 워크스페이스에 설치

1. 왼쪽 메뉴에서 **"Install App"** 클릭
2. **"Install to Workspace"** 클릭
3. **"Allow"** 클릭

---

## 환경 변수 설정

### 1. `.env` 파일 생성

프로젝트 루트 디렉토리에 `.env` 파일을 생성합니다:

```bash
cat > .env << 'EOF'
# 필수: Slack App의 Signing Secret
SLACK_SIGNING_SECRET=your_slack_signing_secret_here

# 선택사항 (기본값 사용 가능)
# CURSOR_CLI_PATH=cursor-agent
# CURSOR_PROJECT_PATH=/path/to/your/project
# DB_PATH=./data/jobs.db
# PORT=8080
EOF
```

### 2. Signing Secret 설정

1. `.env` 파일을 텍스트 에디터로 열기:
   ```bash
   vim .env
   # 또는
   nano .env
   ```

2. `your_slack_signing_secret_here` 부분을 **실제 Signing Secret**으로 교체
   ```bash
   SLACK_SIGNING_SECRET=a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6
   ```

3. 저장 후 종료

### 3. 환경 변수 설명

| 변수 | 필수 | 기본값 | 설명 |
|------|------|--------|------|
| `SLACK_SIGNING_SECRET` | ✅ | 없음 | Slack App의 Signing Secret |
| `CURSOR_CLI_PATH` | ❌ | `cursor-agent` | cursor-agent 실행 파일 경로 |
| `CURSOR_PROJECT_PATH` | ❌ | 없음 | 기본 프로젝트 경로 (API로 변경 가능) |
| `DB_PATH` | ❌ | `./data/jobs.db` | SQLite 데이터베이스 파일 경로 |
| `PORT` | ❌ | `8080` | 서버 포트 |

---

## 서버 실행

### 🚀 방법 1: 개발 스크립트 사용 (권장)

**Go 서버와 ngrok을 한 번에 시작:**

```bash
./start-dev.sh
```

**출력 예시:**
```
✅ 개발 환경이 준비되었습니다!

🌐 ngrok 공개 URL:
   https://abc123def456.ngrok-free.app

📝 Slack App 설정:
   1. https://api.slack.com/apps 접속
   2. 앱 선택 → Slash Commands → /cursor 편집
   3. Request URL에 다음을 입력:
      https://abc123def456.ngrok-free.app/slack/cursor
```

**다음 단계:**
1. 출력된 ngrok URL 복사
2. [Slack API 페이지](https://api.slack.com/apps) → 앱 선택
3. **Slash Commands** → `/cursor` 편집
4. **Request URL**에 복사한 URL 붙여넣기
5. **Save** 클릭

**종료:**
- `Ctrl+C` 한 번으로 Go 서버와 ngrok 모두 종료

### 🔧 방법 2: 수동 실행

**터미널 1 - Go 서버:**
```bash
go run cmd/server/main.go
```

**터미널 2 - ngrok:**
```bash
ngrok http 8080
```

---

## 첫 번째 사용

### 1. 프로젝트 경로 설정

Slack에서 처음 사용할 때 프로젝트 경로를 설정해야 합니다:

```
/cursor set-path /Users/yourname/projects/your-project
```

### 2. 코드 작업 요청

이제 자연어로 코드 작업을 요청할 수 있습니다:

```
/cursor README.md에 설치 가이드를 추가해줘
```

```
/cursor main.go의 버그를 수정해줘
```

```
/cursor 모든 함수에 주석을 추가해줘
```

---

## 문제 해결

### ❌ "SLACK_SIGNING_SECRET 환경변수가 설정되지 않았습니다"

**원인:** `.env` 파일이 없거나, `SLACK_SIGNING_SECRET`이 설정되지 않음

**해결:**
```bash
# .env 파일 확인
cat .env

# SLACK_SIGNING_SECRET이 있는지 확인
grep SLACK_SIGNING_SECRET .env
```

### ❌ "cursor-agent: command not found"

**원인:** cursor-agent가 PATH에 없음

**해결 1 - PATH 확인:**
```bash
# Cursor IDE 설치 확인
ls -la "/Applications/Cursor.app"  # macOS
ls -la "$HOME/.cursor"              # Linux

# cursor-agent 위치 찾기
find / -name cursor-agent 2>/dev/null
```

**해결 2 - .env에 절대 경로 설정:**
```bash
# .env 파일에 추가
CURSOR_CLI_PATH=/Applications/Cursor.app/Contents/Resources/app/bin/cursor-agent
```

### ❌ "Signature mismatch" (Slack에서 에러)

**원인:** Signing Secret이 잘못되었거나, ngrok URL이 변경됨

**해결:**
1. `.env` 파일의 `SLACK_SIGNING_SECRET` 확인
2. Slack App 설정의 Signing Secret과 일치하는지 확인
3. ngrok을 재시작한 경우 Slack App의 Request URL도 업데이트

### ❌ ngrok URL이 매번 바뀜

**원인:** ngrok 무료 플랜은 실행할 때마다 URL이 변경됨

**해결 방법:**

**옵션 1 - ngrok 유료 플랜 ($10/월):**
- 고정 도메인 제공

**옵션 2 - Cloudflare Tunnel (무료):**
```bash
brew install cloudflare/cloudflare/cloudflared
cloudflared tunnel login
cloudflared tunnel create slack-cursor
# 자세한 설정은 README.md 참조
```

**옵션 3 - 실제 서버 배포:**
- Google Cloud Run (무료 티어)
- Fly.io (무료 티어)
   - 자세한 내용은 `docs/technical/deployment-strategy.md` 참조

### ❌ "프로젝트 경로가 설정되지 않았습니다"

**원인:** 첫 사용 시 프로젝트 경로를 설정하지 않음

**해결:**
```
/cursor set-path /path/to/your/project
```

### ❌ 포트 8080이 이미 사용 중

**원인:** 다른 프로그램이 8080 포트를 사용 중

**해결 1 - 다른 포트 사용:**
```bash
# .env 파일에 추가
PORT=3000
```

**해결 2 - 기존 프로세스 종료:**
```bash
# macOS/Linux
lsof -ti:8080 | xargs kill -9
```

---

## 다음 단계

### ✅ 기본 기능 확인 완료 후:

1. **Swagger UI 확인:**
   - http://localhost:8080/swagger/index.html
   - API 문서 확인 및 테스트

2. **작업 결과 조회:**
   ```bash
   # 모든 작업 목록
   curl http://localhost:8080/api/jobs
   
   # 특정 작업 결과
   curl http://localhost:8080/api/jobs/<job_id>
   ```

3. **실제 배포:**
   - `docs/technical/deployment-strategy.md` 참조
   - Google Cloud Run 또는 Fly.io 권장

---

## 📚 추가 문서

- **README.md** - 프로젝트 개요 및 사용법
- **docs/technical/deployment-strategy.md** - 배포 전략 가이드
- **docs/technical/architecture.md** - 기술 설계 문서

---

## 🆘 도움이 필요하신가요?

- **Issues**: GitHub Issues에 문제를 보고해주세요
- **Documentation**: README.md의 문제 해결 섹션 참조
- **Logs**: `logs/server.log` 파일 확인

---

## 🎉 설치 완료!

모든 단계를 완료했다면, 이제 Slack에서 `/cursor` 명령어를 사용할 수 있습니다!

```
/cursor set-path /path/to/project
/cursor README에 새로운 섹션을 추가해줘
```

즐거운 코딩 되세요! 🚀

