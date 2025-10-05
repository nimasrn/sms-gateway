package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nimasrn/message-gateway/pkg/logger"
	"github.com/valyala/fasthttp"
)

var (
	ErrNoAvailableProviders = errors.New("no available providers")
)

type DeliveryStatus string

const (
	StatusDelivered DeliveryStatus = "DELIVERED"
	StatusFailed    DeliveryStatus = "FAILED"
	StatusPending   DeliveryStatus = "PENDING"
)

// Request/Response types
type SendRequest struct {
	MessageID   string `json:"message_id"`
	PhoneNumber string `json:"phone_number"`
	Content     string `json:"content"`
	Priority    string `json:"priority"`
}

type SendResponse struct {
	MessageID   string         `json:"message_id"`
	Status      DeliveryStatus `json:"status"`
	DeliveredAt *time.Time     `json:"delivered_at,omitempty"`
	ErrorCode   string         `json:"error_code,omitempty"`
	ErrorMsg    string         `json:"error_message,omitempty"`
	OperatorID  string         `json:"operator_id"`
	ProcessedAt time.Time      `json:"processed_at"`
}

type StatusResponse struct {
	MessageID   string         `json:"message_id"`
	Status      DeliveryStatus `json:"status"`
	DeliveredAt *time.Time     `json:"delivered_at,omitempty"`
	ErrorCode   string         `json:"error_code,omitempty"`
	ErrorMsg    string         `json:"error_message,omitempty"`
	OperatorID  string         `json:"operator_id"`
}

type ProviderMetrics struct {
	TotalRequests    atomic.Int64
	SuccessfulReqs   atomic.Int64
	FailedReqs       atomic.Int64
	TotalLatencyMs   atomic.Int64
	LastLatencyMs    atomic.Int64
	ConsecutiveFails atomic.Int32
	LastErrorTime    atomic.Int64
	LastSuccessTime  atomic.Int64

	mu             sync.RWMutex
	latencyHistory []int64 // Last N latencies for percentile calculation
	maxHistorySize int
}

func NewProviderMetrics() *ProviderMetrics {
	return &ProviderMetrics{
		latencyHistory: make([]int64, 0, 100),
		maxHistorySize: 100,
	}
}

func (m *ProviderMetrics) RecordSuccess(latencyMs int64) {
	m.TotalRequests.Add(1)
	m.SuccessfulReqs.Add(1)
	m.TotalLatencyMs.Add(latencyMs)
	m.LastLatencyMs.Store(latencyMs)
	m.ConsecutiveFails.Store(0)
	m.LastSuccessTime.Store(time.Now().Unix())

	m.mu.Lock()
	if len(m.latencyHistory) >= m.maxHistorySize {
		m.latencyHistory = m.latencyHistory[1:]
	}
	m.latencyHistory = append(m.latencyHistory, latencyMs)
	m.mu.Unlock()
}

func (m *ProviderMetrics) RecordFailure() {
	m.TotalRequests.Add(1)
	m.FailedReqs.Add(1)
	m.ConsecutiveFails.Add(1)
	m.LastErrorTime.Store(time.Now().Unix())
}

func (m *ProviderMetrics) AvgLatencyMs() int64 {
	total := m.TotalRequests.Load()
	if total == 0 {
		return 0
	}
	return m.TotalLatencyMs.Load() / total
}

func (m *ProviderMetrics) SuccessRate() float64 {
	total := m.TotalRequests.Load()
	if total == 0 {
		return 1.0
	}
	return float64(m.SuccessfulReqs.Load()) / float64(total)
}

func (m *ProviderMetrics) P95LatencyMs() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.latencyHistory) == 0 {
		return 0
	}

	sorted := make([]int64, len(m.latencyHistory))
	copy(sorted, m.latencyHistory)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	p95Index := int(float64(len(sorted)) * 0.95)
	if p95Index >= len(sorted) {
		p95Index = len(sorted) - 1
	}
	return sorted[p95Index]
}

type ProviderState int

const (
	StateHealthy ProviderState = iota
	StateDegraded
	StateUnhealthy
	StateCircuitOpen
)

