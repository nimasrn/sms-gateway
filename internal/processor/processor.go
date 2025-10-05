package processor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nimasrn/message-gateway/internal/config"
	"github.com/nimasrn/message-gateway/internal/queue"
	"github.com/nimasrn/message-gateway/pkg/logger"
	"github.com/nimasrn/message-gateway/pkg/redis"
	"github.com/nimasrn/message-gateway/pkg/worker"
)

const ProcessingTimeout = time.Second * 5
const HealthInterval = time.Second * 30
const ShutdownTimeout = time.Minute

// ProcessorService is the main service that processes queue messages
type ProcessorService struct {
	adapter    redis.RedisAdapter
	queues     []*queue.Queue
	processors Processor
	metrics    *ServiceMetrics
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	worker     *worker.WorkerManager
}

// Processor interface for different message processors
type Processor interface {
	Process(ctx context.Context, message *queue.Message) error
	GetType() string
}

func NewProcessorService(redis redis.RedisAdapter) (*ProcessorService, error) {
	ctx, cancel := context.WithCancel(context.Background())
	service := &ProcessorService{
		adapter:    redis,
		queues:     make([]*queue.Queue, 0),
		processors: nil,
		metrics:    NewServiceMetrics(),
		ctx:        ctx,
		cancel:     cancel,
		worker:     worker.NewWorkerManager(10_000, 100, nil),
	}
	return service, nil
}

// RegisterProcessor registers a new processor
func (s *ProcessorService) RegisterProcessor(processor Processor) {
	s.processors = processor
	logger.Info("Registered processor", "type", processor.GetType())
}

// Start starts the processor service
func (s *ProcessorService) Start() error {
	logger.Info("Starting Processor Service...")

	// Set up worker handler
	s.worker.SetWorker(s.workerHandler)

	// Start worker pool in background
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.worker.Start(); err != nil {
			logger.Error("Worker manager stopped", "error", err)
		}
	}()

	// Create queue consumers
	for i := 0; i < 10; i++ {
		queueConfig := queue.QueueConfig{
			Name:              config.Get().QueueName,
			ConsumerGroup:     config.Get().QueueConsumerGroup,
			ConsumerName:      config.Get().QueueConsumerName,
			MaxRetries:        config.Get().QueueMaxRetries,
			VisibilityTimeout: config.Get().QueueVisibilityTimeout,
			PollInterval:      config.Get().QueuePollInterval,
			BatchSize:         config.Get().QueueBatchSize,
			MaxLen:            config.Get().QueueMaxLen,
			EnableDLQ:         config.Get().QueueEnableDLQ,
		}
		queueConfig.ConsumerName = fmt.Sprintf("%s-instance-%d", queueConfig.ConsumerName, i)

		q, err := queue.NewQueue(s.adapter, queueConfig)
		if err != nil {
			return fmt.Errorf("failed to create queue %d: %w", i, err)
		}

		// Start consuming - messages will be enqueued to worker pool
		if err := q.Consume(s.messageHandler); err != nil {
			return fmt.Errorf("failed to start consumer %d: %w", i, err)
		}

		s.queues = append(s.queues, q)
		logger.Info("Started consumer instance", "instance", i)
	}

	// Start background tasks
	s.wg.Add(2)
	go s.metricsReporter()
	go s.healthChecker()

	logger.Info("Processor Service started", "consumers", len(s.queues), "workers", 100)
	return nil
}

// metricsReporter periodically reports metrics
func (s *ProcessorService) metricsReporter() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.reportMetrics()
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *ProcessorService) reportMetrics() {
	stats := s.metrics.GetStats()

	logger.Info("=== Service Metrics ===")
	logger.Info("Metrics", "total_processed", stats["total_processed"], "total_failed", stats["total_failed"], "rate_per_second", stats["rate_per_second"], "avg_duration_ms", stats["avg_duration_ms"], "uptime_seconds", stats["uptime_seconds"])

	// Queue stats
	for i, q := range s.queues {
		if qStats, err := q.GetStats(); err == nil {
			logger.Info("Queue stats", "queue", i, "total", qStats.TotalMessages, "pending", qStats.PendingMessages)
		}
	}
}

