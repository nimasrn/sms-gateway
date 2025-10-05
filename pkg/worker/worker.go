package worker

import (
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/nimasrn/message-gateway/pkg/logger"
)

type WorkerHandler = func(workerIndex int, job interface{})

type WorkerManager struct {
	bufferSize     int
	jobChannel     chan interface{}
	numberOfWorker int
	sigTerm        chan os.Signal
	do             WorkerHandler
	errHandler     func(err error)
	waiter         *sync.WaitGroup
}

// NewWorkerManager
// is a job manager based on go routines. Define the number of internal
// workers, and start publishing jobs using WorkerManager Publish() API. It will distribute the job
// among its internal pool. The worker never exit and are always listening.
// To exit the worker, simply call Exit(), it will register a sigterm BUT the associated
// job channel will NOT be closed, because channel is externally passed to this instance,
// and other processes might be using the channel.
func NewWorkerManager(bufferSize, numberOfWorkers int, jobChannel chan interface{}) *WorkerManager {
	if jobChannel == nil {
		jobChannel = make(chan interface{}, bufferSize)
	}
	// Buffered channel prevents signal loss if signals arrive before workers start
	// Buffer size = numberOfWorkers ensures each worker can receive one signal
	var sigChan = make(chan os.Signal, numberOfWorkers)
	// Only handle SIGTERM (SIGKILL cannot be caught, SIGSTOP suspends process)
	signal.Notify(sigChan, syscall.SIGTERM)

	return &WorkerManager{
		bufferSize:     bufferSize,
		numberOfWorker: numberOfWorkers,
		jobChannel:     jobChannel,
		sigTerm:        sigChan,
		waiter:         &sync.WaitGroup{},
	}
}

func (w *WorkerManager) GetUnreadCount() int64 {
	if w.jobChannel == nil {
		return 0
	}
	return int64(len(w.jobChannel))
}

func (w *WorkerManager) JobEvents() chan interface{} {
	return w.jobChannel
}

func (w *WorkerManager) SetWorker(worker WorkerHandler) {
	w.do = worker
}

// Enqueue
// Publishes a message onto the channel
func (w *WorkerManager) Enqueue(val interface{}) {
	w.jobChannel <- val
}

// Start
// starts off the workers as many as defined
// by w.numberOfWorker
func (w *WorkerManager) Start() error {
	w.waiter.Add(w.numberOfWorker)
	for i := 0; i < w.numberOfWorker; i++ {
		go func(index int) {
			for {
				select {
				case job := <-w.jobChannel:
					w.do(index, job)
				case <-w.sigTerm:
					w.waiter.Done()
					return
				}
			}
		}(i)
	}
	w.waiter.Wait()

	return errors.New("workers terminated")
}

// Exit
// calling this function would immediately exit from all sub-processes
func (w *WorkerManager) Exit() {
	logger.Info("Exit() is called and worker manager is going to be shutdown")
	for i := 0; i < w.numberOfWorker; i++ {
		w.sigTerm <- syscall.SIGSTOP
	}
}
