package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"rinha-backend-2025/internal/models"
)

const (
	// Single queue for simplicity
	PaymentQueue = "payments:queue"
	
	// Health cache keys
	HealthKeyPrefix = "health:"
	
	// Default timeouts
	DefaultConsumeTimeout = 10 * time.Second
	DefaultHealthTTL      = 30 * time.Second
)

// Service provides Redis operations for the application
type Service struct {
	client *Client
}

// NewService creates a new Redis service
func NewService(client *Client) *Service {
	return &Service{
		client: client,
	}
}

// PaymentJob represents a payment job in the queue - simplified
type PaymentJob struct {
	PaymentID     string    `json:"payment_id"`
	CorrelationID string    `json:"correlation_id"`
	Amount        int64     `json:"amount"`
	RequestedAt   time.Time `json:"requested_at"`
}

// PublishPaymentJob publishes a payment job to the single queue
func (s *Service) PublishPaymentJob(ctx context.Context, payment *models.Payment) error {
	job := PaymentJob{
		PaymentID:     payment.ID.String(),
		CorrelationID: payment.CorrelationID.String(),
		Amount:        int64(payment.Amount * 100), // Convert to cents
		RequestedAt:   payment.RequestedAt,        // Use same timestamp as stored payment
	}

	return s.client.PublishJob(ctx, PaymentQueue, job)
}

// ConsumePaymentJob consumes a payment job from the queue
func (s *Service) ConsumePaymentJob(ctx context.Context) (*PaymentJob, error) {
	data, err := s.client.ConsumeJob(ctx, PaymentQueue, DefaultConsumeTimeout)
	if err != nil {
		return nil, err
	}

	var job PaymentJob
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payment job: %w", err)
	}

	return &job, nil
}

// GetPaymentQueueLength returns the number of pending payment jobs
func (s *Service) GetPaymentQueueLength(ctx context.Context) (int64, error) {
	return s.client.QueueLength(ctx, PaymentQueue)
}

// CacheProcessorHealth caches processor health status
func (s *Service) CacheProcessorHealth(ctx context.Context, processorType string, isHealthy bool) error {
	key := HealthKeyPrefix + processorType
	value := "unhealthy"
	if isHealthy {
		value = "healthy"
	}
	
	return s.client.SetWithExpiration(ctx, key, value, DefaultHealthTTL)
}

// GetProcessorHealth retrieves cached processor health status
func (s *Service) GetProcessorHealth(ctx context.Context, processorType string) (bool, bool, error) {
	key := HealthKeyPrefix + processorType
	
	exists, err := s.client.Exists(ctx, key)
	if err != nil {
		return false, false, err
	}
	
	if !exists {
		return false, false, nil
	}
	
	value, err := s.client.Get(ctx, key)
	if err != nil {
		return false, false, err
	}
	
	isHealthy := value == "healthy"
	return isHealthy, true, nil
}

// InvalidateProcessorHealth removes cached health status
func (s *Service) InvalidateProcessorHealth(ctx context.Context, processorType string) error {
	key := HealthKeyPrefix + processorType
	return s.client.Delete(ctx, key)
}

// Ping checks Redis connectivity
func (s *Service) Ping(ctx context.Context) error {
	return s.client.Ping(ctx)
}

// RetryPaymentJob simply puts failed payment back to main queue
func (s *Service) RetryPaymentJob(ctx context.Context, job *PaymentJob) error {
	// Simple retry: just put it back at the end of the main queue
	return s.client.PublishJob(ctx, PaymentQueue, job)
}

// ProcessRetryJobs is now a no-op since we use single queue
func (s *Service) ProcessRetryJobs(ctx context.Context) error {
	// No longer needed - everything goes to main queue
	return nil
}

// DLQ methods removed - using single queue only

// Close closes the Redis connection
func (s *Service) Close() error {
	return s.client.Close()
}