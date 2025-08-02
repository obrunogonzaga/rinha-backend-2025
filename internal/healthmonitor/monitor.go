package healthmonitor

import (
	"context"
	"log"
	"sync"
	"time"

	"rinha-backend-2025/internal/processors"
	"rinha-backend-2025/internal/redis"
)

// HealthMonitor continuously monitors processor health and caches results
type HealthMonitor struct {
	processorClient *processors.Client
	redisService    *redis.Service
	checkInterval   time.Duration
	healthTimeout   time.Duration
	wg              sync.WaitGroup
	ctx             context.Context
	cancel          context.CancelFunc
}

// Config holds health monitor configuration
type Config struct {
	CheckInterval time.Duration
	HealthTimeout time.Duration
}

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor(processorClient *processors.Client, redisService *redis.Service, config Config) *HealthMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	if config.CheckInterval == 0 {
		config.CheckInterval = 5 * time.Second
	}

	if config.HealthTimeout == 0 {
		config.HealthTimeout = 3 * time.Second
	}

	return &HealthMonitor{
		processorClient: processorClient,
		redisService:    redisService,
		checkInterval:   config.CheckInterval,
		healthTimeout:   config.HealthTimeout,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start begins the health monitoring goroutine
func (hm *HealthMonitor) Start() {
	hm.wg.Add(1)
	go hm.monitor()
	log.Println("Health monitor started")
}

// Stop stops the health monitoring goroutine
func (hm *HealthMonitor) Stop() {
	hm.cancel()
	hm.wg.Wait()
	log.Println("Health monitor stopped")
}

// monitor is the main monitoring loop
func (hm *HealthMonitor) monitor() {
	defer hm.wg.Done()

	// Perform initial health checks immediately
	hm.checkAllProcessors()

	ticker := time.NewTicker(hm.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hm.checkAllProcessors()
		case <-hm.ctx.Done():
			log.Println("Health monitor goroutine stopping")
			return
		}
	}
}

// checkAllProcessors checks health of all processors and caches results
func (hm *HealthMonitor) checkAllProcessors() {
	processors := []processors.ProcessorType{
		processors.ProcessorTypeDefault,
		processors.ProcessorTypeFallback,
	}

	for _, processorType := range processors {
		hm.checkProcessor(processorType)
	}
}

// checkProcessor checks health of a specific processor and caches the result
func (hm *HealthMonitor) checkProcessor(processorType processors.ProcessorType) {
	ctx, cancel := context.WithTimeout(hm.ctx, hm.healthTimeout)
	defer cancel()

	start := time.Now()
	_, err := hm.processorClient.CheckHealth(ctx, processorType)
	duration := time.Since(start)

	isHealthy := err == nil

	// Cache the health status in Redis
	cacheCtx, cacheCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cacheCancel()

	if cacheErr := hm.redisService.CacheProcessorHealth(cacheCtx, string(processorType), isHealthy); cacheErr != nil {
		log.Printf("Failed to cache health status for %s processor: %v", processorType, cacheErr)
	}

	// Log health check results
	if isHealthy {
		log.Printf("Health check OK for %s processor (%.2fms)", processorType, float64(duration.Nanoseconds())/1e6)
	} else {
		log.Printf("Health check FAILED for %s processor (%.2fms): %v", processorType, float64(duration.Nanoseconds())/1e6, err)
	}
}

// GetProcessorHealth retrieves cached health status from Redis
func (hm *HealthMonitor) GetProcessorHealth(ctx context.Context, processorType processors.ProcessorType) (bool, bool, error) {
	return hm.redisService.GetProcessorHealth(ctx, string(processorType))
}

// InvalidateProcessorHealth removes cached health status for a processor
func (hm *HealthMonitor) InvalidateProcessorHealth(ctx context.Context, processorType processors.ProcessorType) error {
	return hm.redisService.InvalidateProcessorHealth(ctx, string(processorType))
}

// ForceHealthCheck immediately checks health of a specific processor
func (hm *HealthMonitor) ForceHealthCheck(processorType processors.ProcessorType) {
	go hm.checkProcessor(processorType)
}