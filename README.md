# Slack-Cursor-CLI 연동 서버

Slack 슬래시 커맨드를 통해 Cursor AI CLI를 실행할 수 있는 Go 서버입니다.

## 🎉 Slack 연동 완료!

이 서버는 Slack과 완전히 연동되어 있으며, `/cursor` 슬래시 커맨드를 통해 Cursor AI를 사용할 수 있습니다.

## 현재 버전: v1.3

이 프로젝트는 점 → 원 → 구체로 확장하며 개발됩니다.

**v1.3 구현 기능:**
- ✅ **Slack 슬래시 커맨드 연동 완료** (`/cursor` 명령어 지원)
- ✅ **HMAC-SHA256 서명 검증** (Slack 요청 인증)
- ✅ **타임스탬프 검증** (Replay Attack 방어)
- ✅ **지연 응답 전송** (Slack 3초 룰 준수)
- ✅ cursor-agent CLI 동기/비동기 실행 (`--force` 플래그)
- ✅ 일반 API 엔드포인트 (`/api/cursor`)
- ✅ **SQLite 기반 작업 결과 저장** (v1.3)
- ✅ **작업 조회 API** (`/api/jobs/:id`, `/api/jobs`) (v1.3)
- ✅ **동적 프로젝트 경로 관리** (`/api/config/project-path`) (v1.2)
- ✅ Swagger UI를 통한 API 문서 및 테스트
- ✅ Process Group 관리 (타임아웃 시 자식 프로세스 종료)
- ✅ SSRF 방어 (허용 도메인 검증)

**향후 추가 예정 (원/구체 단계):**
- Worker Pool 아키텍처
- Viper 설정 관리
- Redis 기반 분산 작업 큐
- Graceful Shutdown

## 빠른 시작 🚀

### 옵션 1: 대화형 설정 마법사 (추천!)

처음 사용하시나요? 설정 마법사가 모든 것을 자동으로 설정해드립니다!

```bash
go run cmd/server/main.go --setup
```

설정 마법사는 다음을 자동으로 수행합니다:
- ✅ cursor-agent 설치 확인 및 자동 설치
- ✅ ngrok 설치 확인 및 자동 설치
- ✅ 환경 변수 대화형 입력
- ✅ 프로젝트 초기화 (data/, logs/ 디렉토리 생성)

### 옵션 2: 수동 설정

수동으로 설정하고 싶으시다면 [SETUP.md](./SETUP.md)를 참고하세요.

## 사전 요구사항

- Go 1.21+
- **cursor-agent CLI** 설치 및 PATH 설정 (Cursor IDE 포함)
- **Slack 앱 생성 및 Signing Secret 확보** (필수)

> 📖 **처음 설치하시나요?** [SETUP.md](./SETUP.md)에서 제3자를 위한 완전한 설치 가이드를 확인하세요!

### Slack 앱 생성 및 설정

