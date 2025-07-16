package processors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type ProcessorType string

const (
	ProcessorTypeDefault  ProcessorType = "default"
	ProcessorTypeFallback ProcessorType = "fallback"
)

type PaymentProcessorRequest struct {
	CorrelationID uuid.UUID `json:"correlationId"`
	Amount        float64   `json:"amount"`
	RequestedAt   string    `json:"requestedAt"`
}

type PaymentProcessorResponse struct {
	Message string `json:"message"`
}

type HealthResponse struct {
	Status string `json:"status"`
}

type Client struct {
	httpClient  *http.Client
	defaultURL  string
	fallbackURL string
}

func NewClient(defaultURL, fallbackURL string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		defaultURL:  defaultURL,
		fallbackURL: fallbackURL,
	}
}

func (c *Client) ProcessPayment(ctx context.Context, req PaymentProcessorRequest, processorType ProcessorType) (*PaymentProcessorResponse, error) {
	url := c.getProcessorURL(processorType)
	
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url+"/payments", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to %s processor: %w", processorType, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("%s processor returned server error: %d", processorType, resp.StatusCode)
	}
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s processor returned error: %d", processorType, resp.StatusCode)
	}

	var processorResp PaymentProcessorResponse
	if err := json.NewDecoder(resp.Body).Decode(&processorResp); err != nil {
		return nil, fmt.Errorf("failed to decode response from %s processor: %w", processorType, err)
	}

	// Validate response format
	if processorResp.Message != "payment processed successfully" {
		return nil, fmt.Errorf("%s processor returned invalid response message: %s", processorType, processorResp.Message)
	}

	return &processorResp, nil
}

func (c *Client) CheckHealth(ctx context.Context, processorType ProcessorType) (*HealthResponse, error) {
	url := c.getProcessorURL(processorType)
	
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url+"/payments/service-health", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send health check to %s processor: %w", processorType, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s processor health check returned error: %d", processorType, resp.StatusCode)
	}

	var healthResp HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
		return nil, fmt.Errorf("failed to decode health response from %s processor: %w", processorType, err)
	}

	return &healthResp, nil
}

func (c *Client) getProcessorURL(processorType ProcessorType) string {
	switch processorType {
	case ProcessorTypeDefault:
		return c.defaultURL
	case ProcessorTypeFallback:
		return c.fallbackURL
	default:
		return c.defaultURL
	}
}