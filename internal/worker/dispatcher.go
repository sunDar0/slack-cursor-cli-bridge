package worker

import (
	"log"
	"sync"
)

// Dispatcher는 작업자 풀과 작업 큐를 관리합니다.
type Dispatcher struct {
	WorkerPool  chan chan Job // 작업자들의 작업 채널을 등록하는 풀 (작업자 풀)
	JobQueue    chan Job      // 외부(핸들러)에서 작업을 받는 공용 큐
	maxWorkers  int           // 작업자 풀의 크기
	workers     []*Worker     // 실행 중인 작업자 인스턴스 (관리용)
	wg          *sync.WaitGroup
	quit        chan struct{} // 디스패처 및 작업자 종료 신호
}

// NewDispatcher는 디스패처를 생성하고 작업자 풀을 초기화합니다.
func NewDispatcher(jobQueue chan Job, maxWorkers int) *Dispatcher {
	workerPool := make(chan chan Job, maxWorkers)

	return &Dispatcher{
		WorkerPool:  workerPool,
		JobQueue:    jobQueue,
		maxWorkers:  maxWorkers,
		workers:     make([]*Worker, 0, maxWorkers),
		wg:          new(sync.WaitGroup),
		quit:        make(chan struct{}),
	}
}

// Start는 디스패처 루프를 실행하고 작업자 풀을 가동합니다.
func (d *Dispatcher) Start(executor *TaskExecutor) {
	// 1. 설정된 수(maxWorkers)만큼 작업자(Worker)를 생성하고 시작합니다.
	for i := 0; i < d.maxWorkers; i++ {
		d.wg.Add(1)
		worker := NewWorker(i+1, d.WorkerPool, d.wg, d.quit, executor)
		worker.Start()
		d.workers = append(d.workers, worker)
	}

	// 2. 디스패치 루프를 별도의 Goroutine으로 실행합니다.
	go d.dispatch()
	log.Printf("%d개의 작업자(Worker)로 디스패처를 시작합니다.", d.maxWorkers)
}

// dispatch는 JobQueue에서 작업을 가져와 WorkerPool의 유휴 작업자에게 전달합니다.
func (d *Dispatcher) dispatch() {
	for {
		select {
		case job := <-d.JobQueue: // 1. 작업 큐에서 새 작업 수신
			// 2. 유휴 작업자의 작업 채널을 WorkerPool에서 가져옵니다.
			//    (유휴 작업자가 없으면 여기서 블록됩니다.)
			workerJobChannel := <-d.WorkerPool

			// 3. 해당 작업자에게 작업 전달
			workerJobChannel <- job

		case <-d.quit:
			// 4. 종료 신호 수신
			return
		}
	}
}

// Stop은 모든 작업자와 디스패처를 우아하게 종료합니다.
func (d *Dispatcher) Stop() {
	log.Println("디스패처 종료 신호 수신...")
	close(d.quit)
	d.wg.Wait()
	log.Println("모든 작업자가 중지되었습니다.")
}
