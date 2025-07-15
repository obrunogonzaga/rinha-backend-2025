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
)

type PaymentJob struct {
	PaymentID     uuid.UUID
	CorrelationID uuid.UUID
	Amount        float64
}

type PaymentWorkerPool struct {
	jobQueue         chan PaymentJob
	workers          int
	processorService *processors.ProcessorService
	dbService        database.Service
	wg               sync.WaitGroup
	ctx              context.Context
	cancel           context.CancelFunc
}

func NewPaymentWorkerPool(workers int, queueSize int, processorService *processors.ProcessorService, dbService database.Service) *PaymentWorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &PaymentWorkerPool{
		jobQueue:         make(chan PaymentJob, queueSize),
		workers:          workers,
		processorService: processorService,
		dbService:        dbService,
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
	close(wp.jobQueue)
	wp.cancel()
	wp.wg.Wait()
	log.Println("Payment worker pool stopped")
}

func (wp *PaymentWorkerPool) SubmitPayment(paymentID, correlationID uuid.UUID, amount float64) error {
	job := PaymentJob{
		PaymentID:     paymentID,
		CorrelationID: correlationID,
		Amount:        amount,
	}

	select {
	case wp.jobQueue <- job:
		return nil
	case <-wp.ctx.Done():
		return wp.ctx.Err()
	default:
		return nil
	}
}

func (wp *PaymentWorkerPool) worker(workerID int) {
	defer wp.wg.Done()
	
	log.Printf("Payment worker %d started", workerID)
	
	for {
		select {
		case job, ok := <-wp.jobQueue:
			if !ok {
				log.Printf("Payment worker %d stopped - job queue closed", workerID)
				return
			}
			wp.processPayment(job, workerID)
			
		case <-wp.ctx.Done():
			log.Printf("Payment worker %d stopped - context cancelled", workerID)
			return
		}
	}
}

func (wp *PaymentWorkerPool) processPayment(job PaymentJob, workerID int) {
	log.Printf("Worker %d processing payment %s", workerID, job.PaymentID)
	
	ctx, cancel := context.WithTimeout(wp.ctx, 30*time.Second)
	defer cancel()

	if err := wp.dbService.UpdatePaymentStatus(ctx, job.PaymentID, models.PaymentStatusProcessing); err != nil {
		log.Printf("Worker %d failed to update payment %s to processing: %v", workerID, job.PaymentID, err)
		return
	}

	resp, processorType, err := wp.processorService.ProcessPaymentWithFallback(ctx, job.CorrelationID, job.Amount)
	if err != nil {
		log.Printf("Worker %d failed to process payment %s: %v", workerID, job.PaymentID, err)
		
		if updateErr := wp.dbService.UpdatePaymentStatus(ctx, job.PaymentID, models.PaymentStatusFailed); updateErr != nil {
			log.Printf("Worker %d failed to update payment %s to failed: %v", workerID, job.PaymentID, updateErr)
		}
		return
	}

	processorTypeStr := string(processorType)
	if err := wp.dbService.CompletePayment(ctx, job.PaymentID, resp.Fee, processorTypeStr); err != nil {
		log.Printf("Worker %d failed to complete payment %s: %v", workerID, job.PaymentID, err)
		return
	}

	log.Printf("Worker %d successfully processed payment %s using %s processor (fee: %.2f)", 
		workerID, job.PaymentID, processorType, resp.Fee)
}