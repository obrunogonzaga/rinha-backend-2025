package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"rinha-backend-2025/internal/models"
)

const (
	// Storage keys
	PaymentPrefix    = "payment:"
	PaymentsByStatus = "payments:status:"
	PaymentsSummary  = "payments:summary"
	
	// Aggregation keys for summary
	SummaryDefault  = "summary:default"
	SummaryFallback = "summary:fallback"
)

// StorageService provides Redis-based payment storage
type StorageService struct {
	client *Client
}

// NewStorageService creates a new Redis storage service
func NewStorageService(client *Client) *StorageService {
	return &StorageService{
		client: client,
	}
}

// CreatePayment stores a payment in Redis with instant response
func (s *StorageService) CreatePayment(ctx context.Context, payment *models.Payment) error {
	// Generate ID if not set
	if payment.ID == uuid.Nil {
		payment.ID = uuid.New()
	}
	
	// Set timestamps consistently
	now := time.Now().UTC()
	payment.CreatedAt = now
	payment.UpdatedAt = now
	
	// Serialize payment
	data, err := json.Marshal(payment)
	if err != nil {
		return fmt.Errorf("failed to marshal payment: %w", err)
	}
	
	// Store payment data
	paymentKey := PaymentPrefix + payment.ID.String()
	if err := s.client.SetWithExpiration(ctx, paymentKey, data, 24*time.Hour); err != nil {
		return err
	}
	
	// Add to status index for fast filtering
	statusKey := PaymentsByStatus + string(payment.Status)
	score := float64(payment.CreatedAt.Unix())
	
	return s.client.rdb.ZAdd(ctx, statusKey, redis.Z{
		Score:  score,
		Member: payment.ID.String(),
	}).Err()
}

// UpdatePaymentStatus updates payment status atomically
func (s *StorageService) UpdatePaymentStatus(ctx context.Context, paymentID uuid.UUID, status models.PaymentStatus) error {
	paymentKey := PaymentPrefix + paymentID.String()
	
	// Get current payment
	data, err := s.client.Get(ctx, paymentKey)
	if err != nil {
		return fmt.Errorf("payment not found: %w", err)
	}
	
	var payment models.Payment
	if err := json.Unmarshal([]byte(data), &payment); err != nil {
		return err
	}
	
	// Remove from old status index
	oldStatusKey := PaymentsByStatus + string(payment.Status)
	s.client.rdb.ZRem(ctx, oldStatusKey, paymentID.String())
	
	// Update payment
	payment.Status = status
	payment.UpdatedAt = time.Now().UTC()
	
	// Save updated payment
	newData, err := json.Marshal(payment)
	if err != nil {
		return err
	}
	
	if err := s.client.SetWithExpiration(ctx, paymentKey, newData, 24*time.Hour); err != nil {
		return err
	}
	
	// Add to new status index
	newStatusKey := PaymentsByStatus + string(status)
	score := float64(payment.CreatedAt.Unix())
	
	return s.client.rdb.ZAdd(ctx, newStatusKey, redis.Z{
		Score:  score,
		Member: paymentID.String(),
	}).Err()
}

// CompletePayment marks payment as completed and updates aggregates atomically (idempotent)
func (s *StorageService) CompletePayment(ctx context.Context, paymentID uuid.UUID, fee float64, processorType string) error {
	paymentKey := PaymentPrefix + paymentID.String()
	
	// Get current payment
	data, err := s.client.Get(ctx, paymentKey)
	if err != nil {
		return fmt.Errorf("payment not found: %w", err)
	}
	
	var payment models.Payment
	if err := json.Unmarshal([]byte(data), &payment); err != nil {
		return err
	}
	
	// IDEMPOTENCY CHECK: If already completed, don't re-aggregate
	if payment.Status == models.PaymentStatusCompleted {
		return nil // Already completed, no changes needed
	}
	
	// Update payment completion data
	now := time.Now().UTC()
	payment.Status = models.PaymentStatusCompleted
	payment.Fee = &fee
	payment.ProcessorType = &processorType
	payment.ProcessedAt = &now
	payment.UpdatedAt = now
	
	// Use Redis Lua script for true atomicity to prevent race conditions
	luaScript := `
		local payment_key = KEYS[1]
		local payment_data = ARGV[1]
		local payment_id = ARGV[2]
		local summary_key = ARGV[3]
		local amount = tonumber(ARGV[4])
		local processing_key = KEYS[2]
		local completed_key = KEYS[3]
		local score = tonumber(ARGV[5])
		local completion_flag = KEYS[4]
		
		-- Check if already completed using a flag
		if redis.call('EXISTS', completion_flag) == 1 then
			return 'already_completed'
		end
		
		-- Atomic update: payment data, status indexes, aggregation, and completion flag
		redis.call('SET', payment_key, payment_data, 'EX', 86400)
		redis.call('SET', completion_flag, '1', 'EX', 86400)
		redis.call('ZREM', processing_key, payment_id)
		redis.call('ZADD', completed_key, score, payment_id)
		redis.call('HINCRBY', summary_key, 'total_requests', 1)
		redis.call('HINCRBYFLOAT', summary_key, 'total_amount', amount)
		
		return 'success'
	`
	
	// Prepare script arguments
	paymentData, _ := json.Marshal(payment)
	processingKey := PaymentsByStatus + string(models.PaymentStatusProcessing)
	completedKey := PaymentsByStatus + string(models.PaymentStatusCompleted)
	summaryKey := SummaryDefault
	if processorType == "fallback" {
		summaryKey = SummaryFallback
	}
	score := float64(payment.CreatedAt.Unix())
	
	// Execute atomic Lua script
	completionFlag := "completed:" + paymentID.String()
	result, err := s.client.rdb.Eval(ctx, luaScript, []string{
		paymentKey,
		processingKey, 
		completedKey,
		completionFlag,
	}, []interface{}{
		string(paymentData),
		paymentID.String(),
		summaryKey,
		payment.Amount,
		score,
	}).Result()
	
	if err != nil {
		return err
	}
	
	if result == "already_completed" {
		return nil // Idempotent - already processed
	}
	
	return nil
}

