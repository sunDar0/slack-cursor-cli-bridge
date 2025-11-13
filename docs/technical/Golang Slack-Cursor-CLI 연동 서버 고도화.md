
# **Golang 기반 Slack-Cursor-CLI 연동 서버: 운영-수준 아키텍처 및 보안 설계 보고서**

---

> **⚠️ 중요: 이 문서는 v1.0 기준으로 작성되었습니다**
>
> **cursor-cli 사용법이 v1.1에서 변경되었습니다:**
> - ❌ **제거됨**: `--files` 플래그 및 관련 파싱 로직 (shlex/pflag)
> - ✅ **추가됨**: `--force` 플래그 (파일 수정 허용)
> - ✅ **변경됨**: 자연어 프롬프트만 사용 (파일은 AI가 자동 탐지)
>
> **최신 cursor-cli 사용법은 `docs/technical/설계-수정본-v1.1.md`를 참조하세요.**
>
> **여전히 유효한 내용:**
> - ✅ Worker Pool + Dispatcher + TaskExecutor 아키텍처
> - ✅ HMAC 인증 + Replay Attack 방어
> - ✅ SSRF 방어
> - ✅ Process Group 관리
> - ✅ Viper 설정 관리
> - ✅ 80개+ 참고 자료
>
> **구현 상태:**
> - ✅ v1.3: SQLite 기반 작업 결과 저장 및 조회
> - ✅ v1.2: 동적 프로젝트 경로 관리
> - ✅ v1.1: cursor-agent `--force` 플래그 적용
> - ⏳ Worker Pool: 원 단계에서 구현 예정

---

## **I. 시스템 아키텍처: 운영-수준의 슬래시 커맨드 처리기**

### **A. 초기 아키텍처의 위험성 분석: 리소스 고갈**

초기 기술 명세는 Slack 슬래시 커맨드(Slash Command) 요청을 수신할 때마다 go runCursorTask(payload)를 호출하여 새로운 Goroutine을 생성하는 모델을 제안합니다. 이 아키텍처는 기능적으로는 동작하지만, 운영 환경에서는 심각한 안정성 위험을 내포합니다.

os/exec를 사용하여 외부 프로세스를 실행하는 작업은 Go 런타임에서 비용이 매우 높은 작업입니다. 특히, 자식 프로세스의 종료를 기다리는 cmd.Wait() 호출은 Go 런타임이 해당 대기를 처리하기 위해 새로운 OS 스레드(Thread)를 생성하도록 강제할 수 있습니다.1 각 스레드는 커널 리소스와 상당한 크기의 스택 메모리(예: 2MB)를 소비합니다.1

만약 Slack에서 짧은 시간 내에 많은 수의 슬래시 커맨드 요청이 폭주(Thundering Herd)할 경우, 이 아키텍처는 통제 없이 수백, 수천 개의 Goroutine과 OS 스레드를 생성하게 됩니다.2 이는 결국 서버의 메모리 고갈(Out-of-Memory, OOM), 스레드 한계 도달, 또는 과도한 가비지 컬렉션(GC) 압력으로 이어져 전체 서비스가 예측 불가능하게 중단(Crash)되는 결과를 초래합니다.1 이는 시스템의 명백한 장애 지점(Single Point of Failure)입니다.

### **B. 제안 아키텍처: 'Job Queue \-\> Dispatcher \-\> Worker Pool'**

이러한 리소스 고갈 취약점을 근본적으로 해결하기 위해, 본 문서는 HTTP 요청 수신(Gin)과 고비용 작업 실행(os/exec)을 완전히 분리(Decoupling)하는 **작업자 풀(Worker Pool) 아키텍처**로의 전면적인 재설계를 제안합니다.4 이 모델은 시스템이 처리할 수 있는 최대 동시 os/exec 작업 수를 명시적으로 제어하여, 요청량에 관계없이 시스템의 리소스 사용률을 안정적으로 유지합니다.6

참고한 고성능 큐 아키텍처 7에 기반하여, 시스템은 다음 세 가지 핵심 구성 요소로 재편됩니다:

1. **Job Queue (chan Job):** 모든 수신 작업을 보관하는 중앙 버퍼 채널(Buffered Channel)입니다. Gin 핸들러는 작업을 이 큐에 추가하고 즉시 응답을 반환합니다.7  
2. **Dispatcher:** 작업자 풀을 총괄 관리합니다. Job Queue에서 작업을 가져와 현재 유휴(Idle) 상태인 Worker에게 작업을 분배(Dispatch)하는 역할을 합니다.7  
3. **Worker Pool:** 시스템의 최대 동시성을 제어하기 위해 *고정된 수*(예: N=5)의 Worker Goroutine을 미리 생성하여 실행합니다.9 각 Worker는 Dispatcher로부터 작업을 할당받아 cursor-cli 실행이라는 실제 작업을 순차적으로 처리합니다.

이 아키텍처는 동시에 실행되는 os/exec의 최대 개수를 N개(Worker의 수)로 엄격하게 제한하여 4, 1에서 경고한 스레드 및 메모리 고갈 문제를 원천적으로 방지합니다.

### **C. 고도화된 데이터 흐름 (재정의)**

1. \*\*\*\* 사용자가 /cursor "프롬프트" \--files "file.go"를 입력합니다.  
2. **\[Gin\]** POST /slack/cursor 엔드포인트가 요청을 수신합니다.  
3. **\[Middleware\]** SlackAuthMiddleware가 요청의 HMAC 서명과 타임스탬프를 검증합니다.10  
4. **\[Handler\]** handleSlashCursor 핸들러가 실행됩니다.  
   * Slack에 3초 이내에 즉각적인 200 OK (ACK) 응답을 전송합니다.  
   * 요청 페이로드로 Job 객체를 생성합니다.  
   * 생성된 Job을 Dispatcher.JobQueue \<- newJob을 통해 중앙 작업 큐에 전송합니다.7  
5. \*\*\*\* (백그라운드 Goroutine에서 실행)  
   * JobQueue에서 newJob을 수신합니다.  
   * Worker Pool에서 유휴 작업자(Worker)의 작업 채널을 대기합니다.  
   * selectedWorker.WorkChannel \<- newJob으로 작업을 할당합니다.  
6. \*\*\*\* (백그라운드 Goroutine, N개 중 하나)  
   * job := \<- WorkChannel을 통해 작업을 수신합니다.  
   * TaskExecutor.Run(job)을 호출하여, **파싱 \-\> 보안 검증 \-\> os/exec 실행 \-\> 타임아웃 처리**의 전체 파이프라인을 수행합니다.11  
   * 실행 완료 후, sendDelayedResponse를 호출하여 Slack response\_url로 결과를 전송합니다.  
7. \*\*\*\* (SSRF 검증 통과 후) 사용자가 비동기적으로 전송된 cursor-cli 실행 결과를 채널에서 확인합니다.14

## **II. 환경 설정 및 비밀키 관리 (Viper)**

### **A. 설계 원칙: 설정의 외부화 (Externalized Configuration)**

운영-수준의 애플리케이션은 비밀키(예: SLACK\_SIGNING\_SECRET), API 키, 또는 배포 환경에 따라 달라지는 설정(예: projectBasePath)을 소스 코드 내에 하드코딩해서는 안 됩니다.15 이는 심각한 보안 취약점이며, 설정을 변경할 때마다 코드를 다시 컴파일하고 배포해야 하는 경직성을 초래합니다.

본 시스템은 12-Factor App 원칙 16을 준수하기 위해 Go 애플리케이션을 위한 완벽한 설정 솔루션인 **Viper** 17를 채택합니다. Viper는 다음 우선순위에 따라 설정을 계층적으로 병합(Merge)하여 유연성과 보안을 동시에 확보합니다 17:

1. 명시적인 Set 호출  
2. 환경 변수 (Environment Variables)  
3. 설정 파일 (예: config.yaml)  
4. 기본값 (Defaults)

### **B. config/config.go 모듈 의사코드**

모든 설정은 단일 Config 구조체를 통해 타입-세이프(Type-safe)하게 관리됩니다. Viper는 설정 파일과 환경 변수를 이 구조체로 자동으로 Unmarshal 합니다.18

Go

// package config  
//  
// 이 파일은 Viper를 사용한 애플리케이션 설정 로드 및 관리를 담당합니다.  
// \[17, 18, 19, 20, 21, 24\]의 모범 사례를 따릅니다.

import (  
    "log"  
    "strings"  
    "github.com/spf13/viper"  
)

