package workers

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"rinha-backend-2025/internal/database"
	"rinha-backend-2025/internal/models"
	"rinha-backend-2025/internal/processors"
	"rinha-backend-2025/internal/redis"
)


type PaymentWorkerPool struct {
	workers          int
	processorService *processors.ProcessorService
	dbService        database.Service
	redisService     *redis.Service
	wg               sync.WaitGroup
	ctx              context.Context
	cancel           context.CancelFunc
}

func NewPaymentWorkerPool(workers int, processorService *processors.ProcessorService, dbService database.Service, redisService *redis.Service) *PaymentWorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &PaymentWorkerPool{
		workers:          workers,
		processorService: processorService,
		dbService:        dbService,
		redisService:     redisService,
		ctx:              ctx,
		cancel:           cancel,
	}
}

func (wp *PaymentWorkerPool) Start() {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}
	log.Printf("Started %d payment workers", wp.workers)
}

func (wp *PaymentWorkerPool) Stop() {
	wp.cancel()
	wp.wg.Wait()
	log.Println("Payment worker pool stopped")
}

func (wp *PaymentWorkerPool) worker(workerID int) {
	defer wp.wg.Done()
	
	for {
		select {
		case <-wp.ctx.Done():
			return
		default:
			// Try to consume a job from Redis
			job, err := wp.redisService.ConsumePaymentJob(wp.ctx)
			if err != nil {
				// If context is cancelled, exit
				if wp.ctx.Err() != nil {
					return
				}
				// For other errors (like timeout), continue to next iteration
				continue
			}
			
			// Process the job directly
			wp.processPayment(*job, workerID)
		}
	}
}

func (wp *PaymentWorkerPool) processPayment(job redis.PaymentJob, _ int) {
	ctx, cancel := context.WithTimeout(wp.ctx, 30*time.Second)
	defer cancel()

	// Parse UUIDs from strings
	paymentID, err := uuid.Parse(job.PaymentID)
	if err != nil {
		return
	}
	
	correlationID, parseErr := uuid.Parse(job.CorrelationID)
	if parseErr != nil {
		return
	}
	
	// Convert amount from cents to currency units
	amount := float64(job.Amount) / 100
	requestedAt := job.RequestedAt // Use consistent timestamp from original request

	if err := wp.dbService.UpdatePaymentStatus(ctx, paymentID, models.PaymentStatusProcessing); err != nil {
		return
	}

	// Simple single attempt - if fails, put back in queue
	_, processorType, err := wp.processorService.ProcessPaymentWithFallback(ctx, correlationID, amount, requestedAt)
	
	if err != nil {
		// Failed - put back in main queue for retry
		wp.redisService.RetryPaymentJob(ctx, &job)
		return
	}

	// Calculate fee based on processor type
	var fee float64
	if processorType == processors.ProcessorTypeDefault {
		fee = amount * 0.03 // 3% for default processor
	} else {
		fee = amount * 0.05 // 5% for fallback processor
	}

	// Complete payment
	processorTypeStr := string(processorType)
	wp.dbService.CompletePayment(ctx, paymentID, fee, processorTypeStr)
}

// RetryProcessor processes retry jobs and DLQ
type RetryProcessor struct {
	redisService *redis.Service
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewRetryProcessor creates a new retry processor
func NewRetryProcessor(redisService *redis.Service) *RetryProcessor {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &RetryProcessor{
		redisService: redisService,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start begins the retry processing goroutine
func (rp *RetryProcessor) Start() {
	rp.wg.Add(1)
	go rp.processRetries()
	log.Println("Retry processor started")
}

// Stop stops the retry processing goroutine
func (rp *RetryProcessor) Stop() {
	rp.cancel()
	rp.wg.Wait()
	log.Println("Retry processor stopped")
}

// processRetries is simplified - no complex retry logic needed
func (rp *RetryProcessor) processRetries() {
	defer rp.wg.Done()
	
	ticker := time.NewTicker(30 * time.Second) // Much less frequent since we use single queue
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// Just a health check - all retries go to main queue
			rp.redisService.ProcessRetryJobs(rp.ctx)
		case <-rp.ctx.Done():
			return
		}
	}
}

// No DLQ reprocessing needed with single queue approach