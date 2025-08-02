package server

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"rinha-backend-2025/internal/models"
)

func (s *Server) RegisterRoutes() http.Handler {
	e := echo.New()
	e.Use(middleware.Recover())

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"https://*", "http://*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	e.GET("/", s.HelloWorldHandler)
	e.GET("/health", s.healthHandler)
	e.POST("/payments", s.createPaymentHandler)
	e.GET("/payments-summary", s.paymentsSummaryHandler)
	e.DELETE("/payments", s.clearPaymentsHandler)

	return e
}

func (s *Server) HelloWorldHandler(c echo.Context) error {
	resp := map[string]string{
		"message": "Hello World",
	}

	return c.JSON(http.StatusOK, resp)
}

func (s *Server) healthHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, s.db.Health())
}

func (s *Server) createPaymentHandler(c echo.Context) error {
	var req models.PaymentRequest
	
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request format"})
	}
	
	if req.Amount <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Amount must be greater than 0"})
	}
	
	// Respond immediately, then process async
	response := models.PaymentResponse{
		Message: "Payment accepted for processing",
	}
	
	// Return response immediately
	if err := c.JSON(http.StatusAccepted, response); err != nil {
		return err
	}
	
	// Process payment asynchronously after response
	go func() {
		requestedAt := time.Now().UTC()
		payment := &models.Payment{
			CorrelationID: req.CorrelationID,
			Amount:        req.Amount,
			Status:        models.PaymentStatusPending,
			RequestedAt:   requestedAt,
		}
		
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		if err := s.db.CreatePayment(ctx, payment); err != nil {
			return
		}
		
		redisCtx, redisCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer redisCancel()
		
		s.redisService.PublishPaymentJob(redisCtx, payment)
	}()
	
	return nil
}

func (s *Server) paymentsSummaryHandler(c echo.Context) error {
	fromStr := c.QueryParam("from")
	toStr := c.QueryParam("to")
	
	var startDate, endDate *time.Time
	
	if fromStr != "" {
		if parsed, err := time.Parse(time.RFC3339, fromStr); err == nil {
			startDate = &parsed
		} else {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid from format. Use ISO 8601 format (e.g., 2020-07-10T12:34:56.000Z)"})
		}
	}
	
	if toStr != "" {
		if parsed, err := time.Parse(time.RFC3339, toStr); err == nil {
			endDate = &parsed
		} else {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid to format. Use ISO 8601 format (e.g., 2020-07-10T12:34:56.000Z)"})
		}
	}
	
	summary, err := s.db.GetPaymentSummary(c.Request().Context(), startDate, endDate)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get payment summary"})
	}
	
	return c.JSON(http.StatusOK, summary)
}

func (s *Server) clearPaymentsHandler(c echo.Context) error {
	err := s.db.ClearPayments(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to clear payments"})
	}
	
	return c.JSON(http.StatusOK, map[string]string{"message": "All payments cleared successfully"})
}