type Provider struct {
	name             string
	url              string
	client           *fasthttp.Client
	metrics          *ProviderMetrics
	state            atomic.Int32
	weight           atomic.Int32 // Base weight/priority
	lastHealthCheck  atomic.Int64
	circuitOpenUntil atomic.Int64

	mu sync.RWMutex
}

func NewProvider(name, url string, weight int, client *fasthttp.Client) *Provider {
	p := &Provider{
		name:    name,
		url:     url,
		client:  client,
		metrics: NewProviderMetrics(),
	}
	p.state.Store(int32(StateHealthy))
	p.weight.Store(int32(weight))
	return p
}

func (p *Provider) GetState() ProviderState {
	return ProviderState(p.state.Load())
}

func (p *Provider) SetState(state ProviderState) {
	p.state.Store(int32(state))
}

func (p *Provider) IsAvailable() bool {
	state := p.GetState()
	if state == StateCircuitOpen {
		// Check if circuit should close
		openUntil := p.circuitOpenUntil.Load()
		if time.Now().Unix() > openUntil {
			p.SetState(StateDegraded)
			return true
		}
		return false
	}
	return state != StateUnhealthy
}

// CalculateScore calculates provider score based on metrics (higher is better)
func (p *Provider) CalculateScore() float64 {
	if !p.IsAvailable() {
		return 0.0
	}

	metrics := p.metrics
	baseWeight := float64(p.weight.Load())

	// Success rate weight (0-100 points)
	successRate := metrics.SuccessRate()
	successScore := successRate * 100

	// Latency score (0-100 points, lower latency = higher score)
	avgLatency := metrics.AvgLatencyMs()
	latencyScore := 100.0
	if avgLatency > 0 {
		// Normalize: 100ms = 100 points, 1000ms = 10 points, 5000ms+ = 0 points
		latencyScore = 100.0 * (1.0 - (float64(avgLatency) / 5000.0))
		if latencyScore < 0 {
			latencyScore = 0
		}
	}

	// Recent performance weight (penalize recent failures)
	consecutiveFails := float64(metrics.ConsecutiveFails.Load())
	recentPenalty := 1.0 - (consecutiveFails * 0.1) // Each fail reduces by 10%
	if recentPenalty < 0.1 {
		recentPenalty = 0.1
	}

	// State penalty
	statePenalty := 1.0
	switch p.GetState() {
	case StateDegraded:
		statePenalty = 0.5
	case StateUnhealthy, StateCircuitOpen:
		statePenalty = 0.0
	}

	// Calculate final score
	score := (successScore*0.4 + latencyScore*0.4 + baseWeight*0.2) * recentPenalty * statePenalty

	return score
}

type Config struct {
	Providers               []ProviderConfig
	Timeout                 time.Duration
	MaxRetries              int
	RetryDelay              time.Duration
	MaxConns                int
	ReadBufferSize          int
	WriteBufferSize         int
	HealthCheckInterval     time.Duration
	CircuitBreakerThreshold int
	CircuitBreakerTimeout   time.Duration
	MetricsWindow           time.Duration
}

type ProviderConfig struct {
	Name   string
	URL    string
	Weight int // Base priority weight (1-100)
}

