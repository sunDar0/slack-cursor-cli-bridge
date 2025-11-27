# 배포 가이드

이 문서는 빌드된 실행 파일을 제3자에게 배포하는 방법을 설명합니다.

## 🔨 빌드 방법

### 크로스 컴파일 스크립트 사용

```bash
./build.sh
```

이 스크립트는 다음 플랫폼용 바이너리를 자동으로 생성합니다:
- macOS Intel (x86_64)
- macOS Apple Silicon (arm64)
- Linux x86_64
- Linux ARM64
- **Windows x86_64** ✅ (v1.3.1부터 지원)

빌드된 파일은 `dist/` 디렉토리에 생성됩니다.

---

## 📦 빌드 결과

```
dist/
├── slack-cursor-hook-darwin-amd64        # macOS Intel (SQLite 포함)
├── slack-cursor-hook-darwin-amd64-nocgo  # macOS Intel (순수 Go)
├── slack-cursor-hook-darwin-arm64        # macOS M1/M2/M3 (SQLite 포함)
├── slack-cursor-hook-darwin-arm64-nocgo  # macOS M1/M2/M3 (순수 Go)
├── slack-cursor-hook-linux-amd64-nocgo   # Linux x86_64 (순수 Go)
├── slack-cursor-hook-linux-arm64-nocgo   # Linux ARM64 (순수 Go)
└── slack-cursor-hook-windows-amd64.exe   # Windows x86_64 (순수 Go)
```

### CGO vs No-CGO 차이

| 특징 | CGO 버전 | No-CGO 버전 |
|------|----------|-------------|
| **SQLite 지원** | ✅ 완전 지원 | ❌ 미지원 |
| **작업 결과 저장** | ✅ 가능 | ❌ 불가능 |
| **크로스 컴파일** | ⚠️ 복잡함 | ✅ 쉬움 |
| **의존성** | C 컴파일러 필요 | 없음 |
| **파일 크기** | ~23MB | ~21MB |

**권장:** macOS 사용자는 CGO 버전 사용, Linux 사용자는 no-CGO 버전 사용

---

## 🚀 제3자 배포 방법

### 1. 플랫폼별 파일 선택

사용자의 운영체제와 아키텍처에 맞는 파일을 제공:

| 사용자 환경 | 파일명 |
|------------|--------|
| macOS Intel | `slack-cursor-hook-darwin-amd64` |
| macOS M1/M2/M3 | `slack-cursor-hook-darwin-arm64` |
| Linux x86_64 | `slack-cursor-hook-linux-amd64-nocgo` |
| Linux ARM64 | `slack-cursor-hook-linux-arm64-nocgo` |
| Windows x86_64 | `slack-cursor-hook-windows-amd64.exe` |

### 2. 사용자 설치 가이드

사용자에게 다음 단계를 안내:

```bash
# 1. 실행 권한 부여
chmod +x slack-cursor-hook-*

# 2. 원하는 위치로 이동 (선택사항)
mv slack-cursor-hook-* ~/bin/cursor-server

# 3. 설정 마법사 실행
./cursor-server --setup
```

설정 마법사가 자동으로 수행하는 작업:
- ✅ cursor-agent 설치 확인 및 자동 설치
- ✅ ngrok 설치 확인 및 자동 설치
- ✅ 환경 변수 대화형 입력
- ✅ 프로젝트 초기화

### 3. 서버 실행

설정 완료 후:

```bash
./cursor-server
```

---

## 🛠️ 고급 빌드 옵션

### 특정 플랫폼만 빌드

```bash
# macOS ARM64만 빌드
GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -o dist/server-macos cmd/server/main.go

# Linux AMD64 (no-CGO)
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o dist/server-linux cmd/server/main.go
```

### 최소 크기 빌드

```bash
# UPX로 압축 (선택사항)
upx --best --lzma dist/slack-cursor-hook-*

# 압축 후 크기: ~7-10MB
```