**1. Slack 앱 생성:**
1. [Slack API 사이트](https://api.slack.com/apps)에 접속
2. "Create New App" 클릭
3. "From scratch" 선택
4. 앱 이름과 워크스페이스 선택 후 생성

**2. Slash Command 추가:**
1. 좌측 메뉴에서 "Slash Commands" 선택
2. "Create New Command" 클릭
3. 다음 정보 입력:
   - **Command**: `/cursor`
   - **Request URL**: `https://your-domain.com/slack/cursor` (로컬 테스트 시 ngrok URL 사용)
   - **Short Description**: `Cursor AI CLI 실행`
   - **Usage Hint**: `"프롬프트" (예: "main.go의 버그를 수정해줘")`
4. "Save" 클릭

**3. Signing Secret 확인:**
1. 좌측 메뉴에서 "Basic Information" 선택
2. "App Credentials" 섹션에서 **Signing Secret** 복사
3. 이 값을 `.env` 파일의 `SLACK_SIGNING_SECRET`에 설정

**4. 워크스페이스에 앱 설치:**
1. 좌측 메뉴에서 "Install App" 선택
2. "Install to Workspace" 클릭
3. 권한 승인

> 💡 **로컬 개발 시**: ngrok을 사용하여 HTTPS URL을 생성하고, Slack 앱의 Request URL에 설정하세요.

### Cursor Agent CLI 설정

**1. cursor-agent 위치 확인:**

```bash
# cursor-agent가 어디에 있는지 확인
which cursor-agent
```

**2-1. PATH에 있는 경우:**
- `.env` 파일에 `CURSOR_CLI_PATH` 설정 불필요!
- 바로 서버 실행 가능

**2-2. PATH에 없는 경우:**

`.env` 파일에 전체 경로 지정:
```env
# .env
SLACK_SIGNING_SECRET=your_secret
CURSOR_CLI_PATH=/Users/username/.local/bin/cursor-agent
```

또는 PATH에 추가:
```bash
# 심볼릭 링크 생성
sudo ln -s /Users/username/.local/bin/cursor-agent /usr/local/bin/cursor-agent
```

## 환경 변수 설정

| 환경 변수 | 설명 | 필수 | 기본값 | 예시 |
|----------|------|------|--------|------|
| `SLACK_SIGNING_SECRET` | Slack 앱의 Signing Secret | ✅ 필수 | - | `abc123...` |
| `CURSOR_CLI_PATH` | cursor-agent CLI 실행 파일 경로 | ❌ 선택 | `cursor-agent` | `/usr/local/bin/cursor-agent` |
| `CURSOR_PROJECT_PATH` | cursor-cli가 실행될 프로젝트 경로 (API로 동적 변경 가능) | ❌ 선택 | 미설정 (API로 설정 필요) | `/Users/username/myproject` |
| `DB_PATH` | SQLite 데이터베이스 파일 경로 | ❌ 선택 | `./data/jobs.db` | `/var/data/jobs.db` |
| `PORT` | 서버가 수신 대기할 포트 | ❌ 선택 | `8080` | `3000` |

> **v1.2+**: `CURSOR_PROJECT_PATH`는 실행 중에 `/api/config/project-path` API 또는 `/cursor set-path` 명령어로 변경할 수 있습니다.

## 설치 및 실행

### 1. 의존성 설치

```bash
go mod download
```

### 2. Swagger CLI 설치 (선택사항)

API 문서를 수정하고 재생성하려면 swag CLI를 설치합니다:

```bash
go install github.com/swaggo/swag/cmd/swag@latest
```

> swag CLI 없이도 서버는 정상 동작합니다. 이미 생성된 `docs/` 폴더의 문서를 사용합니다.

### 3. 환경 변수 설정

**방법 1: .env 파일 사용 (권장)**

프로젝트 루트에 `.env` 파일을 생성합니다:

```bash
# .env 파일 생성 (최소 설정)
cat > .env << 'EOF'
SLACK_SIGNING_SECRET=your_slack_signing_secret_here
EOF
```

또는 직접 편집 (전체 설정):
```env
# .env

# 필수
SLACK_SIGNING_SECRET=your_slack_signing_secret_here

# 선택 (설정하지 않으면 기본값 사용)
# CURSOR_CLI_PATH=/Users/username/.local/bin/cursor-agent
# CURSOR_PROJECT_PATH=/Users/username/myproject  # API로도 설정 가능
# DB_PATH=./data/jobs.db                         # v1.3: SQLite DB 경로
# PORT=8080
```

> 💡 `cursor-agent`가 PATH에 있다면 `CURSOR_CLI_PATH` 설정은 불필요합니다.
> 
> **Non-interactive 모드**: 서버는 `cursor-agent -p "prompt" --output-format text` 형식으로 실행합니다.

**방법 2: 시스템 환경변수 사용**

```bash
# 필수
export SLACK_SIGNING_SECRET="your_signing_secret_here"

# 선택사항 (기본값: 현재 디렉토리)
export CURSOR_PROJECT_PATH="/path/to/your/project"

# 선택사항 (기본값: 8080)
export PORT="8080"
```

> 💡 `.env` 파일이 있으면 자동으로 로드됩니다. 없으면 시스템 환경변수를 사용합니다.

### 4. 서버 실행

#### 방법 1: 개발 스크립트 사용 (추천)

Go 서버와 ngrok을 한 번에 시작:

```bash
./start-dev.sh
```

스크립트가 자동으로:
- ✅ Go 서버 시작
- ✅ ngrok 터널 생성
- ✅ Slack App 설정용 URL 출력
- ✅ 실시간 로그 표시

종료하려면 `Ctrl+C`를 누르세요.

#### 방법 2: 수동 실행

**터미널 1 - Go 서버:**
```bash
go run cmd/server/main.go
```

**터미널 2 - ngrok:**
```bash
ngrok http 8080
```

## 로컬 테스트 (ngrok)

로컬에서 Slack과 연동하려면 ngrok을 사용합니다:

**1. ngrok 설치 및 실행:**
```bash
# ngrok 설치 (macOS)
brew install ngrok

# 또는 공식 사이트에서 다운로드: https://ngrok.com/download

# 서버 실행 후 다른 터미널에서
ngrok http 8080
```

**2. ngrok URL을 Slack에 설정:**
ngrok이 제공하는 HTTPS URL (예: `https://abc123.ngrok.io`)을 복사하여:

1. [Slack API 사이트](https://api.slack.com/apps) → 생성한 앱 선택
2. **Slash Commands** → `/cursor` 편집
3. **Request URL**을 `https://abc123.ngrok.io/slack/cursor`로 설정
4. "Save" 클릭

**3. 연동 확인:**
- Slack 워크스페이스에서 `/cursor` 명령어 입력
- 자동완성이 나타나면 연동 성공!

> ⚠️ **주의**: ngrok 무료 버전은 재시작 시마다 URL이 변경됩니다. 프로덕션 환경에서는 고정 도메인을 사용하세요.

## 사용 방법

### 0️⃣ 프로젝트 경로 설정 (최초 1회)

`CURSOR_PROJECT_PATH` 환경 변수를 설정하지 않았다면, 먼저 작업할 프로젝트 경로를 설정해야 합니다:

**Slack에서:**
```
/cursor set-path /Users/username/myproject
```

**또는 API로:**
```bash
curl -X POST http://localhost:8080/api/config/project-path \
  -H "Content-Type: application/json" \
  -d '{"project_path": "/Users/username/myproject"}'
```

### 1️⃣ Slack에서 사용하기

**기본 사용법:**
임의의 채널에서 다음과 같이 자연어 프롬프트를 입력합니다:

```
/cursor main.go의 버그를 수정해줘
```

```
/cursor 모든 함수에 에러 핸들링을 추가해줘
```

```
/cursor 코드를 리팩토링하고 주석을 추가해줘
```

**프로젝트 경로 설정 (최초 1회):**
프로젝트 경로가 설정되지 않은 경우, 먼저 설정해야 합니다:

```
/cursor set-path /Users/username/myproject
```

**동작 흐름:**
1. `/cursor` 명령어 입력 → 즉시 "⏳ 요청을 접수했습니다" 메시지 표시
2. 서버에서 cursor-agent 실행 (최대 120초)
3. 완료 후 채널에 결과 메시지 자동 전송

**특징:**
- ✅ 자연어로 작업을 설명하면 Cursor AI가 프로젝트 전체를 컨텍스트로 분석합니다
- ✅ `--force` 플래그가 자동으로 추가되어 파일 수정이 허용됩니다
- ✅ 모든 작업은 SQLite DB에 저장되어 나중에 조회 가능합니다
- ✅ HMAC 인증으로 보안이 보장됩니다

### 2️⃣ 일반 API로 사용하기 (테스트/개발용)

#### Swagger UI에서 테스트

1. 브라우저에서 `http://localhost:8080/swagger/index.html` 접속
2. `POST /api/cursor` 엔드포인트 선택
3. "Try it out" 클릭
4. 요청 JSON 편집:
```json
{
  "prompt": "main.go의 버그를 수정해줘",
  "async": false
}
```
5. "Execute" 클릭하여 즉시 실행 결과 확인!

#### curl로 테스트

**동기 실행 (결과 즉시 반환):**
```bash
curl -X POST http://localhost:8080/api/cursor \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "모든 함수에 에러 핸들링을 추가해줘",
    "async": false
  }'
```

**비동기 실행 (job_id 반환 후 결과 조회):**
```bash
# 1. 비동기 작업 시작
JOB_ID=$(curl -X POST http://localhost:8080/api/cursor \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "코드를 리팩토링해줘",
    "async": true
  }' | jq -r '.job_id')

# 2. 작업 결과 조회 (v1.3+)
curl http://localhost:8080/api/jobs/$JOB_ID
```

**작업 목록 조회 (v1.3+):**
```bash
# 모든 작업 조회
curl http://localhost:8080/api/jobs

# 상태별 필터링
curl "http://localhost:8080/api/jobs?status=completed&limit=10"
```

## 아키텍처 (v1.3)

```
┌─────────────┐           ┌─────────────┐
│   Slack     │           │  Browser    │
│   /cursor   │           │  (Swagger)  │
└──────┬──────┘           └──────┬──────┘
       │                         │
       │ HMAC Auth              │ No Auth
       │                         │
       ▼                         ▼
┌──────────────────────────────────────┐
│         Gin HTTP Server              │
│  ┌────────────┐    ┌──────────────┐  │
│  │ /slack/*   │    │   /api/*     │  │
│  │ (인증 필요)  │    │  (공개)       │  │
│  └─────┬──────┘    └──────┬───────┘  │
│        │                  │          │
│        └──────┬───────────┘          │
│               ▼                      │
│      executeCursorCLI()              │
│               │                      │
│               ▼                      │
│      ┌─────────────────┐             │
│      │  SQLite DB      │             │
│      │  (작업 결과 저장) │             │
│      └─────────────────┘             │
└──────────────┬───────────────────────┘
               │
               ▼
      ┌─────────────────┐
      │  cursor-agent   │
      │  (외부 프로세스)   │
      │  --force 플래그   │
      └─────────────────┘
```

**v1.3 주요 흐름:**
1. 요청 수신 → 작업 생성 (DB에 `pending` 상태 저장)
2. cursor-agent 실행 전 → `running` 상태로 업데이트
3. 실행 완료 → 결과와 함께 `completed` 또는 `failed` 상태로 업데이트
4. 사용자는 `/api/jobs/:id`로 언제든지 작업 결과 조회 가능

## 프로젝트 구조

```
/cursor-slack-server
├── cmd/server/
│   └── main.go                      # 서버 진입점
├── internal/
│   ├── server/
│   │   ├── handlers.go              # 핸들러 (Slack + API + Jobs)
│   │   ├── router.go                # Gin 라우터 설정
│   │   └── middleware/
│   │       └── slack_auth.go        # HMAC 인증 미들웨어
│   └── database/
│       └── database.go              # SQLite 작업 저장소 (v1.3)
├── docs/                            # Swagger 문서 (자동 생성)
│   ├── docs.go
│   ├── swagger.json
│   └── swagger.yaml
├── data/                            # SQLite 데이터베이스 (git ignore)
│   └── jobs.db
├── logs/                            # 개발 로그 파일 (git ignore)
│   ├── server.log
│   ├── ngrok.log
│   └── .dev-pids
├── start-dev.sh                     # 개발 환경 시작 스크립트
├── go.mod
├── README.md                        # 프로젝트 개요 및 사용법
└── SETUP.md                         # 제3자를 위한 설치 가이드
```

## Swagger API 문서

서버를 실행한 후 브라우저에서 Swagger UI를 통해 API를 테스트할 수 있습니다:

```
http://localhost:8080/swagger/index.html
```

Swagger UI에서 다음을 수행할 수 있습니다:
- 📚 **API 문서 확인**: 모든 엔드포인트의 상세 스펙
- 🧪 **API 직접 테스트**: `/api/cursor` 엔드포인트를 Swagger UI에서 즉시 실행 가능!
- 📋 **요청/응답 예시**: 각 API의 예시 데이터 확인

### 사용 가능한 엔드포인트

| 엔드포인트 | 인증 | 설명 | Swagger 테스트 | 버전 |
|-----------|------|------|----------------|------|
| `POST /api/cursor` | ❌ 불필요 | cursor-agent 실행 (동기/비동기) | ✅ 가능 | v1.0 |
| `GET /api/jobs/:id` | ❌ 불필요 | 작업 결과 조회 | ✅ 가능 | v1.3 |
| `GET /api/jobs` | ❌ 불필요 | 작업 목록 조회 (필터링/페이징) | ✅ 가능 | v1.3 |
| `GET /api/config/project-path` | ❌ 불필요 | 현재 프로젝트 경로 조회 | ✅ 가능 | v1.2 |
| `POST /api/config/project-path` | ❌ 불필요 | 프로젝트 경로 설정/변경 | ✅ 가능 | v1.2 |
| `POST /slack/cursor` | ✅ HMAC 필요 | Slack 슬래시 커맨드 전용 | ❌ 불가 | v1.0 |
| `GET /health` | ❌ 불필요 | 서버 상태 확인 | ✅ 가능 | v1.0 |

### Swagger 문서 재생성

코드 주석을 수정한 후 Swagger 문서를 업데이트하려면:

```bash
swag init -g cmd/server/main.go -o docs
```

## Health Check

서버 상태 확인:

```bash
curl http://localhost:8080/health
```

응답:
```json
{"status":"ok"}
```

또는 Swagger UI에서 `/health` 엔드포인트를 직접 테스트할 수 있습니다.

## 보안

v1.3에서 구현된 보안 기능:

- ✅ **HMAC-SHA256 서명 검증**: Slack에서 온 요청인지 확인 (모든 `/slack/*` 엔드포인트)
- ✅ **타임스탬프 검증**: 5분 이상 오래된 요청 거부 (Replay Attack 방어)
- ✅ **Context Timeout**: cursor-agent 실행 시 120초 타임아웃
- ✅ **Process Group 관리**: 타임아웃 시 자식 프로세스까지 모두 종료
- ✅ **SSRF 방어**: Slack 응답 URL 도메인 검증 (`hooks.slack.com`만 허용)
- ✅ **동적 프로젝트 경로 검증**: 설정되지 않은 경우 실행 거부
- ✅ **작업 디렉토리 격리**: cursor-agent는 지정된 프로젝트 경로에서만 실행

**보안 모범 사례:**
- 프로덕션 환경에서는 HTTPS를 사용하세요
- Signing Secret은 절대 코드에 하드코딩하지 마세요
- `.env` 파일은 `.gitignore`에 추가되어 있어야 합니다

## 문제 해결

### "SLACK_SIGNING_SECRET 환경변수가 설정되지 않았습니다"

환경 변수를 올바르게 설정했는지 확인합니다:

```bash
# .env 파일 확인
cat .env | grep SLACK_SIGNING_SECRET

# 또는 시스템 환경변수 확인
echo $SLACK_SIGNING_SECRET
```

**해결 방법:**
1. Slack API 사이트 → 앱 선택 → Basic Information → Signing Secret 복사
2. `.env` 파일에 `SLACK_SIGNING_SECRET=your_secret_here` 추가
3. 서버 재시작

### "Signature mismatch"

**원인:**
- Slack App의 Signing Secret이 잘못되었거나
- ngrok URL이 변경되었거나
- Request URL이 잘못 설정되었을 수 있습니다

**해결 방법:**
1. Slack App의 Signing Secret이 `.env` 파일의 값과 일치하는지 확인
2. ngrok URL이 변경되었다면 Slack App 설정의 Request URL 업데이트
3. Request URL이 정확히 `https://your-domain.com/slack/cursor` 형식인지 확인 (끝에 `/` 없음)

### Slack에서 명령어가 작동하지 않음

**확인 사항:**
1. 서버가 실행 중인지 확인: `curl http://localhost:8080/health`
2. ngrok이 실행 중이고 URL이 Slack에 올바르게 설정되었는지 확인
3. Slack 앱이 워크스페이스에 설치되었는지 확인
4. 서버 로그에서 에러 메시지 확인

### "cursor-agent: command not found"

cursor-agent CLI가 PATH에 설정되어 있는지 확인:

```bash
which cursor-agent
```

또는 `.env` 파일에 `CURSOR_CLI_PATH`를 설정하세요.

### "CURSOR_PROJECT_PATH가 설정되지 않았습니다"

v1.2부터 프로젝트 경로는 동적으로 설정됩니다:

```bash
# API로 설정
curl -X POST http://localhost:8080/api/config/project-path \
  -H "Content-Type: application/json" \
  -d '{"project_path": "/path/to/project"}'

# 또는 Slack에서
/cursor set-path /path/to/project
```

## 빌드 및 배포

### 크로스 컴파일

여러 플랫폼용 실행 파일을 한 번에 빌드:

```bash
./build.sh
```

빌드 결과 (`dist/` 디렉토리):
- **macOS Intel/ARM** (SQLite 포함)
- **Linux x86_64/ARM64** (순수 Go, SQLite 제외)
- **Windows x86_64** (순수 Go, SQLite 제외) ✅ v1.3.1

자세한 내용: [DEPLOY.md](./DEPLOY.md)

### 배포

1. `dist/` 디렉토리에서 플랫폼에 맞는 파일 선택
2. 사용자에게 실행 파일 전달
3. 사용자 실행:
   ```bash
   # 처음 실행 (설정 마법사)
   ./실행파일 --setup
   
   # 서버 시작 (ngrok 자동 실행!)
   ./실행파일
   ```

**Go 설치 불필요!** 실행 파일만으로 모든 것이 동작합니다.

**자동 기능:**
- ✅ 서버 자동 시작
- ✅ ngrok 자동 실행 (설치되어 있는 경우)
- ✅ ngrok URL 자동 출력
- ✅ Ctrl+C로 깔끔한 종료

## 다음 단계 (원 단계)

v1.3 완료! 다음 기능들을 추가할 예정입니다:

- [ ] Worker Pool + Job Queue 아키텍처 (동시성 제어)
- [ ] Viper를 사용한 설정 관리
- [ ] Redis 기반 분산 작업 큐
- [ ] Webhook 기반 작업 완료 알림
- [ ] 작업 취소 기능
- [ ] 웹 UI 대시보드

## 참고 자료

- **[SETUP.md](./SETUP.md)** - 제3자를 위한 완전한 설치 가이드
- **[DEPLOY.md](./DEPLOY.md)** - 빌드 및 배포 가이드
- **[docs/technical/](./docs/technical/)** - 기술 설계 문서 및 아키텍처
- **[토이 프로젝트 무료 배포 전략 비교.md](./토이%20프로젝트%20무료%20배포%20전략%20비교.md)** - 배포 전략 가이드

