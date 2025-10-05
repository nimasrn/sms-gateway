package processor

import (
	"sync/atomic"
	"time"
)

type ServiceMetrics struct {
	totalProcessed  int64
	totalFailed     int64
	totalDurationNs int64
	lastResetNs     int64
}

func NewServiceMetrics() *ServiceMetrics {
	return &ServiceMetrics{
		lastResetNs: time.Now().UnixNano(),
	}
}

func (m *ServiceMetrics) RecordSuccess(duration time.Duration) {
	atomic.AddInt64(&m.totalProcessed, 1)
	atomic.AddInt64(&m.totalDurationNs, int64(duration))
}

func (m *ServiceMetrics) RecordFailure() {
	atomic.AddInt64(&m.totalFailed, 1)
}

func (m *ServiceMetrics) GetStats() map[string]interface{} {
	// Atomically load all values
	processed := atomic.LoadInt64(&m.totalProcessed)
	failed := atomic.LoadInt64(&m.totalFailed)
	durationNs := atomic.LoadInt64(&m.totalDurationNs)
	lastResetNs := atomic.LoadInt64(&m.lastResetNs)

	elapsed := time.Since(time.Unix(0, lastResetNs)).Seconds()

	rate := 0.0
	if elapsed > 0 {
		rate = float64(processed) / elapsed
	}

	avgDuration := time.Duration(0)
	if processed > 0 {
		avgDuration = time.Duration(durationNs / processed)
	}

	stats := map[string]interface{}{
		"total_processed": processed,
		"total_failed":    failed,
		"rate_per_second": rate,
		"avg_duration_ms": avgDuration.Milliseconds(),
		"uptime_seconds":  elapsed,
	}

	return stats
}

func (m *ServiceMetrics) Reset() {
	atomic.StoreInt64(&m.totalProcessed, 0)
	atomic.StoreInt64(&m.totalFailed, 0)
	atomic.StoreInt64(&m.totalDurationNs, 0)
	atomic.StoreInt64(&m.lastResetNs, time.Now().UnixNano())
}