### Docker를 통한 Linux CGO 빌드

Linux에서 SQLite를 포함한 빌드가 필요한 경우:

```bash
docker run --rm \
  -v "$PWD":/app \
  -w /app \
  golang:1.23 \
  bash -c "apt-get update && apt-get install -y gcc && \
           go build -o dist/slack-cursor-hook-linux-amd64 cmd/server/main.go"
```

---

## 📋 시스템 요구사항

### 최종 사용자 시스템

**필수:**
- macOS 10.15+ 또는 Linux (kernel 3.2+)
- curl (대부분 기본 설치됨)

**권장:**
- Homebrew (macOS) - ngrok 자동 설치용
- snap (Linux) - ngrok 자동 설치용

**불필요:**
- ❌ Go 설치 불필요
- ❌ 빌드 도구 불필요
- ❌ C 컴파일러 불필요

### 빌드 환경 (개발자)

**필수:**
- Go 1.21+
- gcc (CGO 빌드 시)

**선택사항:**
- Docker (Linux CGO 빌드)
- UPX (바이너리 압축)

---

## ⚠️ 알려진 제한사항

### Windows 제한 사항

**✅ Windows는 지원됩니다!** (v1.3.1부터)

단, 다음 제약이 있습니다:
- **SQLite 미지원**: Windows 빌드는 CGO 없이 빌드되므로 SQLite 기능이 제외됩니다
  - 작업 결과 저장/조회 API (`/api/jobs`) 사용 불가
  - 다른 모든 기능은 정상 동작
  
- **프로세스 관리 차이**:
  - Unix: Process Group으로 자식 프로세스까지 모두 종료
  - Windows: 메인 프로세스만 종료 (자식 프로세스는 남을 수 있음)

**구현 방식:**
- `internal/server/process_unix.go`: macOS/Linux용 (`Setpgid`, `Kill -pid`)
- `internal/server/process_windows.go`: Windows용 (`CREATE_NEW_PROCESS_GROUP`, `Process.Kill()`)

### Linux CGO 크로스 컴파일

macOS에서 Linux용 CGO 빌드는 복잡합니다:
- C 크로스 컴파일러 필요
- 타겟 플랫폼 라이브러리 필요

**해결책:**
- Docker 사용
- 또는 no-CGO 버전 제공 (SQLite 제외)

---

## 🎯 배포 체크리스트

빌드 전:
- [ ] `go mod tidy` 실행
- [ ] 버전 태그 생성 (`git tag v1.3.0`)
- [ ] 코드 테스트 완료

빌드:
- [ ] `./build.sh` 실행
- [ ] `dist/` 디렉토리 확인
- [ ] 각 바이너리 파일 타입 확인 (`file dist/*`)

배포:
- [ ] GitHub Release 생성
- [ ] 각 플랫폼 바이너리 첨부
- [ ] SETUP.md 링크 제공
- [ ] 체인지로그 작성

---

## 📚 추가 자료

- [SETUP.md](./SETUP.md) - 사용자용 설치 가이드
- [README.md](./README.md) - 프로젝트 개요
- [build.sh](./build.sh) - 크로스 컴파일 스크립트
- [docs/technical/deployment-strategy.md](./docs/technical/deployment-strategy.md) - 배포 전략 가이드

---

## 💡 팁

### GitHub Actions로 자동 빌드

`.github/workflows/release.yml`:

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.23'
      
      - name: Build
        run: ./build.sh
      
      - name: Create Release
        uses: softprops/action-gh-release@v1
        with:
          files: dist/*
```

### 버전 정보 임베드

`build.sh`는 자동으로 다음을 임베드:
- Git 태그/커밋 해시
- 빌드 시간

확인:
```bash
./dist/slack-cursor-hook-darwin-arm64 --version
```

---

## 📞 문의

빌드 또는 배포 관련 문제가 있으면 Issue를 등록해주세요.

