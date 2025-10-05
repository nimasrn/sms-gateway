package gateway

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func TestProviderMetrics_RecordSuccess(t *testing.T) {
	metrics := NewProviderMetrics()

	metrics.RecordSuccess(100)
	metrics.RecordSuccess(200)

	assert.Equal(t, int64(2), metrics.TotalRequests.Load())
	assert.Equal(t, int64(2), metrics.SuccessfulReqs.Load())
	assert.Equal(t, int64(0), metrics.FailedReqs.Load())
	assert.Equal(t, float64(1.0), metrics.SuccessRate())
	assert.Equal(t, int64(150), metrics.AvgLatencyMs())
}

func TestProviderMetrics_RecordFailure(t *testing.T) {
	metrics := NewProviderMetrics()

	metrics.RecordSuccess(100)
	metrics.RecordFailure()
	metrics.RecordFailure()

	assert.Equal(t, int64(3), metrics.TotalRequests.Load())
	assert.Equal(t, int64(1), metrics.SuccessfulReqs.Load())
	assert.Equal(t, int64(2), metrics.FailedReqs.Load())
	assert.InDelta(t, 0.333, metrics.SuccessRate(), 0.01)
	assert.Equal(t, int32(2), metrics.ConsecutiveFails.Load())
}

func TestProviderMetrics_P95Latency(t *testing.T) {
	metrics := NewProviderMetrics()

	for i := int64(0); i < 100; i++ {
		metrics.RecordSuccess(i * 10)
	}

	p95 := metrics.P95LatencyMs()
	assert.GreaterOrEqual(t, p95, int64(900))
	assert.LessOrEqual(t, p95, int64(990))
}

func TestProvider_IsAvailable(t *testing.T) {
	client := &fasthttp.Client{}
	provider := NewProvider("test", "http://localhost:8080", 100, client)

	t.Run("healthy provider is available", func(t *testing.T) {
		provider.SetState(StateHealthy)
		assert.True(t, provider.IsAvailable())
	})

	t.Run("degraded provider is available", func(t *testing.T) {
		provider.SetState(StateDegraded)
		assert.True(t, provider.IsAvailable())
	})

	t.Run("unhealthy provider is not available", func(t *testing.T) {
		provider.SetState(StateUnhealthy)
		assert.False(t, provider.IsAvailable())
	})

	t.Run("circuit open provider becomes available after timeout", func(t *testing.T) {
		provider.SetState(StateCircuitOpen)
		provider.circuitOpenUntil.Store(time.Now().Add(-1 * time.Second).Unix())
		assert.True(t, provider.IsAvailable())
		assert.Equal(t, StateDegraded, provider.GetState())
	})

	t.Run("circuit open provider is not available before timeout", func(t *testing.T) {
		provider.SetState(StateCircuitOpen)
		provider.circuitOpenUntil.Store(time.Now().Add(10 * time.Second).Unix())
		assert.False(t, provider.IsAvailable())
	})
}

func TestProvider_CalculateScore(t *testing.T) {
	client := &fasthttp.Client{}
	provider := NewProvider("test", "http://localhost:8080", 100, client)

	t.Run("unavailable provider has zero score", func(t *testing.T) {
		provider.SetState(StateUnhealthy)
		score := provider.CalculateScore()
		assert.Equal(t, 0.0, score)
	})

	t.Run("healthy provider with good metrics", func(t *testing.T) {
		provider.SetState(StateHealthy)
		for i := 0; i < 10; i++ {
			provider.metrics.RecordSuccess(100)
		}
		score := provider.CalculateScore()
		assert.Greater(t, score, 0.0)
	})

	t.Run("degraded provider has reduced score", func(t *testing.T) {
		provider.SetState(StateDegraded)
		for i := 0; i < 10; i++ {
			provider.metrics.RecordSuccess(100)
		}
		score := provider.CalculateScore()
		assert.Greater(t, score, 0.0)
		assert.Less(t, score, 100.0)
	})

	t.Run("consecutive failures reduce score", func(t *testing.T) {
		provider.SetState(StateHealthy)
		provider.metrics.ConsecutiveFails.Store(3)
		score := provider.CalculateScore()
		assert.Greater(t, score, 0.0)
		assert.Less(t, score, 100.0)
	})
}

func TestNewClient_Validation(t *testing.T) {
	t.Run("nil config returns error", func(t *testing.T) {
		client, err := NewClient(nil)
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "config is required")
	})

	t.Run("empty providers returns error", func(t *testing.T) {
		config := &Config{
			Providers: []ProviderConfig{},
			Timeout:   5 * time.Second,
		}
		client, err := NewClient(config)
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "at least one provider is required")
	})

	t.Run("valid config creates client", func(t *testing.T) {
		config := &Config{
			Providers: []ProviderConfig{
				{Name: "primary", URL: "http://localhost:8081", Weight: 100},
			},
			Timeout:                 5 * time.Second,
			MaxRetries:              3,
			RetryDelay:              time.Second,
			MaxConns:                100,
			ReadBufferSize:          4096,
			WriteBufferSize:         4096,
			HealthCheckInterval:     30 * time.Second,
			CircuitBreakerThreshold: 5,
			CircuitBreakerTimeout:   30 * time.Second,
		}
		client, err := NewClient(config)
		require.NoError(t, err)
		require.NotNil(t, client)
		assert.Len(t, client.providers, 1)

		client.Close()
	})
}

