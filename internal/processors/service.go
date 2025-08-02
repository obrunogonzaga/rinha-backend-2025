package processors

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"rinha-backend-2025/internal/circuitbreaker"
	"rinha-backend-2025/internal/redis"
)

type ProcessorService struct {
	client            *Client
	circuitBreakers   *circuitbreaker.ProcessorCircuitBreakers
	redisService      *redis.Service
	healthCache       map[ProcessorType]bool
	healthCacheMutex  sync.RWMutex
	lastHealthCheck   map[ProcessorType]time.Time
	healthCheckCooldown time.Duration
}

func NewProcessorService(defaultURL, fallbackURL string, redisService *redis.Service) *ProcessorService {
	return &ProcessorService{
		client:              NewClient(defaultURL, fallbackURL),
		circuitBreakers:     circuitbreaker.NewProcessorCircuitBreakers(),
		redisService:        redisService,
		healthCache:         make(map[ProcessorType]bool),
		lastHealthCheck:     make(map[ProcessorType]time.Time),
		healthCheckCooldown: 5 * time.Second,
	}
}

func (ps *ProcessorService) ProcessPaymentWithFallback(ctx context.Context, correlationID uuid.UUID, amount float64, requestedAt time.Time) (*PaymentProcessorResponse, ProcessorType, error) {
	req := PaymentProcessorRequest{
		CorrelationID: correlationID,
		Amount:        amount,
		RequestedAt:   requestedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
	}

	// Try default processor first if circuit breaker allows
	if !ps.circuitBreakers.IsDefaultOpen() && ps.isProcessorHealthy(ctx, ProcessorTypeDefault) {
		result, err := ps.circuitBreakers.ProcessPaymentWithDefault(ctx, func(ctx context.Context) (interface{}, error) {
			return ps.client.ProcessPayment(ctx, req, ProcessorTypeDefault)
		})
		if err != nil {
			log.Printf("Failed to process payment with default processor (circuit breaker): %v", err)
		} else {
			resp := result.(*PaymentProcessorResponse)
			return resp, ProcessorTypeDefault, nil
		}
	} else {
		log.Printf("Default processor skipped - circuit breaker: %v, healthy: %v", 
			ps.circuitBreakers.IsDefaultOpen(), ps.isProcessorHealthy(ctx, ProcessorTypeDefault))
	}

	// Try fallback processor if circuit breaker allows
	if !ps.circuitBreakers.IsFallbackOpen() && ps.isProcessorHealthy(ctx, ProcessorTypeFallback) {
		result, err := ps.circuitBreakers.ProcessPaymentWithFallback(ctx, func(ctx context.Context) (interface{}, error) {
			return ps.client.ProcessPayment(ctx, req, ProcessorTypeFallback)
		})
		if err != nil {
			log.Printf("Failed to process payment with fallback processor (circuit breaker): %v", err)
		} else {
			resp := result.(*PaymentProcessorResponse)
			return resp, ProcessorTypeFallback, nil
		}
	} else {
		log.Printf("Fallback processor skipped - circuit breaker: %v, healthy: %v", 
			ps.circuitBreakers.IsFallbackOpen(), ps.isProcessorHealthy(ctx, ProcessorTypeFallback))
	}

	return nil, "", fmt.Errorf("all payment processors failed or circuit breakers are open")
}

// GetCircuitBreakerStates returns the current state of circuit breakers for monitoring
func (ps *ProcessorService) GetCircuitBreakerStates() (defaultState, fallbackState circuitbreaker.State) {
	return ps.circuitBreakers.GetDefaultState(), ps.circuitBreakers.GetFallbackState()
}

// GetCircuitBreakerCounts returns the current counts for circuit breakers for monitoring
func (ps *ProcessorService) GetCircuitBreakerCounts() (defaultCounts, fallbackCounts circuitbreaker.Counts) {
	return ps.circuitBreakers.GetDefaultCounts(), ps.circuitBreakers.GetFallbackCounts()
}

func (ps *ProcessorService) isProcessorHealthy(ctx context.Context, processorType ProcessorType) bool {
	// First try to get health status from Redis cache (managed by health monitor)
	if ps.redisService != nil {
		healthy, exists, err := ps.redisService.GetProcessorHealth(ctx, string(processorType))
		if err == nil && exists {
			return healthy
		}
		if err != nil {
			log.Printf("Failed to get health status from Redis for %s processor: %v", processorType, err)
		}
	}

	// Fallback to local cache if Redis is unavailable
	ps.healthCacheMutex.RLock()
	
	lastCheck, exists := ps.lastHealthCheck[processorType]
	if exists && time.Since(lastCheck) < ps.healthCheckCooldown {
		healthy := ps.healthCache[processorType]
		ps.healthCacheMutex.RUnlock()
		return healthy
	}
	
	ps.healthCacheMutex.RUnlock()

	// Last resort: perform health check directly
	healthy := ps.checkAndCacheHealth(ctx, processorType)
	return healthy
}

func (ps *ProcessorService) checkAndCacheHealth(ctx context.Context, processorType ProcessorType) bool {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	_, err := ps.client.CheckHealth(ctxWithTimeout, processorType)
	healthy := err == nil

	ps.healthCacheMutex.Lock()
	ps.healthCache[processorType] = healthy
	ps.lastHealthCheck[processorType] = time.Now()
	ps.healthCacheMutex.Unlock()

	if !healthy {
		log.Printf("Health check failed for %s processor: %v", processorType, err)
	}

	return healthy
}

func (ps *ProcessorService) markProcessorUnhealthy(processorType ProcessorType) {
	ps.healthCacheMutex.Lock()
	ps.healthCache[processorType] = false
	ps.lastHealthCheck[processorType] = time.Now()
	ps.healthCacheMutex.Unlock()
}