// GetPaymentSummary returns ultra-fast aggregated summary from Redis
func (s *StorageService) GetPaymentSummary(ctx context.Context, startDate, endDate *time.Time) (models.PaymentSummaryResponse, error) {
	// Use pre-computed aggregates for instant response
	defaultData, err := s.client.rdb.HGetAll(ctx, SummaryDefault).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}
	
	fallbackData, err := s.client.rdb.HGetAll(ctx, SummaryFallback).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}
	
	result := make(models.PaymentSummaryResponse)
	
	// Parse default processor data
	defaultRequests, _ := strconv.Atoi(defaultData["total_requests"])
	defaultAmount, _ := strconv.ParseFloat(defaultData["total_amount"], 64)
	
	result["default"] = models.ProcessorSummary{
		TotalRequests: defaultRequests,
		TotalAmount:   defaultAmount,
	}
	
	// Parse fallback processor data
	fallbackRequests, _ := strconv.Atoi(fallbackData["total_requests"])
	fallbackAmount, _ := strconv.ParseFloat(fallbackData["total_amount"], 64)
	
	result["fallback"] = models.ProcessorSummary{
		TotalRequests: fallbackRequests,
		TotalAmount:   fallbackAmount,
	}
	
	return result, nil
}

// GetPayment retrieves a payment by ID
func (s *StorageService) GetPayment(ctx context.Context, paymentID uuid.UUID) (*models.Payment, error) {
	paymentKey := PaymentPrefix + paymentID.String()
	
	data, err := s.client.Get(ctx, paymentKey)
	if err != nil {
		return nil, fmt.Errorf("payment not found: %w", err)
	}
	
	var payment models.Payment
	if err := json.Unmarshal([]byte(data), &payment); err != nil {
		return nil, err
	}
	
	return &payment, nil
}

// ClearPayments removes all payment data (for testing)
func (s *StorageService) ClearPayments(ctx context.Context) error {
	pipe := s.client.rdb.TxPipeline()
	
	// Clear all payment keys
	keys, err := s.client.rdb.Keys(ctx, PaymentPrefix+"*").Result()
	if err != nil {
		return err
	}
	
	if len(keys) > 0 {
		pipe.Del(ctx, keys...)
	}
	
	// Clear completion flags
	completionKeys, err := s.client.rdb.Keys(ctx, "completed:*").Result()
	if err == nil && len(completionKeys) > 0 {
		pipe.Del(ctx, completionKeys...)
	}
	
	// Clear status indexes
	pipe.Del(ctx, PaymentsByStatus+string(models.PaymentStatusPending))
	pipe.Del(ctx, PaymentsByStatus+string(models.PaymentStatusProcessing))
	pipe.Del(ctx, PaymentsByStatus+string(models.PaymentStatusCompleted))
	pipe.Del(ctx, PaymentsByStatus+string(models.PaymentStatusFailed))
	
	// Clear aggregates
	pipe.Del(ctx, SummaryDefault)
	pipe.Del(ctx, SummaryFallback)
	
	_, err = pipe.Exec(ctx)
	return err
}

// Health returns storage health status
func (s *StorageService) Health() map[string]string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	if err := s.client.Ping(ctx); err != nil {
		return map[string]string{
			"status": "down",
			"error":  err.Error(),
		}
	}
	
	return map[string]string{
		"status": "up",
		"type":   "redis",
	}
}

// Close terminates the Redis connection
func (s *StorageService) Close() error {
	return s.client.Close()
}