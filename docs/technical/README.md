# 기술 문서 가이드

## 📚 문서 읽는 순서

### 1️⃣ [필독] 설계-수정본-v1.1.md
**상태**: ✅ 최신 (2025-01)  
**목적**: cursor-cli 올바른 사용법 + 수정된 아키텍처

**반드시 확인할 내용:**
- cursor-agent의 실제 동작 방식
- `--force` 플래그 필수 (파일 수정 허용)
- `--files` 플래그 없음 (자연어 프롬프트 사용)
- 환경변수 `CURSOR_PROJECT_BASE_PATH` 필수화

**Dev Agent 작업 시**: 이 문서를 **우선 기준**으로 사용하세요.

---

### 2️⃣ [참고] Golang Slack-Cursor-CLI 연동 서버 고도화.md
**상태**: ⚠️ v1.0 (cursor-cli 부분만 outdated)  
**목적**: 상세 구현 레퍼런스

**여전히 유효한 내용:**
- ✅ Worker Pool + Dispatcher + TaskExecutor 상세 설계
- ✅ HMAC 인증 + Replay Attack 방어 상세
- ✅ SSRF 방어 상세
- ✅ Process Group 관리 상세
- ✅ Viper 설정 관리 상세
- ✅ 80개+ 참고 자료 링크

**무시해야 할 내용:**
- ❌ `--files` 플래그 관련 모든 내용
- ❌ shlex/pflag 파싱 로직
- ❌ 파일 경로 검증 (Path Traversal) - cursor-cli는 프로젝트 전체를 컨텍스트로 사용

**Dev Agent 작업 시**: v1.1에 없는 **상세 구현 세부사항**을 참고하세요.

---

### 3️⃣ [배포] 토이 프로젝트 무료 배포 전략 비교.md
**상태**: ✅ 유효 (cursor-cli 독립적)  
**목적**: 배포 전략 및 가이드

**포함 내용:**
- ngrok vs Cloudflare Tunnel vs Cloud Run 비교
- Docker 멀티스테이지 빌드 가이드
- Google Cloud Run 배포 가이드
- 무료 PaaS 옵션 스크리닝

**Dev Agent 작업 시**: 배포 단계에서 참고하세요.

---

## 🎯 Dev Agent 작업 가이드

### 점 단계 (현재) 수정 작업

```bash
@dev """
@설계-수정본-v1.1.md 를 기준으로 다음을 수정해줘:

1. handlers.go의 executeCursorCLI 함수
   - --force 플래그 추가
   - --files 관련 로직 제거
   
2. parseCommand 함수 단순화
   - 파일 경로 파싱 제거
   - 전체 텍스트를 프롬프트로 처리
   
3. README.md 사용 예시 업데이트
   - /cursor "자연어 프롬프트" 형식으로 변경
"""
```

### 원 단계 (다음) 고도화 작업

```bash
@dev """
@설계-수정본-v1.1.md 의 아키텍처를 따르되,
@Golang Slack-Cursor-CLI 연동 서버 고도화.md 의
상세 구현을 참고해서 Worker Pool을 구현해줘.

단, cursor-cli 관련 부분은 v1.1 기준을 따를 것.
"""
```

---

## 📊 문서 버전 비교

| 항목 | v1.0 (고도화 문서) | v1.1 (수정본) |
|-----|-------------------|--------------|
| cursor-cli 파라미터 | ❌ `--files` (오류) | ✅ `--force` (올바름) |
| 슬래시 커맨드 | ❌ `/cursor "..." --files ...` | ✅ `/cursor "자연어"` |
| 파싱 로직 | ❌ shlex + pflag (과도) | ✅ TrimSpace (단순) |
| 프로젝트 경로 | ⚠️ 선택 (위험) | ✅ 필수 (안전) |
| Worker Pool 설계 | ✅ 매우 상세 | ✅ 개요 |
| 보안 설계 | ✅ 매우 상세 | ✅ 핵심 |
| 참고 자료 | ✅ 80+ 링크 | ❌ 없음 |
| 배포 전략 | ❌ 없음 | ❌ 없음 |

---

## 🔄 다음 액션

### 즉시 (P0):
- [ ] handlers.go cursor-cli 파라미터 수정
- [ ] README.md 사용 예시 업데이트
- [ ] 환경변수 검증 강화

### 곧 (P1):
- [ ] 기존 고도화 문서의 cursor-cli 부분만 v1.1로 업데이트
- [ ] 또는 "통합 마스터 문서" 생성 검토

### 나중 (P2):
- [ ] Worker Pool 구현 (원 단계)
- [ ] 배포 전략 적용 (Cloud Run)

---

**요약**: 세 문서 모두 유용합니다. v1.1은 "올바른 방향", 고도화 문서는 "상세 구현", 배포 문서는 "배포 가이드"입니다.

