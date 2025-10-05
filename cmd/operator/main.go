package main

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// DeliveryStatus represents the delivery status of a sms
type DeliveryStatus string

const (
	StatusDelivered DeliveryStatus = "DELIVERED"
	StatusFailed    DeliveryStatus = "FAILED"
	StatusPending   DeliveryStatus = "PENDING"
)

// SendSMSRequest represents the request to send an SMS
type SendSMSRequest struct {
	MessageID   string `json:"message_id" binding:"required"`
	PhoneNumber string `json:"phone_number" binding:"required"`
	Content     string `json:"content" binding:"required"`
	Priority    string `json:"priority"` // "express" or "normal"
}

// SendSMSResponse represents the response from sending an SMS
type SendSMSResponse struct {
	MessageID   string         `json:"message_id"`
	Status      DeliveryStatus `json:"status"`
	DeliveredAt *time.Time     `json:"delivered_at,omitempty"`
	ErrorCode   string         `json:"error_code,omitempty"`
	ErrorMsg    string         `json:"error_sms,omitempty"`
	OperatorID  string         `json:"operator_id"`
	ProcessedAt time.Time      `json:"processed_at"`
}

// BatchSendRequest represents a batch send request
type BatchSendRequest struct {
	Messages []SendSMSRequest `json:"smss" binding:"required"`
}

// StatusCheckResponse represents delivery status response
type StatusCheckResponse struct {
	MessageID   string         `json:"message_id"`
	Status      DeliveryStatus `json:"status"`
	DeliveredAt *time.Time     `json:"delivered_at,omitempty"`
	ErrorCode   string         `json:"error_code,omitempty"`
	ErrorMsg    string         `json:"error_sms,omitempty"`
	OperatorID  string         `json:"operator_id"`
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status       string    `json:"status"`
	OperatorID   string    `json:"operator_id"`
	Timestamp    time.Time `json:"timestamp"`
	DeliveryRate float64   `json:"delivery_rate"`
}

// MockOperator simulates an SMS operator service
type MockOperator struct {
	deliveryRate float64
	minDelay     time.Duration
	maxDelay     time.Duration
	operatorID   string
	rng          *rand.Rand
}

// NewMockOperator creates a new mock operator instance
func NewMockOperator(deliveryRate float64, minDelay, maxDelay time.Duration) *MockOperator {
	return &MockOperator{
		deliveryRate: deliveryRate,
		minDelay:     minDelay,
		maxDelay:     maxDelay,
		operatorID:   "MOCK_OPERATOR_" + uuid.New().String()[:8],
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// simulateDelivery simulates the sms delivery process
func (m *MockOperator) simulateDelivery(req *SendSMSRequest) *SendSMSResponse {
	// Calculate delay
	delay := m.randomDelay()

	// Express smss get 50% faster delivery
	if req.Priority == "express" {
		delay = delay / 2
	}

	// Simulate network delay
	time.Sleep(delay)

	response := &SendSMSResponse{
		MessageID:   req.MessageID,
		OperatorID:  m.operatorID,
		ProcessedAt: time.Now(),
	}

	// Determine success or failure
	if m.shouldSucceed() {
		now := time.Now()
		response.Status = StatusDelivered
		response.DeliveredAt = &now

		log.Info().
			Str("message_id", req.MessageID).
			Str("phone", req.PhoneNumber).
			Dur("delay", delay).
			Msg("SMS delivered successfully")
	} else {
		response.Status = StatusFailed
		response.ErrorCode = m.randomErrorCode()
		response.ErrorMsg = m.errorMessage(response.ErrorCode)

		log.Warn().
			Str("message_id", req.MessageID).
			Str("phone", req.PhoneNumber).
			Str("error_code", response.ErrorCode).
			Msg("SMS delivery failed")
	}

	return response
}

func (m *MockOperator) randomDelay() time.Duration {
	delta := m.maxDelay - m.minDelay
	randomDelta := time.Duration(m.rng.Int63n(int64(delta)))
	return m.minDelay + randomDelta
}

func (m *MockOperator) shouldSucceed() bool {
	return m.rng.Float64() < m.deliveryRate
}

func (m *MockOperator) randomErrorCode() string {
	errorCodes := []string{
		"INVALID_NUMBER",
		"NETWORK_ERROR",
		"TIMEOUT",
		"BLOCKED",
		"INVALID_CONTENT",
		"OPERATOR_REJECTED",
	}
	return errorCodes[m.rng.Intn(len(errorCodes))]
}

func (m *MockOperator) errorMessage(code string) string {
	smss := map[string]string{
		"INVALID_NUMBER":    "The phone number is invalid or not in service",
		"NETWORK_ERROR":     "Network connectivity issue with operator",
		"TIMEOUT":           "SMS delivery timed out",
		"BLOCKED":           "The recipient has blocked smss",
		"INVALID_CONTENT":   "SMS content violates operator policies",
		"OPERATOR_REJECTED": "Operator rejected the sms",
	}

	if msg, ok := smss[code]; ok {
		return msg
	}
	return "Unknown error occurred"
}

// Handler struct holds the mock operator and routes
type Handler struct {
	operator *MockOperator
}

func NewHandler(operator *MockOperator) *Handler {
	return &Handler{operator: operator}
}

// SendSMS handles single SMS send requests
func (h *Handler) SendSMS(c *gin.Context) {
	var req SendSMSRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request",
			"details": err.Error(),
		})
		return
	}

	log.Info().
		Str("message_id", req.MessageID).
		Str("phone", req.PhoneNumber).
		Str("priority", req.Priority).
		Msg("Received SMS send request")

	response := h.operator.simulateDelivery(&req)

	statusCode := http.StatusOK
	if response.Status == StatusFailed {
		statusCode = http.StatusAccepted // 202: accepted but failed delivery
	}

	c.JSON(statusCode, response)
}

