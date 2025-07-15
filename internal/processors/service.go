package processors

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

type ProcessorService struct {
	client            *Client
	healthCache       map[ProcessorType]bool
	healthCacheMutex  sync.RWMutex
	lastHealthCheck   map[ProcessorType]time.Time
	healthCheckCooldown time.Duration
}

func NewProcessorService(defaultURL, fallbackURL string) *ProcessorService {
	return &ProcessorService{
		client:              NewClient(defaultURL, fallbackURL),
		healthCache:         make(map[ProcessorType]bool),
		lastHealthCheck:     make(map[ProcessorType]time.Time),
		healthCheckCooldown: 5 * time.Second,
	}
}

func (ps *ProcessorService) ProcessPaymentWithFallback(ctx context.Context, correlationID uuid.UUID, amount float64) (*PaymentProcessorResponse, ProcessorType, error) {
	req := PaymentProcessorRequest{
		CorrelationID: correlationID,
		Amount:        amount,
	}

	processorOrder := []ProcessorType{ProcessorTypeDefault, ProcessorTypeFallback}
	
	for _, processorType := range processorOrder {
		if !ps.isProcessorHealthy(ctx, processorType) {
			log.Printf("Processor %s is not healthy, skipping", processorType)
			continue
		}

		resp, err := ps.processPaymentWithRetry(ctx, req, processorType)
		if err != nil {
			log.Printf("Failed to process payment with %s processor: %v", processorType, err)
			ps.markProcessorUnhealthy(processorType)
			continue
		}

		return resp, processorType, nil
	}

	return nil, "", fmt.Errorf("all payment processors failed")
}

func (ps *ProcessorService) processPaymentWithRetry(ctx context.Context, req PaymentProcessorRequest, processorType ProcessorType) (*PaymentProcessorResponse, error) {
	maxRetries := 3
	baseDelay := 100 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * baseDelay
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		resp, err := ps.client.ProcessPayment(ctx, req, processorType)
		if err != nil {
			log.Printf("Payment attempt %d failed for %s processor: %v", attempt+1, processorType, err)
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("payment failed after %d attempts with %s processor", maxRetries, processorType)
}

func (ps *ProcessorService) isProcessorHealthy(ctx context.Context, processorType ProcessorType) bool {
	ps.healthCacheMutex.RLock()
	
	lastCheck, exists := ps.lastHealthCheck[processorType]
	if exists && time.Since(lastCheck) < ps.healthCheckCooldown {
		healthy := ps.healthCache[processorType]
		ps.healthCacheMutex.RUnlock()
		return healthy
	}
	
	ps.healthCacheMutex.RUnlock()

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