type Client struct {
	config    *Config
	providers []*Provider
	mu        sync.RWMutex
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

func NewClient(config *Config) (*Client, error) {
	if config == nil {
		return nil, errors.New("config is required")
	}

	if len(config.Providers) == 0 {
		return nil, errors.New("at least one provider is required")
	}

	client := &Client{
		config:    config,
		providers: make([]*Provider, 0, len(config.Providers)),
		stopCh:    make(chan struct{}),
	}

	// Initialize providers
	for _, pc := range config.Providers {
		httpClient := &fasthttp.Client{
			MaxConnsPerHost:     config.MaxConns,
			ReadTimeout:         config.Timeout,
			WriteTimeout:        config.Timeout,
			MaxIdleConnDuration: 60 * time.Second,
			ReadBufferSize:      config.ReadBufferSize,
			WriteBufferSize:     config.WriteBufferSize,
		}

		provider := NewProvider(pc.Name, pc.URL, pc.Weight, httpClient)
		client.providers = append(client.providers, provider)

		logger.Info("Provider initialized", "name", pc.Name, "url", pc.URL, "weight", pc.Weight)
	}

	// Start background tasks
	client.wg.Add(2)
	go client.healthChecker()
	go client.metricsCollector()

	logger.Info("Operator client initialized", "providers", len(client.providers), "timeout", config.Timeout)

	return client, nil
}

// SelectBestProvider selects the best performing provider
func (c *Client) SelectBestProvider() (*Provider, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.providers) == 0 {
		return nil, ErrNoAvailableProviders
	}

	var bestProvider *Provider
	var bestScore float64

	for _, provider := range c.providers {
		if !provider.IsAvailable() {
			continue
		}

		score := provider.CalculateScore()
		if score > bestScore {
			bestScore = score
			bestProvider = provider
		}
	}

	if bestProvider == nil {
		return nil, ErrNoAvailableProviders
	}

	logger.Debug("Selected provider", "provider", bestProvider.name, "score", bestScore)

	return bestProvider, nil
}

// SendSMS sends a single SMS through the best available provider
func (c *Client) SendSMS(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.config.RetryDelay):
			}
		}

		provider, err := c.SelectBestProvider()
		if err != nil {
			lastErr = err
			continue
		}

		startTime := time.Now()
		response, err := c.doRequest(ctx, provider, "POST", "/api/v1/sms/send", reqBody)
		latency := time.Since(startTime).Milliseconds()

		if err != nil {
			provider.metrics.RecordFailure()
			c.checkCircuitBreaker(provider)

			logger.Warn("Request failed, retrying", "error", err, "provider", provider.name, "attempt", attempt+1)

			lastErr = err
			continue
		}

		provider.metrics.RecordSuccess(latency)

		var resp SendResponse
		if err := json.Unmarshal(response, &resp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}

		logger.Info("SMS sent to provider", "message_id", req.MessageID, "status", string(resp.Status), "provider", provider.name, "latency_ms", latency)

		return &resp, nil
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", c.config.MaxRetries+1, lastErr)
}

// GetStatus queries the delivery status of a message
func (c *Client) GetStatus(ctx context.Context, messageID string) (*StatusResponse, error) {
	provider, err := c.SelectBestProvider()
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/api/v1/sms/status/%s", messageID)
	response, err := c.doRequest(ctx, provider, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp StatusResponse
	if err := json.Unmarshal(response, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp, nil
}

// doRequest performs HTTP request with timeout
func (c *Client) doRequest(ctx context.Context, provider *Provider, method, path string, body []byte) ([]byte, error) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	url := provider.url + path
	req.SetRequestURI(url)
	req.Header.SetMethod(method)
	req.Header.SetContentType("application/json")

	if body != nil {
		req.SetBody(body)
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(c.config.Timeout)
	}

	if err := provider.client.DoDeadline(req, resp, deadline); err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	statusCode := resp.StatusCode()
	if statusCode != fasthttp.StatusOK && statusCode != fasthttp.StatusAccepted {
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", statusCode, resp.Body())
	}

	result := make([]byte, len(resp.Body()))
	copy(result, resp.Body())

	return result, nil
}

func (c *Client) checkCircuitBreaker(provider *Provider) {
	consecutiveFails := provider.metrics.ConsecutiveFails.Load()
	if consecutiveFails >= int32(c.config.CircuitBreakerThreshold) {
		provider.SetState(StateCircuitOpen)
		openUntil := time.Now().Add(c.config.CircuitBreakerTimeout).Unix()
		provider.circuitOpenUntil.Store(openUntil)

		logger.Warn("Circuit breaker opened", "provider", provider.name, "consecutive_fails", consecutiveFails, "timeout", c.config.CircuitBreakerTimeout)
	}
}

func (c *Client) healthChecker() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.performHealthChecks()
		case <-c.stopCh:
			return
		}
	}
}