func (s *ProcessorService) healthChecker() {
	defer s.wg.Done()

	ticker := time.NewTicker(HealthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.performHealthCheck()
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *ProcessorService) performHealthCheck() {
	// Check Redis connection
	if err := s.adapter.Client().Ping(context.Background()).Err(); err != nil {
		logger.Error("HEALTH CHECK FAILED: Redis connection error", "error", err)
		return
	}

	// Check queue health
	for i, q := range s.queues {
		stats, err := q.GetStats()
		if err != nil {
			logger.Warn("HEALTH CHECK WARNING: Queue stats unavailable", "queue", i, "error", err)
			continue
		}

		// Alert if pending messages are high
		if stats.PendingMessages > 10000 {
			logger.Warn("HEALTH CHECK WARNING: Queue has high lag", "queue", i, "pending_messages", stats.PendingMessages)
		}
	}

	logger.Info("HEALTH CHECK: OK - Service healthy")
}

// Stop gracefully stops the service
func (s *ProcessorService) Stop() {
	logger.Info("Shutting down Processor Service...")

	s.cancel()

	// Stop all queues
	timeout := ShutdownTimeout
	stopChan := make(chan bool, len(s.queues))

	for i, q := range s.queues {
		go func(index int, queue *queue.Queue) {
			if err := queue.Stop(timeout); err != nil {
				logger.Error("Error stopping queue", "queue", index, "error", err)
			}
			stopChan <- true
		}(i, q)
	}

	// Wait for all queues
	for range s.queues {
		select {
		case <-stopChan:
		case <-time.After(timeout + 5*time.Second):
			logger.Warn("Timeout waiting for queues to stop")
		}
	}

	// Stop worker manager
	s.worker.Exit()

	// Wait for background tasks
	s.wg.Wait()

	// Final metrics
	s.reportMetrics()

	logger.Info("Processor Service stopped")
}

type jobResult struct {
	msg        *queue.Message
	resultChan chan error
	ctx        context.Context
}

// messageHandler receives messages from queue and enqueues to worker pool
func (s *ProcessorService) messageHandler(ctx context.Context, msg *queue.Message) error {
	// Create a result channel for this message
	resultChan := make(chan error, 1)

	// Create cancellable context with timeout for this message
	msgCtx, cancel := context.WithTimeout(ctx, ProcessingTimeout+time.Second)
	defer cancel()

	// Wrap message with result channel and context
	job := &jobResult{
		msg:        msg,
		resultChan: resultChan,
		ctx:        msgCtx,
	}

	// Enqueue to worker pool
	s.worker.Enqueue(job)

	// Block until worker completes processing or context times out
	select {
	case err := <-resultChan:
		return err
	case <-msgCtx.Done():
		// Context cancelled or timed out
		return fmt.Errorf("timeout waiting for worker to process message: %w", msgCtx.Err())
	}
}

// workerHandler processes messages in worker pool
func (s *ProcessorService) workerHandler(workerIndex int, job interface{}) {
	jobRes, ok := job.(*jobResult)
	if !ok {
		logger.Error("Invalid job type in worker", "worker", workerIndex)
		return
	}

	msg := jobRes.msg
	start := time.Now()
	var resultErr error

	// Check if context already cancelled before processing
	select {
	case <-jobRes.ctx.Done():
		logger.Warn("Job context cancelled before processing started", "worker", workerIndex)
		return
	default:
		// Continue processing
	}

	// Find processor for this event type
	if s.processors == nil {
		logger.Info("No processor found", "worker", workerIndex)
		s.metrics.RecordFailure()
		resultErr = nil // ACK - unknown type won't succeed on retry
	} else {
		// Use the context from jobResult (already has timeout from messageHandler)
		if err := s.processors.Process(jobRes.ctx, msg); err != nil {
			s.metrics.RecordFailure()
			logger.Error("Failed to process message", "worker", workerIndex, "error", err)
			resultErr = err // NACK - return error
		} else {
			// Success
			duration := time.Since(start)
			s.metrics.RecordSuccess(duration)
			resultErr = nil // ACK - return nil
		}
	}

	// Non-blocking send to result channel
	// If messageHandler timed out, channel may have no receiver
	select {
	case jobRes.resultChan <- resultErr:
		// Successfully sent result
	case <-jobRes.ctx.Done():
		// Context cancelled while trying to send result
		logger.Warn("Context cancelled while sending result, message handler timed out", "worker", workerIndex)
	}
}
