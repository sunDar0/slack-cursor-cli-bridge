package worker

import (
	"log"
	"sync"
)

// Worker는 실제 작업을 수행하는 행위자입니다.
// 자신의 작업 채널(WorkChannel)을 Dispatcher의 WorkerPool에 등록하여 작업 할당을 대기합니다.
type Worker struct {
	ID          int
	WorkerPool  chan chan Job   // 디스패처의 작업자 풀
	WorkChannel chan Job        // 이 작업자 개인의 작업 채널
	wg          *sync.WaitGroup
	quit        chan struct{}
	executor    *TaskExecutor   // (의존성 주입) 실제 작업 실행기
}

// NewWorker는 작업자를 생성합니다.
func NewWorker(id int, pool chan chan Job, wg *sync.WaitGroup, quit chan struct{}, exec *TaskExecutor) *Worker {
	return &Worker{
		ID:          id,
		WorkerPool:  pool,
		WorkChannel: make(chan Job),
		wg:          wg,
		quit:        quit,
		executor:    exec,
	}
}

// Start는 작업자의 메인 루프를 Goroutine으로 실행합니다.
func (w *Worker) Start() {
	go func() {
		defer w.wg.Done()
		log.Printf("작업자 #%d 시작됨", w.ID)

		for {
			// 1. 작업 준비 완료.
			//    내 작업 채널을 디스패처의 WorkerPool에 등록하여 작업을 받을 준비가 되었음을 알림.
			w.WorkerPool <- w.WorkChannel

			select {
			case job := <-w.WorkChannel: // 2. 디스패처로부터 작업 수신
				log.Printf("작업자 #%d: Job %s 처리 시작", w.ID, job.ID)

				// 3. (중요) 실제 작업 실행
				//    이 작업은 동기적으로 실행되며, 이 작업이 끝나야 다음 작업을 받습니다.
				//    이것이 동시성을 'N'개로 제어하는 핵심입니다.
				w.executor.Run(job)

				log.Printf("작업자 #%d: Job %s 처리 완료", w.ID, job.ID)

			case <-w.quit: // 4. 종료 신호 수신
				log.Printf("작업자 #%d 종료 중...", w.ID)
				return
			}
		}
	}()
}