func TestClient_SelectBestProvider(t *testing.T) {
	config := &Config{
		Providers: []ProviderConfig{
			{Name: "primary", URL: "http://localhost:8081", Weight: 100},
			{Name: "secondary", URL: "http://localhost:8082", Weight: 80},
			{Name: "backup", URL: "http://localhost:8083", Weight: 60},
		},
		Timeout:                 5 * time.Second,
		MaxRetries:              3,
		MaxConns:                100,
		ReadBufferSize:          4096,
		WriteBufferSize:         4096,
		HealthCheckInterval:     30 * time.Second,
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   30 * time.Second,
	}

	client, err := NewClient(config)
	require.NoError(t, err)
	defer client.Close()

	t.Run("selects provider with highest score", func(t *testing.T) {
		provider, err := client.SelectBestProvider()
		assert.NoError(t, err)
		assert.NotNil(t, provider)
	})

	t.Run("returns error when all providers unavailable", func(t *testing.T) {
		for _, p := range client.providers {
			p.SetState(StateUnhealthy)
		}

		provider, err := client.SelectBestProvider()
		assert.Error(t, err)
		assert.Nil(t, provider)
		assert.Equal(t, ErrNoAvailableProviders, err)

		for _, p := range client.providers {
			p.SetState(StateHealthy)
		}
	})

	t.Run("skips unhealthy providers", func(t *testing.T) {
		client.providers[0].SetState(StateUnhealthy)

		provider, err := client.SelectBestProvider()
		assert.NoError(t, err)
		assert.NotNil(t, provider)
		assert.NotEqual(t, "primary", provider.name)

		client.providers[0].SetState(StateHealthy)
	})
}

func TestClient_CheckCircuitBreaker(t *testing.T) {
	config := &Config{
		Providers: []ProviderConfig{
			{Name: "test", URL: "http://localhost:8081", Weight: 100},
		},
		Timeout:                 5 * time.Second,
		CircuitBreakerThreshold: 3,
		CircuitBreakerTimeout:   10 * time.Second,
		MaxConns:                100,
		ReadBufferSize:          4096,
		WriteBufferSize:         4096,
		HealthCheckInterval:     30 * time.Second,
	}

	client, err := NewClient(config)
	require.NoError(t, err)
	defer client.Close()

	provider := client.providers[0]

	t.Run("opens circuit after threshold failures", func(t *testing.T) {
		provider.metrics.ConsecutiveFails.Store(3)
		client.checkCircuitBreaker(provider)

		assert.Equal(t, StateCircuitOpen, provider.GetState())
		assert.Greater(t, provider.circuitOpenUntil.Load(), time.Now().Unix())
	})

	t.Run("does not open circuit below threshold", func(t *testing.T) {
		provider.SetState(StateHealthy)
		provider.metrics.ConsecutiveFails.Store(2)
		client.checkCircuitBreaker(provider)

		assert.NotEqual(t, StateCircuitOpen, provider.GetState())
	})
}

func TestSendRequest_Validation(t *testing.T) {
	req := &SendRequest{
		MessageID:   "msg123",
		PhoneNumber: "+1234567890",
		Content:     "Test message",
		Priority:    "normal",
	}

	data, err := json.Marshal(req)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	var decoded SendRequest
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, req.MessageID, decoded.MessageID)
	assert.Equal(t, req.PhoneNumber, decoded.PhoneNumber)
}

func TestProviderStats_Sorting(t *testing.T) {
	config := &Config{
		Providers: []ProviderConfig{
			{Name: "p1", URL: "http://localhost:8081", Weight: 50},
			{Name: "p2", URL: "http://localhost:8082", Weight: 100},
			{Name: "p3", URL: "http://localhost:8083", Weight: 75},
		},
		Timeout:                 5 * time.Second,
		MaxConns:                100,
		ReadBufferSize:          4096,
		WriteBufferSize:         4096,
		HealthCheckInterval:     30 * time.Second,
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   30 * time.Second,
	}

	client, err := NewClient(config)
	require.NoError(t, err)
	defer client.Close()

	client.providers[1].metrics.RecordSuccess(100)
	client.providers[1].metrics.RecordSuccess(150)

	stats := client.GetProviderStats()
	assert.Len(t, stats, 3)
	assert.GreaterOrEqual(t, stats[0].Score, stats[1].Score)
	assert.GreaterOrEqual(t, stats[1].Score, stats[2].Score)
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state    ProviderState
		expected string
	}{
		{StateHealthy, "HEALTHY"},
		{StateDegraded, "DEGRADED"},
		{StateUnhealthy, "UNHEALTHY"},
		{StateCircuitOpen, "CIRCUIT_OPEN"},
		{ProviderState(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := stateString(tt.state)
			assert.Equal(t, tt.expected, result)
		})
	}
}
