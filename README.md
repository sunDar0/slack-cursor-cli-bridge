# Slack-Cursor-CLI 연동 서버 (v1.4)

Slack 슬래시 커맨드를 통해 로컬 또는 서버의 **Cursor AI**를 원격으로 제어하는 Go 기반 서버입니다.

## 🌟 주요 기능

- **Slack 연동**: `/cursor "자연어 프롬프트"` 명령으로 AI에게 작업 지시
- **안전한 실행**:
  - **Worker Pool**: 동시 작업 수 제한으로 시스템 과부하 방지 (v1.4)
  - **보안 검증**: HMAC 서명, 타임스탬프 검증, SSRF 방어
  - **프로세스 관리**: 타임아웃 시 자식 프로세스까지 깔끔하게 종료
- **작업 관리**:
  - SQLite 데이터베이스에 모든 작업 이력 및 결과 저장
  - 작업 상태 조회 API 제공
- **사용 편의성**:
  - **설정 마법사**: `--setup` 플래그로 초기 설정 자동화
  - **ngrok 자동화**: 개발 환경에서 터널링 자동 수행
  - **동적 경로**: 실행 중 작업 대상 프로젝트 경로 변경 가능

## 📚 문서 가이드

이 프로젝트는 상세한 문서를 제공합니다. 목적에 맞는 문서를 확인하세요.

| 문서 | 설명 | 대상 |
| :--- | :--- | :--- |
| **[SETUP.md](./SETUP.md)** | **설치 가이드**. 처음 시작하는 분들을 위한 단계별 안내 | 사용자 |
| **[DEPLOY.md](./DEPLOY.md)** | **배포 가이드**. 바이너리 빌드 및 배포 방법 | 운영자 |
| **[Architecture](./docs/technical/architecture.md)** | **시스템 아키텍처**. 내부 동작 원리 및 기술적 세부 사항 | 개발자 |
| **[Deployment](./docs/technical/deployment-strategy.md)** | **배포 전략 비교**. ngrok, Cloudflare, Cloud Run 등 전략 분석 | 운영자 |

## 🚀 빠른 시작

### 1. 설정 마법사 실행 (추천)
모든 의존성(cursor-agent, ngrok) 확인 및 환경 설정을 자동으로 수행합니다.

```bash
go run cmd/server/main.go --setup
```

### 2. 서버 시작
개발 모드로 시작하면 ngrok 터널까지 자동으로 생성됩니다.

```bash
./start-dev.sh
```

## 💻 사용 방법

### Slack 명령어
```bash
# AI에게 작업 지시 (자연어)
/cursor "main.go의 버그를 수정해줘"

# 작업 프로젝트 경로 변경
/cursor set-path /Users/username/projects/my-project

# 도움말
/cursor help
```

### API 엔드포인트
- `POST /api/cursor`: 작업 요청 (Slack과 동일)
- `GET /api/jobs`: 작업 목록 조회
- `GET /api/jobs/:id`: 특정 작업 결과 조회

## 🛠 기술 스택
- **Language**: Go 1.22+
- **Web Framework**: Gin
- **Database**: SQLite (mattn/go-sqlite3)
- **Process Management**: `os/exec`, `syscall` (Process Groups)
- **CLI Tool**: Cursor Agent CLI

## 🔄 변경 이력
- **v1.4**: Worker Pool 활성화 (안정성 강화)
- **v1.3**: SQLite 작업 이력 저장 및 조회 기능
- **v1.2**: 동적 프로젝트 경로 관리
- **v1.1**: 자연어 프롬프트 기반 아키텍처 적용
- **v1.0**: 초기 구현
