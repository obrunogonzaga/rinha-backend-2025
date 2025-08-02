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
	
	correlationID, err := uuid.Parse(job.CorrelationID)
	if err != nil {
		return
	}
	
	// Convert amount from cents to currency units
	amount := float64(job.Amount) / 100
	requestedAt := time.Now() // Use current time since it's not stored in Redis job

	if err := wp.dbService.UpdatePaymentStatus(ctx, paymentID, models.PaymentStatusProcessing); err != nil {
		return
	}

	_, processorType, err := wp.processorService.ProcessPaymentWithFallback(ctx, correlationID, amount, requestedAt)
	
	if err != nil {
		// Schedule for retry instead of marking as failed
		if retryErr := wp.redisService.RetryPaymentJob(ctx, &job); retryErr != nil {
			// Only fail if we can't even schedule retry
			wp.dbService.UpdatePaymentStatus(ctx, paymentID, models.PaymentStatusFailed)
		}
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

// processRetries continuously processes retry jobs
func (rp *RetryProcessor) processRetries() {
	defer rp.wg.Done()
	
	ticker := time.NewTicker(10 * time.Second) // Check every 10 seconds
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			if err := rp.redisService.ProcessRetryJobs(rp.ctx); err != nil {
				log.Printf("Retry processor failed to process retry jobs: %v", err)
			}
		case <-rp.ctx.Done():
			log.Println("Retry processor goroutine stopping")
			return
		}
	}
}