// Config는 애플리케이션의 모든 설정을 계층적으로 정의합니다.  
// \`mapstructure\` 태그는 Viper가 Unmarshal할 때 사용하는 키입니다.  
type Config struct {  
    Server   ServerConfig   \`mapstructure:"server"\`  
    Slack    SlackConfig    \`mapstructure:"slack"\`  
    Cursor   CursorConfig   \`mapstructure:"cursor"\`  
    Security SecurityConfig \`mapstructure:"security"\`  
    Worker   WorkerConfig   \`mapstructure:"worker"\`  
}

type ServerConfig struct {  
    Port string \`mapstructure:"port"\`  
}

type SlackConfig struct {  
    // (보안) SigningSecret은 파일에 절대 저장되어서는 안 되며,  
    // 오직 환경 변수(SLACK\_SIGNING\_SECRET)를 통해서만 주입되어야 합니다.  
    SigningSecret string \`mapstructure:"signing\_secret"\`  
}

type CursorConfig struct {  
    ProjectBasePath      string \`mapstructure:"project\_base\_path"\`  
    ExecutionTimeoutSeconds int    \`mapstructure:"execution\_timeout\_seconds"\`  
}

type SecurityConfig struct {  
    AllowedResponseDomainsstring \`mapstructure:"allowed\_response\_domains"\`  
}

type WorkerConfig struct {  
    PoolSize  int \`mapstructure:"pool\_size"\` // 동시 실행 작업자 수  
    QueueSize int \`mapstructure:"queue\_size"\` // 인-메모리 작업 큐 크기  
}

// LoadConfig는 설정 파일(예: config.yaml)과 환경 변수를 읽어 Config 구조체로 반환합니다.  
func LoadConfig() (config Config, err error) {  
    // 1\. 파일 설정 (선택적)   
    viper.AddConfigPath("./config") // 검색 경로 1  
    viper.AddConfigPath(".")        // 검색 경로 2  
    viper.SetConfigName("config")   // 파일 이름 (확장자 제외)  
    viper.SetConfigType("yaml")     // 파일 타입

    // 설정 파일을 읽어들입니다. 파일이 없어도 오류가 아닙니다.  
    // 환경 변수만으로도 동작할 수 있어야 합니다.  
    \_ \= viper.ReadInConfig()

    // 2\. 환경 변수 설정 (필수)   
    // 환경 변수를 자동으로 읽어들입니다.  
    viper.AutomaticEnv()  
      
    // 환경 변수 키와 구조체 필드 매핑을 위한 리플레이서 설정 \[19, 20\]  
    // 예: security.allowed\_response\_domains \-\> SECURITY\_ALLOWED\_RESPONSE\_DOMAINS  
    viper.SetEnvKeyReplacer(strings.NewReplacer(".", "\_"))

    // 3\. (중요) 환경 변수를 Viper 키에 명시적으로 바인딩 \[21\]  
    // 이는 구조체 Unmarshal 시 환경 변수를 직접 매핑하는 데 도움을 줍니다.  
    // 특히, 파일에 해당 키가 \*전혀\* 없을 때 유용합니다.  
    viper.BindEnv("slack.signing\_secret", "SLACK\_SIGNING\_SECRET")  
    viper.BindEnv("cursor.project\_base\_path", "CURSOR\_PROJECT\_BASE\_PATH")  
    //... 기타 환경 변수 바인딩...

    // 4\. 기본값 설정   
    // 설정 파일이나 환경 변수에 값이 없을 경우 사용될 기본값입니다.  
    viper.SetDefault("server.port", "8080")  
    viper.SetDefault("cursor.execution\_timeout\_seconds", 120) // 2분 타임아웃  
    viper.SetDefault("worker.pool\_size", 5)                   // 동시 실행 5개  
    viper.SetDefault("worker.queue\_size", 100)  
    viper.SetDefault("security.allowed\_response\_domains",string{"hooks.slack.com"})

    // 5\. Viper 설정을 Config 구조체로 Unmarshal   
    err \= viper.Unmarshal(\&config)  
    if err\!= nil {  
        return Config{}, err  
    }

    // 6\. (보안) 필수 값 검증 \[15, 22\]  
    // 특정 값(특히 비밀키)은 반드시 제공되어야 합니다.  
    if config.Slack.SigningSecret \== "" {  
        log.Fatalf("FATAL: SLACK\_SIGNING\_SECRET 환경 변수가 설정되지 않았습니다. 서버를 시작할 수 없습니다.")  
    }  
    if config.Cursor.ProjectBasePath \== "" {  
        log.Fatalf("FATAL: CURSOR\_PROJECT\_BASE\_PATH 환경 변수가 설정되지 않았습니다. 서버를 시작할 수 없습니다.")  
    }

    return config, nil  
}

### **C. 표 1: 필수 설정 파라미터**

이 표는 시스템 운영자가 서버를 배포하고 구성하는 데 필요한 모든 설정 변수를 정의합니다.

| 설정 키 (Key) | 환경 변수 (Env Var) | 타입 | 설명 | 필수 |
| :---- | :---- | :---- | :---- | :---- |
| server.port | SERVER\_PORT | string | Gin 서버가 수신 대기할 포트 | 아니오 (기본값: 8080\) |
| slack.signing\_secret | SLACK\_SIGNING\_SECRET | string | Slack 앱의 Signing Secret. **절대 파일에 저장 금지.** | **예** |
| cursor.project\_base\_path | CURSOR\_PROJECT\_BASE\_PATH | string | cursor-cli가 실행될 프로젝트의 절대 경로. (경로 조작 방어의 기준점) | **예** |
| cursor.execution\_timeout\_seconds | CURSOR\_EXECUTION\_TIMEOUT\_SECONDS | int | cursor-cli 명령의 최대 실행 시간(초) | 아니오 (기본값: 120\) |
| worker.pool\_size | WORKER\_POOL\_SIZE | int | cursor-cli를 동시에 실행할 최대 작업자 수 (동시성 제어) | 아니오 (기본값: 5\) |
| worker.queue\_size | WORKER\_QUEUE\_SIZE | int | 인-메모리 작업 큐의 최대 버퍼 크기 | 아니오 (기본값: 100\) |
| security.allowed\_response\_domains | SECURITY\_ALLOWED\_RESPONSE\_DOMAINS | string | (SSRF 방어) 지연 응답을 전송할 Slack 도메인 (쉼표로 구분) | 아니오 (기본값: hooks.slack.com) |

## **III. 핵심 서비스: 작업 디스패처 및 동시성 제어**

이 모듈은 os/exec 리소스 고갈을 방지하는 시스템의 핵심입니다. HTTP 요청과 실제 작업 실행을 분리합니다.7

### **A. internal/worker/job.go (작업 정의)**

Job 구조체는 Gin 핸들러에서 워커(Worker)로 전달되어야 하는 모든 정보를 캡슐화합니다.

Go

// package worker

import (  
    "time"  
    "github.com/your-org/cursor-slack-server/internal/server" // 핸들러의 페이로드 타입  
)

// Job은 cursor-cli 실행 작업을 정의합니다.  
type Job struct {  
    ID          string                    // 로깅 및 추적을 위한 고유 ID (예: UUID)  
    Payload     server.SlackCommandPayload // Slack에서 받은 원본 페이로드  
    ReceivedAt  time.Time                 // 요청 수신 시간 (큐 대기 시간 측정용)  
}

### **B. internal/worker/dispatcher.go (디스패처)**

Dispatcher는 모든 Worker를 시작하고, 외부(핸들러)로부터 JobQueue를 통해 작업을 받아 유휴 Worker에게 분배합니다.7

Go

// package worker

import (  
    "log"  
    "sync"  
)

// Dispatcher는 작업자 풀과 작업 큐를 관리합니다.  
type Dispatcher struct {  
    WorkerPool  chan chan Job // 작업자들의 작업 채널을 등록하는 풀 (작업자 풀)  
    JobQueue    chan Job      // 외부(핸들러)에서 작업을 받는 공용 큐  
    maxWorkers  int           // 작업자 풀의 크기  
    workers    \*Worker     // 실행 중인 작업자 인스턴스 (관리용)  
    wg          \*sync.WaitGroup  
    quit        chan struct{} // 디스패처 및 작업자 종료 신호  
}

// NewDispatcher는 디스패처를 생성하고 작업자 풀을 초기화합니다.  
func NewDispatcher(jobQueue chan Job, maxWorkers int) \*Dispatcher {  
    // 의 설계를 따라, WorkerPool은 작업자들의 '작업 채널'을 받는 채널입니다.  
    workerPool := make(chan chan Job, maxWorkers)

    return \&Dispatcher{  
        WorkerPool:  workerPool,  
        JobQueue:    jobQueue,  
        maxWorkers:  maxWorkers,  
        wg:          new(sync.WaitGroup),  
        quit:        make(chan struct{}),  
    }  
}

// Start는 디스패처 루프를 실행하고 작업자 풀을 가동합니다.  
func (d \*Dispatcher) Start(executor \*TaskExecutor) {  
    // 1\. 설정된 수(maxWorkers)만큼 작업자(Worker)를 생성하고 시작합니다.  
    for i := 0; i \< d.maxWorkers; i++ {  
        d.wg.Add(1) // WaitGroup 카운터 증가  
        worker := NewWorker(i+1, d.WorkerPool, d.wg, d.quit, executor)  
        worker.Start() // 각 작업자가 Goroutine에서 실행됨  
        d.workers \= append(d.workers, worker)  
    }  
      
    // 2\. 디스패치 루프를 별도의 Goroutine으로 실행합니다.  
    go d.dispatch()  
    log.Printf("%d개의 작업자(Worker)로 디스패처를 시작합니다.", d.maxWorkers)  
}

// dispatch는 JobQueue에서 작업을 가져와 WorkerPool의 유휴 작업자에게 전달합니다.  
func (d \*Dispatcher) dispatch() {  
    for {  
        select {  
        case job := \<-d.JobQueue: // 1\. 작업 큐에서 새 작업 수신  
            // 2\. 유휴 작업자의 작업 채널을 WorkerPool에서 가져옵니다.\[8\]  
            //    (유휴 작업자가 없으면 여기서 블록됩니다.)  
            workerJobChannel := \<-d.WorkerPool   
              
            // 3\. 해당 작업자에게 작업 전달  
            workerJobChannel \<- job  
              
        case \<-d.quit:  
            // 4\. 종료 신호 수신  
            return  
        }  
    }  
}

// Stop은 모든 작업자와 디스패처를 우아하게 종료합니다.\[23\]  
func (d \*Dispatcher) Stop() {  
    log.Println("디스패처 종료 신호 수신...")  
    close(d.quit) // 모든 작업자에게 종료 신호 전송  
    d.wg.Wait()   // 모든 작업자가 종료될 때까지 대기   
    log.Println("모든 작업자가 중지되었습니다.")  
}

### **C. internal/worker/worker.go (작업자)**

Worker는 실제 작업을 수행하는 행위자입니다. 자신의 작업 채널(WorkChannel)을 Dispatcher의 WorkerPool에 등록하여 작업 할당을 대기합니다.7

Go

// package worker

import (  
    "log"  
    "sync"  
)

// Worker는 실제 작업을 수행하는 행위자입니다.  
type Worker struct {  
    ID          int  
    WorkerPool  chan chan Job   // 디스패처의 작업자 풀  
    WorkChannel chan Job        // 이 작업자 개인의 작업 채널  
    wg          \*sync.WaitGroup  
    quit        chan struct{}  
    executor    \*TaskExecutor   // (의존성 주입) 실제 작업 실행기  
}

// NewWorker는 작업자를 생성합니다.  
func NewWorker(id int, pool chan chan Job, wg \*sync.WaitGroup, quit chan struct{}, exec \*TaskExecutor) \*Worker {  
    return \&Worker{  
        ID:          id,  
        WorkerPool:  pool,  
        WorkChannel: make(chan Job), // 각 작업자 고유의 채널  
        wg:          wg,  
        quit:        quit,  
        executor:    exec,  
    }  
}

// Start는 작업자의 메인 루프를 Goroutine으로 실행합니다.  
func (w \*Worker) Start() {  
    go func() {  
        defer w.wg.Done() // Goroutine 종료 시 WaitGroup 카운터 감소  
        log.Printf("작업자 \#%d 시작됨", w.ID)  
          
        for {  
            // 1\. 작업 준비 완료.  
            //    내 작업 채널을 디스패처의 WorkerPool에 등록하여 작업을 받을 준비가 되었음을 알림.\[8\]  
            w.WorkerPool \<- w.WorkChannel  
              
            select {  
            case job := \<-w.WorkChannel: // 2\. 디스패처로부터 작업 수신  
                log.Printf("작업자 \#%d: Job %s 처리 시작", w.ID, job.ID)  
                  
                // 3\. (중요) 실제 작업 실행  
                //    이 작업은 동기적으로 실행되며, 이 작업이 끝나야 다음 작업을 받습니다.  
                //    이것이 동시성을 'N'개로 제어하는 핵심입니다.  
                w.executor.Run(job)  
                  
                log.Printf("작업자 \#%d: Job %s 처리 완료", w.ID, job.ID)

            case \<-w.quit: // 4\. 종료 신호 수신  
                log.Printf("작업자 \#%d 종료 중...", w.ID)  
                return  
            }  
        }  
    }()  
}

## **IV. API 엔드포인트 및 요청 처리 (Gin)**

### **A. cmd/server/main.go (서버 진입점)**

main 패키지는 애플리케이션의 "Composition Root" 역할을 합니다. 모든 의존성을 생성(설정, 디스패처, 작업 큐) 및 주입하고, 우아한 종료(Graceful Shutdown) 로직을 처리합니다.

Go

// package main

import (  
    "context"  
    "log"  
    "net/http"  
    "os"  
    "os/signal"  
    "syscall"  
    "time"  
      
    "github.com/gin-gonic/gin"  
    "github.com/your-org/cursor-slack-server/config"  
    "github.com/your-org/cursor-slack-server/internal/server"  
    "github.com/your-org/cursor-slack-server/internal/worker"  
)

func main() {  
    // 1\. 설정 로드 (Viper) \[18, 24\]  
    cfg, err := config.LoadConfig()  
    if err\!= nil {  
        log.Fatalf("설정 로드 실패: %v", err)  
    }

    // 2\. 의존성 생성  
      
    // 2a. 작업 큐 생성 (버퍼 크기는 설정에서)  
    jobQueue := make(chan worker.Job, cfg.Worker.QueueSize)  
      
    // 2b. TaskExecutor (실제 작업 실행기) 생성  
    taskExecutor := worker.NewTaskExecutor(  
        cfg.Cursor.ProjectBasePath,  
        time.Duration(cfg.Cursor.ExecutionTimeoutSeconds)\*time.Second,  
        cfg.Security.AllowedResponseDomains,  
    )

    // 2c. Dispatcher 생성 및 시작 (작업자 풀 가동)   
    dispatcher := worker.NewDispatcher(jobQueue, cfg.Worker.PoolSize)  
    dispatcher.Start(taskExecutor) // 백그라운드에서 작업자들 실행 시작

    // 3\. Gin 라우터 설정 (의존성 주입)  
    // 핸들러가 JobQueue에 접근할 수 있도록 주입합니다.  
    router := server.SetupRouter(\&cfg, jobQueue)

    // 4\. Gin 서버 설정 및 시작 (별도 Goroutine)  
    srv := \&http.Server{  
        Addr:    ":" \+ cfg.Server.Port,  
        Handler: router,  
    }

    go func() {  
        if err := srv.ListenAndServe(); err\!= nil && err\!= http.ErrServerClosed {  
            log.Fatalf("Gin 서버 실행 실패: %s\\n", err)  
        }  
    }()

    // 5\. 우아한 종료(Graceful Shutdown) 처리 \[23\]  
    quitChan := make(chan os.Signal, 1)  
    // SIGINT (Ctrl+C) 또는 SIGTERM (Kubernetes 종료 신호) 대기  
    signal.Notify(quitChan, syscall.SIGINT, syscall.SIGTERM)  
      
    // 신호 대기 (블로킹)  
    \<-quitChan  
    log.Println("서버 종료 신호 수신...")

    // 5a. HTTP 서버 종료 (타임아웃 설정)  
    ctx, cancel := context.WithTimeout(context.Background(), 10\*time.Second)  
    defer cancel()  
    if err := srv.Shutdown(ctx); err\!= nil {  
        log.Fatal("HTTP 서버 종료 실패:", err)  
    }

    // 5b. Dispatcher 및 모든 Worker 종료  
    dispatcher.Stop()

    log.Println("서버가 우아하게 종료되었습니다.")  
}

### **B. internal/server/router.go (라우터 설정)**

라우터는 엔드포인트와 미들웨어를 정의합니다.

Go

// package server

import (  
    "github.com/gin-gonic/gin"  
    "github.com/your-org/cursor-slack-server/config"  
    "github.com/your-org/cursor-slack-server/internal/server/middleware"  
    "github.com/your-org/cursor-slack-server/internal/worker"  
)

// SetupRouter는 Gin 라우터와 미들웨어, 핸들러를 설정합니다.  
func SetupRouter(cfg \*config.Config, jobQueue chan\<- worker.Job) \*gin.Engine {  
    // gin.Default() 대신 New()를 사용하여 로깅, 복구 미들웨어를 명시적으로 제어  
    r := gin.New()  
    r.Use(gin.Logger())   // 표준 로깅 미들웨어  
    r.Use(gin.Recovery()) // 패닉 복구 미들웨어

    // API 엔드포인트 그룹화  
    slackApi := r.Group("/slack")  
    {  
        // (보안 1\) Slack 요청 인증 미들웨어 적용   
        // 이 미들웨어가 먼저 실행되어야 핸들러가 신뢰할 수 있는 요청만 처리합니다.  
        authMiddleware := middleware.SlackAuthMiddleware(cfg.Slack.SigningSecret)  
        slackApi.Use(authMiddleware)

        // (보안 2\) (선택적) DDOS 방지를 위한 속도 제한 미들웨어 \[25, 26\]  
        // 예: IP당 10초에 5개 요청으로 제한  
        // slackApi.Use(middleware.RateLimiterMiddleware(5, 10\*time.Second))  
          
        // 핸들러 바인딩  
        // jobQueue를 핸들러에 전달하기 위해 클로저(Closure) 사용  
        slackApi.POST("/cursor", HandleSlashCursor(jobQueue))  
    }  
      
    // (선택적) 상태 확인(Health Check) 엔드포인트  
    r.GET("/health", func(c \*gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

    return r  
}

### **C. internal/server/handlers.go (핸들러)**

핸들러의 책임은 초기 명세와 달리 대폭 축소되었습니다. os/exec를 직접 호출하는 대신, \*\*즉시 응답(ACK)\*\*과 **작업 큐에 제출**이라는 두 가지 임무만 수행합니다. 이로써 HTTP 요청 처리가 10ms 이내에 매우 빠르게 완료됩니다.

Go

// package server

import (  
    "net/http"  
    "time"  
      
    "github.com/gin-gonic/gin"  
    "github.com/google/uuid"  
    "github.com/your-org/cursor-slack-server/internal/server/middleware"  
    "github.com/your-org/cursor-slack-server/internal/worker"  
)

// SlackCommandPayload는 Slack이 보내는 폼 데이터를 바인딩합니다.  
// (form:...) 태그는 application/x-www-form-urlencoded 컨텐츠 타입에 필요합니다.  
type SlackCommandPayload struct {  
    Text        string \`form:"text"\`  
    UserName    string \`form:"user\_name"\`  
    UserID      string \`form:"user\_id"\`  
    ResponseURL string \`form:"response\_url"\`  
    TriggerID   string \`form:"trigger\_id"\`  
}

// HandleSlashCursor는 /cursor 명령어의 메인 핸들러입니다.  
// JobQueue 채널을 주입받기 위해 고차 함수(Higher-Order Function)로 구현합니다.  
func HandleSlashCursor(jobQueue chan\<- worker.Job) gin.HandlerFunc {  
    return func(c \*gin.Context) {  
        var payload SlackCommandPayload  
          
        // (중요) SlackAuthMiddleware가 Request.Body를 이미 읽었으므로,  
        // c.ShouldBind()는 미들웨어에서 복원된 Body를 읽습니다.  
        if err := c.ShouldBind(\&payload); err\!= nil {  
            // Slack은 오류 발생 시에도 200 OK와 함께 오류 메시지를 받는 것을 선호할 수 있습니다.  
            // 여기서는 400 Bad Request로 처리합니다.  
            c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})  
            return  
        }  
          
        // 1\. 즉시 응답 (ACK) \- 3초 룰 준수  
        // 요청자에게만 보이는 임시 메시지(ephemeral)를 전송합니다.  
        c.JSON(http.StatusOK, gin.H{  
            "response\_type": "ephemeral",   
            "text":          "⏳ " \+ payload.UserName \+ "님의 요청을 접수했습니다. 작업이 큐에 추가되었습니다.",  
        })

        // 2\. (비동기) Job 생성  
        // 미들웨어에서 생성/주입한 요청 ID가 있다면 사용합니다.  
        reqID, exists := c.Get(middleware.RequestIDKey)  
        if\!exists {  
            reqID \= uuid.NewString() // 없으면 새로 생성  
        }

        newJob := worker.Job{  
            ID:          reqID.(string),  
            Payload:     payload, // payload는 값으로 복사되어 Goroutine에 안전  
            ReceivedAt:  time.Now(),  
        }

        // 3\. (비동기) Dispatcher의 JobQueue로 작업 전송   
        // 이 작업은 Goroutine을 생성하지 않습니다.  
        // 만약 큐가 가득 차면(Backpressure), 핸들러는 여기서 잠시 대기하게 되며,  
        // 이는 시스템이 최대 부하에 도달했음을 의미하는 정상적인 동작입니다.  
        go func() {  
            // (참고) 핸들러가 HTTP 응답(c.JSON)을 반환하는 것을 블록하지 않기 위해  
            // 큐 전송을 별도 Goroutine에서 처리할 수 있습니다.  
            // 하지만 이 경우, 서버 종료 시 이 Goroutine이 유실될 수 있으므로  
            // 동기식 전송(jobQueue \<- newJob)이 더 간단하고 안전할 수 있습니다.  
            // 여기서는 비동기 전송을 선택하여 핸들러 응답 속도를 보장합니다.  
            jobQueue \<- newJob  
        }()  
    }  
}

## **V. 보안 계층 1: Slack 요청 인증 미들웨어**

### **A. internal/server/middleware/slack\_auth.go**

이 미들웨어는 서버로 들어오는 모든 요청이 실제로 Slack에서 보낸 것인지, 그리고 탈취된 오래된 요청은 아닌지 검증하는 **필수 보안 계층**입니다.

* **HMAC 서명 검증:** 10의 가이드에 따라 X-Slack-Signature 헤더와 요청 본문, Signing Secret을 사용하여 HMAC-SHA256 서명을 비교합니다.  
* **재전송 공격(Replay Attack) 방어:** 서명 검증만으로는 공격자가 유효한 요청을 가로채서 그대로 재전송(Replay)하는 것을 막을 수 없습니다.10 이를 방지하기 위해 X-Slack-Request-Timestamp 헤더를 읽어, 현재 서버 시간과 5분 이상 차이 나는 오래된 요청을 거부합니다.  
* **이중 본문 읽기(Double Body Read) 문제 해결:** HMAC 서명 계산을 위해 원본(raw) 요청 본문을 읽어야 합니다.10 하지만 Gin 핸들러 역시 c.ShouldBind를 통해 본문을 읽어야 합니다. c.Request.Body는 한 번만 읽을 수 있는 스트림입니다.27  
* **해결책:** 28의 모범 사례에 따라, 미들웨어에서 ioutil.ReadAll로 본문을 bodyBytes에 읽어들인 뒤, c.Request.Body를 ioutil.NopCloser(bytes.NewBuffer(bodyBytes))로 즉시 복원합니다. 이렇게 하면 미들웨어와 핸들러 모두가 본문을 안전하게 읽을 수 있습니다.

Go

// package middleware

import (  
    "bytes"  
    "crypto/hmac"  
    "crypto/sha256"  
    "encoding/hex"  
    "fmt"  
    "io/ioutil"  
    "log"  
    "net/http"  
    "strconv"  
    "time"

    "github.com/gin-gonic/gin"  
    "github.com/google/uuid"  
)

// RequestIDKey는 Gin Context에서 요청 ID를 식별하기 위한 키입니다.  
const RequestIDKey \= "requestID"

const maxTimestampAge \= 5 \* time.Minute // 5분

// SlackAuthMiddleware는 Slack 요청의 서명과 타임스탬프를 검증합니다.  
func SlackAuthMiddleware(signingSecret string) gin.HandlerFunc {  
    return func(c \*gin.Context) {  
        // (선택적) 모든 요청에 고유 ID 부여  
        c.Set(RequestIDKey, uuid.NewString())

        // 1\. 요청 본문 읽기 (이중 읽기 문제 해결)   
        bodyBytes, err := ioutil.ReadAll(c.Request.Body)  
        if err\!= nil {  
            log.Printf("\[%s\] Failed to read body: %v", c.GetString(RequestIDKey), err)  
            c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to read body"})  
            return  
        }  
        // (중요) 핸들러가 다시 읽을 수 있도록 본문 복원  
        c.Request.Body \= ioutil.NopCloser(bytes.NewBuffer(bodyBytes))

        // 2\. 타임스탬프 검증 (Replay Attack 방어)   
        timestampStr := c.GetHeader("X-Slack-Request-Timestamp")  
        timestampInt, err := strconv.ParseInt(timestampStr, 10, 64)  
        if err\!= nil {  
            c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid timestamp"})  
            return  
        }  
        timestamp := time.Unix(timestampInt, 0)  
          
        // 시간차가 5분을 초과하면 요청 거부  
        if time.Since(timestamp) \> maxTimestampAge {  
            log.Printf("\[%s\] Timestamp too old (Replay Attack?): %s", c.GetString(RequestIDKey), timestampStr)  
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Timestamp too old"})  
            return  
        }

        // 3\. HMAC 서명 검증   
        slackSignature := c.GetHeader("X-Slack-Signature")  
        if slackSignature \== "" {  
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Signature missing"})  
            return  
        }  
          
        // v0=\<timestamp\>:\<raw\_body\> 형태의 basestring 생성  
        baseString := fmt.Sprintf("v0:%s:%s", timestampStr, string(bodyBytes))

        // HMAC-SHA256 계산  
        h := hmac.New(sha256.New,byte(signingSecret))  
        h.Write(byte(baseString))  
        expectedSignature := "v0=" \+ hex.EncodeToString(h.Sum(nil))

        // 서명 비교 (Timing Attack 방지를 위해 hmac.Equal 사용)  
        if\!hmac.Equal(byte(slackSignature),byte(expectedSignature)) {  
            log.Printf("\[%s\] Signature mismatch", c.GetString(RequestIDKey))  
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Signature mismatch"})  
            return  
        }

        // 4\. 모든 검증 통과  
        c.Next()  
    }  
}

## **VI. 보안 계층 2: 비동기 작업 파이프라인 (Task Executor)**

TaskExecutor는 Worker로부터 Job을 받아, 실제 cursor-cli를 실행하는 모든 로직을 담당합니다. **모든 핵심 보안 검증(파싱, 경로, 실행, 응답)이 이 모듈에 집중됩니다.**

### **A. internal/worker/task\_executor.go (작업 실행기)**

Go

// package worker

import (  
    "bytes"  
    "context"  
    "encoding/json"  
    "fmt"  
    "log"  
    "net/http"  
    "net/url"  
    "os/exec"  
    "path/filepath"  
    "strings"  
    "syscall"  
    "time"  
      
    //... (필요한 import들)...  
)

// TaskExecutor는 실제 cursor-cli 작업을 실행하고 모든 보안 검증을 수행합니다.  
type TaskExecutor struct {  
    projectBasePath      string        // (보안) 작업이 실행될 루트 디렉토리  
    executionTimeout     time.Duration // (보안) 최대 실행 시간  
    allowedResponseDomainsstring    // (보안) SSRF 방어용 허용 도메인  
}

// NewTaskExecutor는 TaskExecutor의 인스턴스를 생성합니다.  
func NewTaskExecutor(basePath string, timeout time.Duration, allowedDomainsstring) \*TaskExecutor {  
    // (중요) BasePath를 절대 경로로 변환하여 저장  
    absPath, err := filepath.Abs(basePath)  
    if err\!= nil {  
        log.Fatalf("TaskExecutor: 유효하지 않은 project\_base\_path: %v", err)  
    }  
      
    return \&TaskExecutor{  
        projectBasePath:      absPath, // 항상 절대 경로 유지  
        executionTimeout:     timeout,  
        allowedResponseDomains: allowedDomains,  
    }  
}

// Run은 Job을 받아 (1)파싱 \-\> (2)검증 \-\> (3)실행 \-\> (4)응답의 전체 파이프라인을 수행합니다.  
func (te \*TaskExecutor) Run(job Job) {  
    payload := job.Payload  
    responseURL := payload.ResponseURL  
    jobID := job.ID

    // 1\. 커맨드 파싱 (Shlex & PFlag) \[30, 31\]  
    prompt, files, err := te.parseCursorCommand(payload.Text)  
    if err\!= nil {  
        errMsg := fmt.Sprintf("명령어 파싱 오류: %v", err)  
        log.Printf("\[%s\] %s", jobID, errMsg)  
        te.sendDelayedResponse(responseURL, errMsg)  
        return  
    }

    // 2\. 파일 경로 검증 (Path Traversal 방어)   
    validatedFiles, err := te.validateFilePaths(files)  
    if err\!= nil {  
        errMsg := fmt.Sprintf("보안 오류 (Path Traversal): %v", err)  
        log.Printf("\[%s\] %s", jobID, errMsg)  
        te.sendDelayedResponse(responseURL, errMsg)  
        return  
    }  
      
    // 3\. 명령어 실행 (Timeout 및 Process Group) \[11, 12\]  
    log.Printf("\[%s\] 작업자 실행 시작: prompt='%s', files=%v", jobID, prompt, validatedFiles)  
    output, err := te.executeCursorCommand(jobID, prompt, validatedFiles)  
      
    // 4\. 결과 포맷팅  
    resultMessage := string(output)  
    if err\!= nil {  
        log.Printf("\[%s\] 작업자 실행 오류: %v, output: %s", jobID, err, resultMessage)  
        resultMessage \= fmt.Sprintf("❌ Cursor AI 실행 중 에러 발생: %v\\n\\n%s", err, resultMessage)  
    } else {  
        log.Printf("\[%s\] 작업자 실행 완료.", jobID)  
        resultMessage \= fmt.Sprintf("✅ Cursor AI 작업 완료:\\n\\n%s", resultMessage)  
    }

    // 5\. 결과 전송 (SSRF 방어)   
    // Slack은 \`\`\` (백틱 3개)로 코드 블록을 인식합니다.  
    te.sendDelayedResponse(responseURL, "\`\`\`\\n"\+resultMessage+"\\n\`\`\`")  
}

### **B. 1단계: 강력한 커맨드 파싱 (Shlex & PFlag)**

초기 명세의 strings.SplitN과 strings.Fields는 \`"/cursor "버그 수정" \--files "src/main.go" "src/other.go"\`\`와 같이 인용부호(quotes)나 공백이 포함된 인자를 올바르게 처리하지 못합니다.

* **해결책:**  
  1. github.com/google/shlex 31를 사용하여 쉘(shell)과 동일한 규칙으로 입력 텍스트를 string 토큰으로 분리합니다.  
  2. github.com/spf13/pflag 30 (또는 표준 flag 34)을 사용하여 이 string에서 \--files 플래그와 나머지 인자(prompt)를 안전하게 추출합니다.

Go

// package worker

import (  
    "\[github.com/google/shlex\](https://github.com/google/shlex)" //   
    "\[github.com/spf13/pflag\](https://github.com/spf13/pflag)"   //   
)

// parseCursorCommand는 Slack 텍스트를 파싱하여 prompt와 file 리스트로 분리합니다.  
func (te \*TaskExecutor) parseCursorCommand(text string) (prompt string, filesstring, err error) {  
    // 1\. shlex를 사용한 쉘-방식 분리 \[31\]  
    // 예: \`"fix bug" \--files "f1.go" "f2.go"\` \-\> \`\["fix bug", "--files", "f1.go", "f2.go"\]\`  
    args, err := shlex.Split(text)  
    if err\!= nil {  
        return "", nil, fmt.Errorf("shlex 파싱 실패 (인용부호가 올바른지 확인하세요): %w", err)  
    }

    if len(args) \== 0 {  
        return "", nil, fmt.Errorf("입력된 명령어가 없습니다")  
    }

    // 2\. pflag.FlagSet을 사용한 안전한 플래그 파싱 \[30, 34\]  
    // FlagSet은 프로그램(os.Exit)을 종료하지 않도록 ContinueOnError로 설정합니다.  
    fs := pflag.NewFlagSet("cursor", pflag.ContinueOnError)  
    var filePathsstring  
      
    // \--files 플래그 정의.  
    fs.StringSliceVar(\&filePaths, "files",string{}, "작업에 포함할 파일 경로")

    // (중요) pflag가 알 수 없는 플래그를 무시하도록 설정 (prompt에 \--가 있을 수 있음)  
    fs.ParseErrorsWhitelist.UnknownFlags \= true   
      
    // pflag 파싱 실행  
    if err := fs.Parse(args); err\!= nil {  
        return "", nil, fmt.Errorf("플래그 파싱 실패: %w", err)  
    }

    // 3\. 플래그가 아닌 나머지 인자(NArgs)를 'prompt'로 결합  
    // fs.Args()는 플래그로 처리되지 않은 모든 인자를 반환합니다.  
    prompt \= strings.Join(fs.Args(), " ")  
    if prompt \== "" {  
         return "", nil, fmt.Errorf("AI 프롬프트가 비어있습니다 (예: /cursor \\"버그 수정\\" \--files...)")  
    }

    return prompt, filePaths, nil  
}

### **C. 2단계: 경로 조작(Path Traversal) 방어**

* **위협:** 사용자가 \--files "../../../etc/passwd" 또는 \--files "/etc/passwd"와 같은 악의적인 입력을 시도하여, 허용된 projectBasePath 외부의 민감한 파일을 읽거나 쓰도록 유도할 수 있습니다.  
* **해결책:** filepath.Clean 35만으로는 절대 경로('/' 시작)나 ..를 완전히 방어할 수 없습니다. 13의 모범 사례에 따라, filepath.Abs와 filepath.Rel을 조합하여 요청된 파일의 최종 절대 경로가 projectBasePath의 하위 경로임을 수학적으로 검증합니다.

Go

// package worker

// validateFilePaths는 모든 파일 경로가 te.projectBasePath 내에 있는지 검증합니다.  
// 의 원칙을 따릅니다.  
func (te \*TaskExecutor) validateFilePaths(filesstring) (string, error) {  
    var validatedPathsstring  
      
    // te.projectBasePath는 NewTaskExecutor에서 이미 절대 경로로 보장됨.  
    basePathAbs := te.projectBasePath

    for \_, file := range files {  
        // 1\. 요청된 파일 경로를 정리합니다.  
        cleanedFile := filepath.Clean(file)  
          
        // 2\. BasePath와 결합된 절대 경로를 계산합니다.  
        // (중요) Join을 사용하여 BasePath 내의 경로를 만듭니다.  
        joinedPath := filepath.Join(basePathAbs, cleanedFile)  
          
        // 3\. 결합된 경로의 최종 절대 경로를 다시 계산합니다 (Symlink 등 해석).\[32\]  
        // Abs는 Clean을 내부적으로 호출합니다.  
        joinedPathAbs, err := filepath.Abs(joinedPath)  
        if err\!= nil {  
            return nil, fmt.Errorf("유효하지 않은 파일 경로: %s", file)  
        }  
          
        // 4\. (보안 핵심) Rel을 사용한 상위 디렉토리 검증 \[13\]  
        // 최종 경로(joinedPathAbs)가 BasePath(basePathAbs)의 하위 경로인지 확인합니다.  
        relPath, err := filepath.Rel(basePathAbs, joinedPathAbs)  
        if err\!= nil {  
             // Rel 계산이 실패하면 (예: 윈도우/리눅스 드라이브가 다름) 거부  
             return nil, fmt.Errorf("경로 검증 실패 (Rel): %s", file)  
        }

        // 5\. 상대 경로가 '..'로 시작하거나 '..'인 경우,  
        //    이는 basePathAbs의 상위 디렉토리를 의미하므로 거부합니다.  
        if strings.HasPrefix(relPath, "..") |

| relPath \== ".." {  
            return nil, fmt.Errorf("허용되지 않은 경로 접근 시도: %s", file)  
        }  
          
        // (보안) Symlink 검사 (선택 사항이지만 강력히 권장됨) \[36\]  
        //... (os.Lstat으로 심볼릭 링크 여부 확인 로직)...

        // 검증 통과. BasePath 기준의 상대 경로(cleanedFile)를 반환합니다.  
        validatedPaths \= append(validatedPaths, cleanedFile)  
    }  
    return validatedPaths, nil  
}

### **D. 3단계: 안전한 프로세스 실행 (타임아웃 및 격리)**

* **위협 1 (무한 실행):** cursor-cli가 응답 없는 상태(Hang)가 되면 해당 Worker Goroutine이 영구적으로 중단됩니다.  
* **위협 2 (좀비 프로세스):** exec.CommandContext 11는 cursor-cli 프로세스 *자체*는 타임아웃 시 종료시키지만, cursor-cli가 실행한 *자식 프로세스*(예: git, node 등)는 종료시키지 못할 수 있습니다.12 이 자식 프로세스들이 '좀비 프로세스'로 남아 서버 리소스를 계속 소모할 수 있습니다.  
* **해결책:**  
  1. context.WithTimeout 11을 사용하여 최대 실행 시간을 강제합니다.  
  2. syscall.SysProcAttr{Setpgid: true} 12를 설정하여, cursor-cli와 그 모든 자식 프로세스를 \*새로운 프로세스 그룹(Process Group)\*에 배치합니다.  
  3. 타임아웃 발생 시, syscall.Kill(-cmd.Process.Pid,...) 12를 호출하여 PID 대신 **프로세스 그룹 ID 전체**에 종료 신호(SIGKILL)를 보내 모든 관련 프로세스를 한 번에 종료시킵니다.

Go

// package worker

// executeCursorCommand는 context.WithTimeout과 process group kill을 사용하여  
// cursor-cli를 안전하게 실행합니다. \[11, 12\]  
func (te \*TaskExecutor) executeCursorCommand(jobID string, prompt string, filesstring) (byte, error) {  
    // 1\. 타임아웃 컨텍스트 생성   
    ctx, cancel := context.WithTimeout(context.Background(), te.executionTimeout)  
    defer cancel()

    // 2\. 명령어 인자 생성  
    args :=string{prompt}  
    if len(files) \> 0 {  
        args \= append(args, "--files")  
        args \= append(args, files...) // 검증된 파일 경로들  
    }

    cmd := exec.CommandContext(ctx, "cursor", args...)  
      
    // 3\. (보안) 작업 디렉토리 격리 \[38\]  
    // 모든 파일 경로는 이 디렉토리 기준의 상대 경로여야 합니다.  
    cmd.Dir \= te.projectBasePath

    // 4\. (보안 핵심) 자식 프로세스까지 함께 종료하기 위해 Process Group 설정   
    // (참고: 이 설정은 Unix/Linux/macOS에서만 동작합니다. Windows는 별도 처리 필요)  
    cmd.SysProcAttr \= \&syscall.SysProcAttr{Setpgid: true}

    log.Printf("\[%s\] Executing command: cursor %s (in %s)", jobID, strings.Join(args, " "), cmd.Dir)

    // 5\. 실행 및 결과 수집 (stdout \+ stderr)  
    var outb, errb bytes.Buffer  
    cmd.Stdout \= \&outb  
    cmd.Stderr \= \&errb  
      
    err := cmd.Start()  
    if err\!= nil {  
        return nil, fmt.Errorf("명령어 시작 실패: %w", err)  
    }

    err \= cmd.Wait()

    // 6\. 에러 처리 (타임아웃 확인)  
    combinedOutput := append(outb.Bytes(), errb.Bytes()...)

    if ctx.Err() \== context.DeadlineExceeded {  
        log.Printf("\[%s\] 작업 시간 초과 (%v). 프로세스 그룹 강제 종료 시도...", jobID, te.executionTimeout)  
        // (보안 핵심) 의 로직에 따라 프로세스 그룹을 강제 종료  
        if cmd.Process\!= nil {  
             // 음수 PID는 프로세스 그룹 ID를 의미합니다.  
             // SIGKILL로 프로세스 그룹(-pid) 전체를 종료시킵니다.  
             \_ \= syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)  
        }  
        return combinedOutput, fmt.Errorf("명령어 실행 시간 초과 (%v)", te.executionTimeout)  
    }  
      
    if err\!= nil {  
        // 타임아웃 외의 다른 실행 오류  
        return combinedOutput, fmt.Errorf("cursor-cli 실행 실패: %w", err)  
    }

    return combinedOutput, nil  
}

### **E. 4단계: SSRF 방어 응답 전송**

* **위협 (Server-Side Request Forgery, SSRF):** payload.ResponseURL은 사용자가 제공한 입력입니다. 만약 인증 미들웨어를 우회한 공격자가 이 URL을 http://169.254.169.254/latest/meta-data (AWS 메타데이터) 39 또는 http://internal-redis:6379 (내부 서비스)로 조작하면, 서버는 신뢰된 내부 네트워크에서 해당 주소로 POST 요청을 보내게 됩니다.40 이는 치명적인 SSRF 취약점입니다.  
* **해결책:** 14에서 권장하는 대로, http.Post를 호출하기 전에 responseURL의 도메인을 추출하여, 설정 파일(Config)에 정의된 **허용 목록(Allow-list)**(예: hooks.slack.com)과 일치하는지 엄격하게 검증합니다.

Go

// package worker

// SlackDelayedResponse는 Slack 지연 응답용 JSON 구조체입니다.  
type SlackDelayedResponse struct {  
    Text         string \`json:"text"\`  
    ResponseType string \`json:"response\_type"\` // "in\_channel" 또는 "ephemeral"  
}

// sendDelayedResponse는 SSRF 공격을 방지하기 위해 ResponseURL을 검증한 후 전송합니다.  
// 의 원칙을 따릅니다.  
func (te \*TaskExecutor) sendDelayedResponse(responseURL string, message string) {  
    // 1\. (보안 핵심) SSRF 방어를 위한 URL 검증   
    parsedURL, err := url.Parse(responseURL)  
    if err\!= nil {  
        log.Printf("SSRF 방어: 유효하지 않은 ResponseURL (파싱 실패): %s", responseURL)  
        return  
    }

    // 2\. 스킴(Scheme) 검증  
    if parsedURL.Scheme\!= "https" {  
        log.Printf("SSRF 방어: 'https'가 아닌 스킴 차단: %s", parsedURL.Scheme)  
        return  
    }

    // 3\. 허용 목록(Allow-list) 기반 도메인 검증 \[41\]  
    isAllowed := false  
    for \_, allowedDomain := range te.allowedResponseDomains {  
        // (예: "hooks.slack.com")  
        if parsedURL.Hostname() \== allowedDomain |

| strings.HasSuffix(parsedURL.Hostname(), "."\+allowedDomain) {  
            isAllowed \= true  
            break  
        }  
    }

    if\!isAllowed {  
        log.Printf("SSRF 방어: 허용되지 않는 도메인으로의 응답 시도 차단: %s", responseURL)  
        return  
    }  
      
    // 4\. Slack 응답 페이로드 생성  
    // (결과가 너무 클 경우 Slack의 메시지 제한(예: 4000자)에 맞춰 잘라내야 할 수 있음)  
    payload := SlackDelayedResponse{  
        Text:         message,  
        ResponseType: "in\_channel", // 또는 "ephemeral"  
    }  
      
    jsonPayload, err := json.Marshal(payload)  
    if err\!= nil {  
        log.Printf("Error marshaling delayed response: %v", err)  
        return  
    }

    // 5\. (안전) 검증된 도메인으로 결과 전송  
    resp, err := http.Post(responseURL, "application/json", bytes.NewBuffer(jsonPayload))  
    if err\!= nil {  
        log.Printf("Error sending delayed response to %s: %v", responseURL, err)  
        return  
    }  
    defer resp.Body.Close()  
      
    if resp.StatusCode\!= http.StatusOK {  
         log.Printf("Slack delayed response returned non-200 status: %d", resp.StatusCode)  
         // (필요시) Slack의 오류 응답 본문 로깅  
    }  
}

## **VII. 고도화된 프로젝트 구조 및 의존성**

### **A. 제안하는 디렉토리 구조 (Standard Go Project Layout)**

초기 명세의 플랫(flat) 구조(모든 파일을 main 패키지에 위치)는 소규모 프로젝트에는 적합하지만, 의존성 관리, 모듈화, 테스트 용이성 측면에서 한계가 있습니다. 본 문서는 표준 Go 프로젝트 레이아웃을 채택하여 관심사(Concerns)를 명확히 분리합니다.

/cursor-slack-server  
├── /cmd  
│   └── /server  
│       └── main.go         \# (서버 진입점, 의존성 주입, S30 우아한 종료)  
├── /config  
│   ├── config.go           \# (Viper 설정 로더, Config 구조체, S94, S95)  
│   └── config.example.yaml \# (Viper 설정 예시 파일)  
├── /internal  
│   ├── /server             \# (Gin 서버 관련 로직: HTTP 계층)  
│   │   ├── handlers.go     \# (HandleSlashCursor, S16 JobQueue 주입)  
│   │   ├── router.go       \# (Gin 라우터 설정)  
│   │   └── /middleware     \# (미들웨어 패키지)  
│   │       ├── slack\_auth.go \# (HMAC, Timestamp 검증, S59, S66)  
│   │       └── (rate\_limiter.go) \# (선택적 속도 제한, S51)  
│   └── /worker             \# (비동기 작업자 풀 관련 로직: 비즈니스 계층)  
│       ├── dispatcher.go   \# (디스패처, S16)  
│       ├── job.go          \# (Job 구조체 정의)  
│       ├── task\_executor.go \# (실제 작업 실행기: Parse-\>Validate-\>Exec-\>Respond)  
│       └── worker.go       \# (작업자, S16)  
├── go.mod                  \# (프로젝트 의존성)  
├── go.sum  
└── README.md               \# (Table 1 설정 파라미터 포함)

### **B. go.mod 필수 의존성**

go.mod 파일은 프로젝트가 사용하는 라이브러리를 정의합니다.

코드 스니펫

module \[github.com/your-org/cursor-slack-server\](https://github.com/your-org/cursor-slack-server)

go 1.21

require (  
    \[github.com/gin-gonic/gin\](https://github.com/gin-gonic/gin) v1.9.1           // 웹 프레임워크  
    \[github.com/google/shlex\](https://github.com/google/shlex) v0.0.0-20191202100458-e7afc7fbc510 // 쉘-방식 파서 \[31\]  
    \[github.com/google/uuid\](https://github.com/google/uuid) v1.6.0             // 로깅을 위한 고유 ID 생성  
    \[github.com/spf13/pflag\](https://github.com/spf13/pflag) v1.0.5             // 플래그 파서   
    \[github.com/spf13/viper\](https://github.com/spf13/viper) v1.18.2            // 설정 관리 \[17, 24\]

    // Viper의 간접 의존성들  
    // (fsnotify, toml, yaml 등)  
)

## **VIII. 요약: 위협 모델 및 완화 전략**

### **A. 표 2: 위협 모델 및 고도화된 완화 전략**

이 표는 초기 기술 명세에 내재된 잠재적 위협과, 본 고도화된 설계 문서가 이를 어떻게 완화하는지 요약합니다.

| 위협 벡터 | 초기 명세의 취약점 | 고도화된 완화 전략 (본 설계) | 관련 섹션 / 근거 |
| :---- | :---- | :---- | :---- |
| **리소스 고갈 (DoS)** | go runCursorTask로 요청마다 Goroutine/스레드 무한 생성 | **Worker Pool \+ Job Queue 아키텍처** 채택. 동시 실행 os/exec 수를 N개(예: 5개)로 엄격히 제한. \[4, 7\] | III, IV-C |
| **무한 실행 (프로세스 행)** | os/exec에 타임아웃이 없음. cursor-cli가 멈추면 Goroutine 영구 중단. | context.WithTimeout을 사용한 exec.CommandContext 강제. 11 | VI-D |
| **좀비 프로세스 (리소스 누수)** | context.WithTimeout이 자식 프로세스를 종료시키지 못할 수 있음. 12 | syscall.Setpgid로 프로세스 그룹을 생성하고, 타임아웃 시 syscall.Kill(-pid)로 **그룹 전체를** 종료. 12 | VI-D |
| **경로 조작 (Path Traversal)** | TODO로 남겨짐. filepath.Clean만으로는 불충분. 35 | filepath.Abs와 filepath.Rel을 조합하여 요청 경로가 projectBasePath를 벗어나는지("..") 원천 확인. \[13\] | VI-C |
| **서버 측 요청 위조 (SSRF)** | response\_url을 검증 없이 http.Post로 전송. \[39, 40\] | url.Parse를 통해 response\_url의 도메인을 추출하고, hooks.slack.com과 같은 \*\*허용 목록(Allow-list)\*\*과 일치하는지 확인. 14 | VI-E |
| **재전송 공격 (Replay Attack)** | Slack 서명 검증만 언급됨 (타임스탬프 누락). 10 | X-Slack-Request-Timestamp를 확인하여 5분 이상 지난 오래된 요청은 거부. 10 | V-A |
| **인증 우회 (Body Read)** | 미들웨어와 핸들러의 이중 Body 읽기 문제\[28\]로 인한 잠재적 인증 실패. | ioutil.ReadAll로 본문을 읽고, bytes.NewBuffer로 c.Request.Body를 복원하여 두 계층 모두에서 본문 접근 보장. 28 | V-A |
| **명령어 파싱 오류** | strings.SplitN 사용. 인용부호(")가 포함된 프롬프트나 파일명 처리 불가. | google/shlex \[31\]로 쉘-방식 파싱 후, pflag 30로 플래그와 인자를 안전하게 분리. | VI-B |
| **설정 하드코딩 (유지보수)** | projectBasePath 등 설정이 하드코딩될 가능성. \[15\] | Viper \[24\]를 도입하여 파일/환경 변수를 통한 계층적 설정 관리. SLACK\_SIGNING\_SECRET는 환경 변수로만 주입. \[16, 17\] | II |

### **B. 향후 확장성 권고**

현재 아키텍처는 인-메모리(in-memory) chan Job을 작업 큐로 사용합니다. 이 방식은 구현이 간단하지만, 서버가 재시작될 경우 큐에 대기 중이던 모든 작업이 유실되는 단점이 있습니다.

시스템의 중요도가 높아지고 작업 유실을 방지해야 할 경우, 다음 단계는 이 인-메모리 큐를 Redis와 같은 외부 브로커를 사용하는 **영속성 있는(Persistent) 작업 큐**로 교체하는 것입니다. Asynq 42 또는 gocraft/work 44와 같은 Go 라이브러리를 활용할 수 있습니다.

본 보고서에서 제안된 Dispatcher와 Worker 아키텍처는 이러한 외부 큐 시스템과 쉽게 통합되도록 추상화되어 있습니다. Dispatcher의 JobQueue 수신 로직을 Redis 큐에서 Dequeue하는 로직으로 변경하기만 하면 됩니다.

#### **참고 자료**

1. Go memory leak when doing concurrent os/exec.Command.Wait() \- Stack Overflow, 11월 6, 2025에 액세스, [https://stackoverflow.com/questions/34346064/go-memory-leak-when-doing-concurrent-os-exec-command-wait](https://stackoverflow.com/questions/34346064/go-memory-leak-when-doing-concurrent-os-exec-command-wait)  
2. Goroutine Worker Pools \- Go Optimization Guide, 11월 6, 2025에 액세스, [https://goperf.dev/01-common-patterns/worker-pool/](https://goperf.dev/01-common-patterns/worker-pool/)  
3. How Many Goroutines Can Go Run? A Deep Dive into Resource Limits | Leapcell, 11월 6, 2025에 액세스, [https://leapcell.io/blog/how-many-goroutines-can-go-run](https://leapcell.io/blog/how-many-goroutines-can-go-run)  
4. Scaling Go Services with Worker Pools: Lessons from Shopify and Beyond | Praella, 11월 6, 2025에 액세스, [https://praella.com/blogs/shopify-news/scaling-go-services-with-worker-pools-lessons-from-shopify-and-beyond](https://praella.com/blogs/shopify-news/scaling-go-services-with-worker-pools-lessons-from-shopify-and-beyond)  
5. Building a Worker Pool in Go for Better Concurrency | by Siddharth Narayan | Medium, 11월 6, 2025에 액세스, [https://medium.com/@siddharthnarayan/building-a-worker-pool-in-go-for-better-concurrency-3e3499dc35a7](https://medium.com/@siddharthnarayan/building-a-worker-pool-in-go-for-better-concurrency-3e3499dc35a7)  
6. Approaches to manage concurrent workloads, like worker pools and pipelines, 11월 6, 2025에 액세스, [https://dev.to/dwarvesf/approaches-to-manage-concurrent-workloads-like-worker-pools-and-pipelines-52ed](https://dev.to/dwarvesf/approaches-to-manage-concurrent-workloads-like-worker-pools-and-pipelines-52ed)  
7. Building a High-Performance Worker Queue in Golang (Go) \- Djamware, 11월 6, 2025에 액세스, [https://www.djamware.com/post/686cb3281869621ca18c495f/building-a-highperformance-worker-queue-in-golang-go](https://www.djamware.com/post/686cb3281869621ca18c495f/building-a-highperformance-worker-queue-in-golang-go)  
8. Writing worker queues, in Go \- Riding the wave., 11월 6, 2025에 액세스, [https://nesv.github.io/golang/2014/02/25/worker-queues-in-go.html](https://nesv.github.io/golang/2014/02/25/worker-queues-in-go.html)  
9. How would you define a pool of goroutines to be executed at once? \- Stack Overflow, 11월 6, 2025에 액세스, [https://stackoverflow.com/questions/18405023/how-would-you-define-a-pool-of-goroutines-to-be-executed-at-once](https://stackoverflow.com/questions/18405023/how-would-you-define-a-pool-of-goroutines-to-be-executed-at-once)  
10. Verifying requests from Slack | Slack Developer Docs, 11월 6, 2025에 액세스, [https://docs.slack.dev/authentication/verifying-requests-from-slack](https://docs.slack.dev/authentication/verifying-requests-from-slack)  
11. os/exec CommandContext example \- GitHub Gist, 11월 6, 2025에 액세스, [https://gist.github.com/udondan/00c3b48eada0a38349643f584a85ee38](https://gist.github.com/udondan/00c3b48eada0a38349643f584a85ee38)  
12. Golang context.WithTimeout doesn't work with exec.CommandContext "su \-c" command, 11월 6, 2025에 액세스, [https://stackoverflow.com/questions/67750520/golang-context-withtimeout-doesnt-work-with-exec-commandcontext-su-c-command](https://stackoverflow.com/questions/67750520/golang-context-withtimeout-doesnt-work-with-exec-commandcontext-su-c-command)  
13. check if given path is a subdirectory of another in golang \- Stack Overflow, 11월 6, 2025에 액세스, [https://stackoverflow.com/questions/28024731/check-if-given-path-is-a-subdirectory-of-another-in-golang](https://stackoverflow.com/questions/28024731/check-if-given-path-is-a-subdirectory-of-another-in-golang)  
14. Server Side Request Forgery Prevention \- OWASP Cheat Sheet Series, 11월 6, 2025에 액세스, [https://cheatsheetseries.owasp.org/cheatsheets/Server\_Side\_Request\_Forgery\_Prevention\_Cheat\_Sheet.html](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html)  
15. OWASP Golang Security Best Practices | by Yogesh Nishad \- Medium, 11월 6, 2025에 액세스, [https://rabson.medium.com/owasp-golang-security-best-practices-7defaaba8a55](https://rabson.medium.com/owasp-golang-security-best-practices-7defaaba8a55)  
16. passwords, secrets, keys \- best practice : r/golang \- Reddit, 11월 6, 2025에 액세스, [https://www.reddit.com/r/golang/comments/szhapw/passwords\_secrets\_keys\_best\_practice/](https://www.reddit.com/r/golang/comments/szhapw/passwords_secrets_keys_best_practice/)  
17. spf13/viper: Go configuration with fangs \- GitHub, 11월 6, 2025에 액세스, [https://github.com/spf13/viper](https://github.com/spf13/viper)  
18. A Guide to Configuration Management in Go with Viper \- DEV Community, 11월 6, 2025에 액세스, [https://dev.to/kittipat1413/a-guide-to-configuration-management-in-go-with-viper-5271](https://dev.to/kittipat1413/a-guide-to-configuration-management-in-go-with-viper-5271)  
19. Request body disappear if we access it on middleware · Issue \#1651 · gin-gonic/gin \- GitHub, 11월 6, 2025에 액세스, [https://github.com/gin-gonic/gin/issues/1651](https://github.com/gin-gonic/gin/issues/1651)  
20. How to read request body twice in Golang middleware? \- Stack Overflow, 11월 6, 2025에 액세스, [https://stackoverflow.com/questions/46948050/how-to-read-request-body-twice-in-golang-middleware](https://stackoverflow.com/questions/46948050/how-to-read-request-body-twice-in-golang-middleware)  
21. How to Build a Multi-Workspace Slack Application in Go \- BlinkOps, 11월 6, 2025에 액세스, [https://www.blinkops.com/blog/how-to-build-a-multi-workspace-slack-application-in-go](https://www.blinkops.com/blog/how-to-build-a-multi-workspace-slack-application-in-go)  
22. pflag package \- github.com/spf13/pflag \- Go Packages, 11월 6, 2025에 액세스, [https://pkg.go.dev/github.com/spf13/pflag](https://pkg.go.dev/github.com/spf13/pflag)  
23. shlex package \- github.com/google/shlex \- Go Packages, 11월 6, 2025에 액세스, [https://pkg.go.dev/github.com/google/shlex](https://pkg.go.dev/github.com/google/shlex)  
24. Easiest fix to file Path traversal attacks: Secure coding methodology | by Ronnie Joseph | Bug Bounty Hunting | Medium, 11월 6, 2025에 액세스, [https://medium.com/bug-bounty-hunting/easiest-fix-to-file-path-traversal-attacks-secure-coding-methodology-75bb03ae8674](https://medium.com/bug-bounty-hunting/easiest-fix-to-file-path-traversal-attacks-secure-coding-methodology-75bb03ae8674)  
25. python's shlex.split alternative for Go \- Stack Overflow, 11월 6, 2025에 액세스, [https://stackoverflow.com/questions/36958515/pythons-shlex-split-alternative-for-go](https://stackoverflow.com/questions/36958515/pythons-shlex-split-alternative-for-go)  
26. Command-Line Arguments in Go: How to Use the Flag Library | by Leapcell | Medium, 11월 6, 2025에 액세스, [https://leapcell.medium.com/command-line-arguments-in-go-how-to-use-the-flag-library-0975f9319c6a](https://leapcell.medium.com/command-line-arguments-in-go-how-to-use-the-flag-library-0975f9319c6a)  
27. Preventing Path Traversal in Golang \- StackHawk, 11월 6, 2025에 액세스, [https://www.stackhawk.com/blog/golang-path-traversal-guide-examples-and-prevention/](https://www.stackhawk.com/blog/golang-path-traversal-guide-examples-and-prevention/)  
28. Running a command with a timeout in Go \- jarv.org, 11월 6, 2025에 액세스, [https://jarv.org/posts/command-with-timeout/](https://jarv.org/posts/command-with-timeout/)  
29. Preventing server-side request forgery in Node.js applications \- Snyk, 11월 6, 2025에 액세스, [https://snyk.io/blog/preventing-server-side-request-forgery-node-js/](https://snyk.io/blog/preventing-server-side-request-forgery-node-js/)  
30. Server-Side Request Forgery: What It Is & How To Fix It | Wiz, 11월 6, 2025에 액세스, [https://www.wiz.io/academy/server-side-request-forgery](https://www.wiz.io/academy/server-side-request-forgery)  
31. Defending Against SSRF: Understanding, Detecting, and Mitigating Server-Side Request Forgery Vulnerabilities in Java | by Ajay Monga | Medium, 11월 6, 2025에 액세스, [https://medium.com/@ajay.monga73/defending-against-ssrf-understanding-detecting-and-mitigating-server-side-request-forgery-f2d1fd62413d](https://medium.com/@ajay.monga73/defending-against-ssrf-understanding-detecting-and-mitigating-server-side-request-forgery-f2d1fd62413d)  
32. hibiken/asynq: Simple, reliable, and efficient distributed task queue in Go \- GitHub, 11월 6, 2025에 액세스, [https://github.com/hibiken/asynq](https://github.com/hibiken/asynq)  
33. Task Queues in Go: Asynq vs Machinery vs Work: Powering Background Jobs in High-Throughput Systems | by Geison | Medium, 11월 6, 2025에 액세스, [https://medium.com/@geisonfgfg/task-queues-in-go-asynq-vs-machinery-vs-work-powering-background-jobs-in-high-throughput-systems-45066a207aa7](https://medium.com/@geisonfgfg/task-queues-in-go-asynq-vs-machinery-vs-work-powering-background-jobs-in-high-throughput-systems-45066a207aa7)  
34. Background Jobs in GoLang — Your Ultimate Guide to Empower Your Applications | by Sunny Yadav | Simform Engineering | Medium, 11월 6, 2025에 액세스, [https://medium.com/simform-engineering/background-jobs-in-golang-your-ultimate-guide-to-empower-your-applications-e1a2db941d82](https://medium.com/simform-engineering/background-jobs-in-golang-your-ultimate-guide-to-empower-your-applications-e1a2db941d82)