// GetStatus handles delivery status check requests
func (h *Handler) GetStatus(c *gin.Context) {
	smsID := c.Param("message_id")

	if smsID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "message_id is required",
		})
		return
	}

	// Simulate API delay
	time.Sleep(100 * time.Millisecond)

	// For demo, return random status
	response := StatusCheckResponse{
		MessageID:  smsID,
		OperatorID: h.operator.operatorID,
	}

	if h.operator.shouldSucceed() {
		now := time.Now()
		response.Status = StatusDelivered
		response.DeliveredAt = &now
	} else {
		response.Status = StatusFailed
		response.ErrorCode = "TIMEOUT"
		response.ErrorMsg = "SMS delivery timed out"
	}

	c.JSON(http.StatusOK, response)
}

// HealthCheck handles health check requests
func (h *Handler) HealthCheck(c *gin.Context) {
	// Simulate 5% downtime
	if h.operator.rng.Float64() < 0.05 {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unavailable",
			"error":  "Operator temporarily unavailable",
		})
		return
	}

	c.JSON(http.StatusOK, HealthResponse{
		Status:       "healthy",
		OperatorID:   h.operator.operatorID,
		Timestamp:    time.Now(),
		DeliveryRate: h.operator.deliveryRate,
	})
}

// UpdateConfig allows changing operator configuration at runtime
func (h *Handler) UpdateConfig(c *gin.Context) {
	var config struct {
		DeliveryRate *float64 `json:"delivery_rate"`
	}

	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request",
			"details": err.Error(),
		})
		return
	}

	if config.DeliveryRate != nil {
		if *config.DeliveryRate >= 0 && *config.DeliveryRate <= 1.0 {
			h.operator.deliveryRate = *config.DeliveryRate
			log.Info().Float64("rate", *config.DeliveryRate).Msg("Updated delivery rate")
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"sms":           "Configuration updated",
		"delivery_rate": h.operator.deliveryRate,
	})
}

// SetupRouter configures all routes
func SetupRouter(handler *Handler) *gin.Engine {
	router := gin.Default()

	// Add request logging middleware
	router.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)

		log.Info().
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status", c.Writer.Status()).
			Dur("duration", duration).
			Msg("Request processed")
	})

	// API routes
	v1 := router.Group("/api/v1")
	{
		v1.POST("/sms/send", handler.SendSMS)
		v1.GET("/sms/status/:message_id", handler.GetStatus)
		v1.GET("/health", handler.HealthCheck)
		v1.PUT("/config", handler.UpdateConfig)
	}

	// Root health check
	router.GET("/health", handler.HealthCheck)

	return router
}

func main() {
	// Setup logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Get configuration from environment
	port := getEnv("PORT", "8081")
	deliveryRate := getEnvFloat("DELIVERY_RATE", 1)
	minDelay := getEnvDuration("MIN_DELAY", 1*time.Second)
	maxDelay := getEnvDuration("MAX_DELAY", 5*time.Second)

	log.Info().
		Str("port", port).
		Float64("delivery_rate", deliveryRate).
		Dur("min_delay", minDelay).
		Dur("max_delay", maxDelay).
		Msg("Starting Mock SMS Operator")

	// Create mock operator
	operator := NewMockOperator(deliveryRate, minDelay, maxDelay)
	handler := NewHandler(operator)
	router := SetupRouter(handler)

	// Setup HTTP server
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Info().Str("addr", srv.Addr).Msg("Server started")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exited")
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		var f float64
		if _, err := fmt.Sscanf(value, "%f", &f); err == nil {
			return f
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}
