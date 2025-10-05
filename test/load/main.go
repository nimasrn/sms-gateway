package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Request payload structure
type MessagePayload struct {
	CustomerID int    `json:"customer_id"`
	Mobile     string `json:"mobile"`
	Content    string `json:"content"`
}

// Test configuration
type LoadTestConfig struct {
	URL               string
	RequestsPerSecond int
	DurationSeconds   int
	ConcurrentWorkers int
	APIKey            string
}

// Stats tracking
type Stats struct {
	successCount  atomic.Int64
	errorCount    atomic.Int64
	responseTimes []float64
	mu            sync.Mutex
}

func (s *Stats) addResponseTime(duration float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responseTimes = append(s.responseTimes, duration)
}

func (s *Stats) getResponseTimes() []float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	times := make([]float64, len(s.responseTimes))
	copy(times, s.responseTimes)
	return times
}

func sendRequest(client *http.Client, config LoadTestConfig, payload []byte, stats *Stats) {
	start := time.Now()

	req, err := http.NewRequest("POST", config.URL, bytes.NewBuffer(payload))
	if err != nil {
		stats.errorCount.Add(1)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", config.APIKey)

	resp, err := client.Do(req)
	if err != nil {
		stats.errorCount.Add(1)
		stats.addResponseTime(time.Since(start).Seconds())
		return
	}
	defer resp.Body.Close()

	// Read and discard body
	io.Copy(io.Discard, resp.Body)

	duration := time.Since(start).Seconds()
	stats.addResponseTime(duration)

	if resp.StatusCode == 200 || resp.StatusCode == 201 {
		stats.successCount.Add(1)
	} else {
		stats.errorCount.Add(1)
	}
}

func worker(client *http.Client, config LoadTestConfig, payload []byte, stats *Stats, jobs <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()

	for range jobs {
		sendRequest(client, config, payload, stats)
	}
}

func calculatePercentile(times []float64, percentile float64) float64 {
	if len(times) == 0 {
		return 0
	}

	// Sort times
	sorted := make([]float64, len(times))
	copy(sorted, times)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	index := int(float64(len(sorted)) * percentile)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func main() {
	// Read configuration from environment variables
	config := LoadTestConfig{
		URL:               getEnvOrDefault("TARGET_URL", "http://localhost:8080/api/v1/messages/express"),
		RequestsPerSecond: getEnvIntOrDefault("REQUESTS_PER_SECOND", 5000),
		DurationSeconds:   getEnvIntOrDefault("DURATION_SECONDS", 30),
		ConcurrentWorkers: getEnvIntOrDefault("CONCURRENT_WORKERS", 500),
		APIKey:            getEnvOrDefault("API_KEY", ""),
	}

	// Create payload
	payload := MessagePayload{
		CustomerID: 1,
		Mobile:     "+4915123456789",
		Content:    "Hello from Load Test",
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}

	// Print test configuration
	fmt.Println("Starting load test...")
	fmt.Printf("Target: %s\n", config.URL)
	fmt.Printf("Total requests: %d\n", config.RequestsPerSecond*config.DurationSeconds)
	fmt.Printf("Target RPS: %d\n", config.RequestsPerSecond)
	fmt.Printf("Concurrent workers: %d\n", config.ConcurrentWorkers)
	fmt.Printf("Duration: %d seconds\n", config.DurationSeconds)
	fmt.Println(strings.Repeat("-", 50))

	stats := &Stats{}

	// Create HTTP client with connection pooling
	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        config.ConcurrentWorkers,
			MaxIdleConnsPerHost: config.ConcurrentWorkers,
			IdleConnTimeout:     90 * time.Second,
		},
		Timeout: 60 * time.Second,
	}

	// Create job channel
	jobs := make(chan struct{}, config.RequestsPerSecond)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < config.ConcurrentWorkers; i++ {
		wg.Add(1)
		go worker(client, config, payloadBytes, stats, jobs, &wg)
	}

	// Send requests
	startTime := time.Now()
	totalRequests := config.RequestsPerSecond * config.DurationSeconds
	requestsSent := 0

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for i := 0; i < config.DurationSeconds && requestsSent < totalRequests; i++ {
		batchStart := time.Now()

		// Send batch of requests for this second
		for j := 0; j < config.RequestsPerSecond && requestsSent < totalRequests; j++ {
			jobs <- struct{}{}
			requestsSent++
		}

		// Progress update
		success := stats.successCount.Load()
		errors := stats.errorCount.Load()
		fmt.Printf("[%ds] Completed: %d | Success: %d | Errors: %d\n",
			i+1, success+errors, success, errors)

		// Wait for next second
		elapsed := time.Since(batchStart)
		if elapsed < time.Second {
			time.Sleep(time.Second - elapsed)
		}
	}

	close(jobs)
	wg.Wait()

	endTime := time.Now()
	duration := endTime.Sub(startTime).Seconds()

	// Calculate statistics
	success := stats.successCount.Load()
	errors := stats.errorCount.Load()
	total := success + errors
	actualRPS := float64(total) / duration

	times := stats.getResponseTimes()
	var avgResponseTime float64
	var minTime, maxTime float64

	if len(times) > 0 {
		sum := 0.0
		minTime = times[0]
		maxTime = times[0]

		for _, t := range times {
			sum += t
			if t < minTime {
				minTime = t
			}
			if t > maxTime {
				maxTime = t
			}
		}
		avgResponseTime = sum / float64(len(times))
	}

	p50 := calculatePercentile(times, 0.50)
	p95 := calculatePercentile(times, 0.95)
	p99 := calculatePercentile(times, 0.99)

	// Print results
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("LOAD TEST RESULTS")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("Duration: %.2f seconds\n", duration)
	fmt.Printf("Total requests: %d\n", total)
	fmt.Printf("Successful: %d\n", success)
	fmt.Printf("Failed: %d\n", errors)
	if total > 0 {
		fmt.Printf("Success rate: %.2f%%\n", float64(success)/float64(total)*100)
	}
	fmt.Printf("\nActual RPS: %.2f\n", actualRPS)
	fmt.Printf("\nResponse times:\n")
	fmt.Printf("  Average: %.2f ms\n", avgResponseTime*1000)
	fmt.Printf("  P50: %.2f ms\n", p50*1000)
	fmt.Printf("  P95: %.2f ms\n", p95*1000)
	fmt.Printf("  P99: %.2f ms\n", p99*1000)
	fmt.Printf("  Min: %.2f ms\n", minTime*1000)
	fmt.Printf("  Max: %.2f ms\n", maxTime*1000)
}
