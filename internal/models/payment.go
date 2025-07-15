package models

import (
	"time"
	"github.com/google/uuid"
)

type PaymentStatus string

const (
	PaymentStatusPending    PaymentStatus = "pending"
	PaymentStatusProcessing PaymentStatus = "processing"
	PaymentStatusCompleted  PaymentStatus = "completed"
	PaymentStatusFailed     PaymentStatus = "failed"
)

type Payment struct {
	ID            uuid.UUID     `json:"id" db:"id"`
	CorrelationID uuid.UUID     `json:"correlationId" db:"correlation_id"`
	Amount        float64       `json:"amount" db:"amount"`
	Fee           *float64      `json:"fee,omitempty" db:"fee"`
	ProcessorType *string       `json:"processorType,omitempty" db:"processor_type"`
	Status        PaymentStatus `json:"status" db:"status"`
	RequestedAt   time.Time     `json:"requestedAt" db:"requested_at"`
	ProcessedAt   *time.Time    `json:"processedAt,omitempty" db:"processed_at"`
	CreatedAt     time.Time     `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time     `json:"updatedAt" db:"updated_at"`
}

type PaymentRequest struct {
	CorrelationID uuid.UUID `json:"correlationId" validate:"required"`
	Amount        float64   `json:"amount" validate:"required,gt=0"`
}

type PaymentResponse struct {
	Message string `json:"message"`
}

type PaymentSummary struct {
	ProcessorType string  `json:"processorType" db:"processor_type"`
	TotalAmount   float64 `json:"totalAmount" db:"total_amount"`
	TotalFee      float64 `json:"totalFee" db:"total_fee"`
	Count         int     `json:"count" db:"count"`
}

type PaymentSummaryResponse struct {
	Summary []PaymentSummary `json:"summary"`
}