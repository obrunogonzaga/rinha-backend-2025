package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"rinha-backend-2025/internal/models"
)

const (
	// Queue names
	PaymentQueue    = "payments:queue"
	PaymentDLQ      = "payments:dlq"
	PaymentRetrySet = "payments:retry"
	
	// Health cache keys
	HealthKeyPrefix = "health:"
	
	// Default timeouts
	DefaultConsumeTimeout = 10 * time.Second
	DefaultHealthTTL      = 30 * time.Second
	
	// Retry settings
	MaxRetries = 3
	BaseRetryDelay = 30 * time.Second
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

// PaymentJob represents a payment job in the queue
type PaymentJob struct {
	PaymentID     string    `json:"payment_id"`
	CorrelationID string    `json:"correlation_id"`
	Amount        int64     `json:"amount"`
	RetryCount    int       `json:"retry_count"`
	LastAttempt   time.Time `json:"last_attempt"`
	NextRetry     time.Time `json:"next_retry"`
}

// PublishPaymentJob publishes a payment job to the queue
func (s *Service) PublishPaymentJob(ctx context.Context, payment *models.Payment) error {
	job := PaymentJob{
		PaymentID:     payment.ID.String(),
		CorrelationID: payment.CorrelationID.String(),
		Amount:        int64(payment.Amount * 100), // Convert to cents
		RetryCount:    0,
		LastAttempt:   time.Now(),
		NextRetry:     time.Now(),
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

// RetryPaymentJob schedules a job for retry with exponential backoff
func (s *Service) RetryPaymentJob(ctx context.Context, job *PaymentJob) error {
	job.RetryCount++
	job.LastAttempt = time.Now()
	
	if job.RetryCount > MaxRetries {
		// Move to Dead Letter Queue
		return s.client.PublishJob(ctx, PaymentDLQ, job)
	}
	
	// Calculate next retry time with exponential backoff
	backoffDuration := BaseRetryDelay * time.Duration(1<<uint(job.RetryCount-1)) // 30s, 60s, 120s
	job.NextRetry = time.Now().Add(backoffDuration)
	
	// Use sorted set for delayed retry
	score := float64(job.NextRetry.Unix())
	jsonData, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal retry job: %w", err)
	}
	
	return s.client.rdb.ZAdd(ctx, PaymentRetrySet, redis.Z{
		Score:  score,
		Member: string(jsonData),
	}).Err()
}

// ProcessRetryJobs moves ready retry jobs back to main queue
func (s *Service) ProcessRetryJobs(ctx context.Context) error {
	now := float64(time.Now().Unix())
	
	// Get jobs ready for retry
	result, err := s.client.rdb.ZRangeByScore(ctx, PaymentRetrySet, &redis.ZRangeBy{
		Min: "0",
		Max: fmt.Sprintf("%f", now),
	}).Result()
	
	if err != nil {
		return err
	}
	
	for _, jobStr := range result {
		var job PaymentJob
		if err := json.Unmarshal([]byte(jobStr), &job); err != nil {
			continue
		}
		
		// Move back to main queue
		if err := s.client.PublishJob(ctx, PaymentQueue, job); err != nil {
			continue
		}
		
		// Remove from retry set
		s.client.rdb.ZRem(ctx, PaymentRetrySet, jobStr)
	}
	
	return nil
}

// ConsumeDLQJob consumes a job from Dead Letter Queue
func (s *Service) ConsumeDLQJob(ctx context.Context) (*PaymentJob, error) {
	data, err := s.client.ConsumeJob(ctx, PaymentDLQ, DefaultConsumeTimeout)
	if err != nil {
		return nil, err
	}

	var job PaymentJob
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal DLQ job: %w", err)
	}

	return &job, nil
}

// GetDLQLength returns the number of jobs in Dead Letter Queue
func (s *Service) GetDLQLength(ctx context.Context) (int64, error) {
	return s.client.QueueLength(ctx, PaymentDLQ)
}

// GetRetrySetLength returns the number of jobs in retry set
func (s *Service) GetRetrySetLength(ctx context.Context) (int64, error) {
	return s.client.rdb.ZCard(ctx, PaymentRetrySet).Result()
}

// Close closes the Redis connection
func (s *Service) Close() error {
	return s.client.Close()
}