// performHealthChecks checks health of all providers
func (c *Client) performHealthChecks() {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.Timeout)
	defer cancel()

	c.mu.RLock()
	providers := make([]*Provider, len(c.providers))
	copy(providers, c.providers)
	c.mu.RUnlock()

	for _, provider := range providers {
		healthy := c.checkProviderHealth(ctx, provider)
		provider.lastHealthCheck.Store(time.Now().Unix())

		oldState := provider.GetState()
		newState := oldState

		if healthy {
			if oldState == StateUnhealthy || oldState == StateDegraded {
				newState = StateHealthy
			}
		} else {
			newState = StateUnhealthy
		}

		if newState != oldState {
			provider.SetState(newState)
			logger.Info("Provider state changed", "provider", provider.name, "old_state", stateString(oldState), "new_state", stateString(newState))
		}
	}
}

// checkProviderHealth checks if a provider is healthy
func (c *Client) checkProviderHealth(ctx context.Context, provider *Provider) bool {
	response, err := c.doRequest(ctx, provider, "GET", "/health", nil)
	if err != nil {
		return false
	}

	var health struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(response, &health); err != nil {
		return false
	}

	return health.Status == "healthy"
}

// metricsCollector periodically evaluates provider performance
func (c *Client) metricsCollector() {
	defer c.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.evaluateProviders()
		case <-c.stopCh:
			return
		}
	}
}

// evaluateProviders evaluates and adjusts provider states based on metrics
func (c *Client) evaluateProviders() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, provider := range c.providers {
		if provider.GetState() == StateCircuitOpen {
			continue
		}

		successRate := provider.metrics.SuccessRate()
		avgLatency := provider.metrics.AvgLatencyMs()

		// Determine state based on performance
		if successRate < 0.8 || avgLatency > 5000 {
			if provider.GetState() != StateDegraded {
				provider.SetState(StateDegraded)
				logger.Warn("Provider degraded", "provider", provider.name, "success_rate", successRate, "avg_latency_ms", avgLatency)
			}
		} else if successRate > 0.95 && avgLatency < 2000 {
			if provider.GetState() != StateHealthy {
				provider.SetState(StateHealthy)
				logger.Info("Provider recovered to healthy state", "provider", provider.name)
			}
		}
	}
}

// GetProviderStats returns detailed statistics for all providers
func (c *Client) GetProviderStats() []ProviderStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := make([]ProviderStats, 0, len(c.providers))
	for _, provider := range c.providers {
		stats = append(stats, ProviderStats{
			Name:             provider.name,
			URL:              provider.url,
			State:            stateString(provider.GetState()),
			Score:            provider.CalculateScore(),
			TotalRequests:    provider.metrics.TotalRequests.Load(),
			SuccessfulReqs:   provider.metrics.SuccessfulReqs.Load(),
			FailedReqs:       provider.metrics.FailedReqs.Load(),
			SuccessRate:      provider.metrics.SuccessRate(),
			AvgLatencyMs:     provider.metrics.AvgLatencyMs(),
			P95LatencyMs:     provider.metrics.P95LatencyMs(),
			LastLatencyMs:    provider.metrics.LastLatencyMs.Load(),
			ConsecutiveFails: provider.metrics.ConsecutiveFails.Load(),
		})
	}

	// Sort by score
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Score > stats[j].Score
	})

	return stats
}

// Close closes the client and releases resources
func (c *Client) Close() error {
	close(c.stopCh)
	c.wg.Wait()
	logger.Info("Operator client closed")
	return nil
}

// Supporting types
type ProviderStats struct {
	Name             string
	URL              string
	State            string
	Score            float64
	TotalRequests    int64
	SuccessfulReqs   int64
	FailedReqs       int64
	SuccessRate      float64
	AvgLatencyMs     int64
	P95LatencyMs     int64
	LastLatencyMs    int64
	ConsecutiveFails int32
}

func stateString(state ProviderState) string {
	switch state {
	case StateHealthy:
		return "HEALTHY"
	case StateDegraded:
		return "DEGRADED"
	case StateUnhealthy:
		return "UNHEALTHY"
	case StateCircuitOpen:
		return "CIRCUIT_OPEN"
	default:
		return "UNKNOWN"
	}
}
