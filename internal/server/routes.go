package server

import (
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"rinha-backend-2025/internal/models"
)

func (s *Server) RegisterRoutes() http.Handler {
	e := echo.New()
	e.Use(middleware.Logger())
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
	
	requestedAt := time.Now().UTC()
	payment := &models.Payment{
		CorrelationID: req.CorrelationID,
		Amount:        req.Amount,
		Status:        models.PaymentStatusPending,
		RequestedAt:   requestedAt,
	}
	
	log.Printf("Creating payment with RequestedAt: %v", payment.RequestedAt)
	
	if err := s.db.CreatePayment(c.Request().Context(), payment); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to process payment"})
	}
	
	log.Printf("Submitting payment to worker with RequestedAt: %v", payment.RequestedAt)
	
	if err := s.workerPool.SubmitPayment(payment.ID, payment.CorrelationID, payment.Amount, payment.RequestedAt); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to submit payment for processing"})
	}
	
	response := models.PaymentResponse{
		Message: "Payment accepted for processing",
	}
	
	return c.JSON(http.StatusAccepted, response)
}

func (s *Server) paymentsSummaryHandler(c echo.Context) error {
	log.Printf("paymentsSummaryHandler called")
	
	fromStr := c.QueryParam("from")
	toStr := c.QueryParam("to")
	
	log.Printf("Query params - from: %s, to: %s", fromStr, toStr)
	
	var startDate, endDate *time.Time
	
	if fromStr != "" {
		if parsed, err := time.Parse(time.RFC3339, fromStr); err == nil {
			startDate = &parsed
		} else {
			log.Printf("Invalid from format: %s", fromStr)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid from format. Use ISO 8601 format (e.g., 2020-07-10T12:34:56.000Z)"})
		}
	}
	
	if toStr != "" {
		if parsed, err := time.Parse(time.RFC3339, toStr); err == nil {
			endDate = &parsed
		} else {
			log.Printf("Invalid to format: %s", toStr)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid to format. Use ISO 8601 format (e.g., 2020-07-10T12:34:56.000Z)"})
		}
	}
	
	log.Printf("Calling GetPaymentSummary with startDate: %v, endDate: %v", startDate, endDate)
	
	summary, err := s.db.GetPaymentSummary(c.Request().Context(), startDate, endDate)
	if err != nil {
		log.Printf("Error from GetPaymentSummary: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get payment summary", "details": err.Error()})
	}
	
	log.Printf("GetPaymentSummary returned summary: %+v", summary)
	
	return c.JSON(http.StatusOK, summary)
}

func (s *Server) clearPaymentsHandler(c echo.Context) error {
	log.Printf("clearPaymentsHandler called")
	
	err := s.db.ClearPayments(c.Request().Context())
	if err != nil {
		log.Printf("Error clearing payments: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to clear payments"})
	}
	
	return c.JSON(http.StatusOK, map[string]string{"message": "All payments cleared successfully"})
}
