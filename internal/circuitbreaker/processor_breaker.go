package circuitbreaker

import (
	"context"
	"time"
)

// ProcessorCircuitBreakers manages circuit breakers for payment processors
type ProcessorCircuitBreakers struct {
	defaultBreaker  *CircuitBreaker
	fallbackBreaker *CircuitBreaker
}

// NewProcessorCircuitBreakers creates circuit breakers for both processors
func NewProcessorCircuitBreakers() *ProcessorCircuitBreakers {
	// Configuration for default processor
	defaultConfig := Config{
		MaxRequests: 3,
		Interval:    10 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts Counts) bool {
			// Trip if failure rate is above 60% with at least 5 requests
			if counts.Requests >= 5 {
				failureRate := float64(counts.TotalFailures) / float64(counts.Requests)
				return failureRate >= 0.6
			}
			// Or if we have 3 consecutive failures
			return counts.ConsecutiveFailures >= 3
		},
	}

	// Configuration for fallback processor (more tolerant)
	fallbackConfig := Config{
		MaxRequests: 5,
		Interval:    15 * time.Second,
		Timeout:     45 * time.Second,
		ReadyToTrip: func(counts Counts) bool {
			// Trip if failure rate is above 80% with at least 5 requests
			if counts.Requests >= 5 {
				failureRate := float64(counts.TotalFailures) / float64(counts.Requests)
				return failureRate >= 0.8
			}
			// Or if we have 5 consecutive failures
			return counts.ConsecutiveFailures >= 5
		},
	}

	return &ProcessorCircuitBreakers{
		defaultBreaker:  NewCircuitBreaker("default-processor", defaultConfig),
		fallbackBreaker: NewCircuitBreaker("fallback-processor", fallbackConfig),
	}
}

// ProcessorCallFunc represents a function that calls a processor
type ProcessorCallFunc func(ctx context.Context) (interface{}, error)

// ProcessPaymentWithDefault processes payment using the default processor with circuit breaker
func (pcb *ProcessorCircuitBreakers) ProcessPaymentWithDefault(
	ctx context.Context,
	callFunc ProcessorCallFunc,
) (interface{}, error) {
	return pcb.defaultBreaker.Execute(ctx, callFunc)
}

// ProcessPaymentWithFallback processes payment using the fallback processor with circuit breaker
func (pcb *ProcessorCircuitBreakers) ProcessPaymentWithFallback(
	ctx context.Context,
	callFunc ProcessorCallFunc,
) (interface{}, error) {
	return pcb.fallbackBreaker.Execute(ctx, callFunc)
}

// IsDefaultOpen returns true if the default processor circuit breaker is open
func (pcb *ProcessorCircuitBreakers) IsDefaultOpen() bool {
	return pcb.defaultBreaker.State() == StateOpen
}

// IsFallbackOpen returns true if the fallback processor circuit breaker is open
func (pcb *ProcessorCircuitBreakers) IsFallbackOpen() bool {
	return pcb.fallbackBreaker.State() == StateOpen
}

// GetDefaultState returns the state of the default processor circuit breaker
func (pcb *ProcessorCircuitBreakers) GetDefaultState() State {
	return pcb.defaultBreaker.State()
}

// GetFallbackState returns the state of the fallback processor circuit breaker
func (pcb *ProcessorCircuitBreakers) GetFallbackState() State {
	return pcb.fallbackBreaker.State()
}

// GetDefaultCounts returns the counts for the default processor circuit breaker
func (pcb *ProcessorCircuitBreakers) GetDefaultCounts() Counts {
	return pcb.defaultBreaker.Counts()
}

// GetFallbackCounts returns the counts for the fallback processor circuit breaker
func (pcb *ProcessorCircuitBreakers) GetFallbackCounts() Counts {
	return pcb.fallbackBreaker.Counts()
}