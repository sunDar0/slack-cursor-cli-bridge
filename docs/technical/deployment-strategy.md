# Deployment Strategy Comparison

이 문서는 Slack-Cursor-Hook 서버를 외부(Slack)에서 접근 가능하도록 배포하기 위한 다양한 전략을 비교 분석합니다. 토이 프로젝트 및 개인 사용 목적에 최적화된 무료 솔루션을 중점으로 다룹니다.

---

## 1. 배포 전략 요약

| 전략 | 비용 | 난이도 | 안정성 | 추천 대상 |
| :--- | :--- | :--- | :--- | :--- |
| **1. Local + ngrok** | 무료 (제한적) | ⭐ (매우 쉬움) | ⚠️ (URL 변경됨) | **개발 및 테스트 단계**, 단기 사용 |
| **2. Local + Cloudflare Tunnel** | **무료** | ⭐⭐ (쉬움) | ⭐⭐⭐⭐⭐ (고정 도메인) | **개인 사용**, 홈 서버 운영 |
| **3. Cloud Run (GCP)** | 무료 티어 내 | ⭐⭐⭐ (중간) | ⭐⭐⭐⭐⭐ (완전 관리형) | **안정적인 상시 운영**, 서버리스 선호 시 |
| **4. Fly.io** | 무료 티어 내 | ⭐⭐ (쉬움) | ⭐⭐⭐⭐ | 간단한 컨테이너 배포 |

---

## 2. 상세 분석

### 전략 1: Localhost + ngrok (현재 기본 방식)
서버를 로컬 노트북/데스크탑에서 실행하고, ngrok을 통해 외부 터널을 뚫는 방식입니다. 프로젝트에 내장된 기능으로 가장 빠르게 시작할 수 있습니다.

- **장점**: 별도 서버 불필요, 설정 마법사로 자동화됨.
- **단점**: 무료 버전은 재시작 시 URL이 변경됨 (Slack 설정 매번 변경 필요), 로컬 컴퓨터가 켜져 있어야 함.
- **설정**: `./start-dev.sh` 실행 시 자동 적용.

### 전략 2: Localhost + Cloudflare Tunnel (추천)
ngrok과 유사하지만, **무료로 고정 도메인**을 사용할 수 있고 연결이 더 안정적입니다.

- **장점**: 무료, 고정 URL 사용 가능, 보안 우수.
- **단점**: Cloudflared 설치 및 초기 설정 필요.
- **설정 방법**:
  1. `cloudflared` 설치
  2. `cloudflared tunnel login`
  3. 터널 생성 및 실행: `cloudflared tunnel run --url http://localhost:8080`

### 전략 3: Google Cloud Run (Docker)
Docker 컨테이너를 클라우드에 배포하는 방식입니다. 호출될 때만 과금되는 서버리스 구조입니다.

- **장점**: 컴퓨터를 꺼도 됨, 높은 가용성, HTTPS 자동 적용.
- **단점**: Docker 이미지 빌드 필요, Cold Start(초기 지연) 있을 수 있음.
- **비용**: 월 200만 건 요청까지 무료 티어 제공 (개인 용도로 충분).

#### Dockerfile 예시 (멀티 스테이지 빌드)
```dockerfile
# Build Stage
FROM golang:1.23-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY . .
RUN go build -o server cmd/server/main.go

# Runtime Stage
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/server .
# cursor-agent 설치 (Linux용)
RUN apk add --no-cache curl bash && \
    curl -fsSL https://cursor.com/install | bash
CMD ["./server"]
```

---

## 3. 결론 및 추천

1. **개발 및 테스트**: 프로젝트에 내장된 **ngrok** 기능을 사용하세요. (`--setup` 및 `start-dev.sh` 활용)
2. **개인 상시 운영**: 집에 켜두는 맥미니나 서버가 있다면 **Cloudflare Tunnel**이 최고의 무료 옵션입니다.
3. **완전한 클라우드 운영**: 로컬 의존성을 없애고 싶다면 **Google Cloud Run**에 Docker 이미지를 배포하세요.

