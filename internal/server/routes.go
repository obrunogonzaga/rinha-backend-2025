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
	
	payment := &models.Payment{
		CorrelationID: req.CorrelationID,
		Amount:        req.Amount,
		Status:        models.PaymentStatusPending,
		RequestedAt:   time.Now().UTC(),
	}
	
	if err := s.db.CreatePayment(c.Request().Context(), payment); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to process payment"})
	}
	
	if err := s.workerPool.SubmitPayment(payment.ID, payment.CorrelationID, payment.Amount); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to submit payment for processing"})
	}
	
	response := models.PaymentResponse{
		Message: "Payment accepted for processing",
	}
	
	return c.JSON(http.StatusAccepted, response)
}

func (s *Server) paymentsSummaryHandler(c echo.Context) error {
	log.Printf("paymentsSummaryHandler called")
	
	startDateStr := c.QueryParam("startDate")
	endDateStr := c.QueryParam("endDate")
	
	log.Printf("Query params - startDate: %s, endDate: %s", startDateStr, endDateStr)
	
	var startDate, endDate *time.Time
	
	if startDateStr != "" {
		if parsed, err := time.Parse("2006-01-02", startDateStr); err == nil {
			startDate = &parsed
		} else {
			log.Printf("Invalid startDate format: %s", startDateStr)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid startDate format. Use YYYY-MM-DD"})
		}
	}
	
	if endDateStr != "" {
		if parsed, err := time.Parse("2006-01-02", endDateStr); err == nil {
			endOfDay := parsed.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
			endDate = &endOfDay
		} else {
			log.Printf("Invalid endDate format: %s", endDateStr)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid endDate format. Use YYYY-MM-DD"})
		}
	}
	
	log.Printf("Calling GetPaymentSummary with startDate: %v, endDate: %v", startDate, endDate)
	
	summaries, err := s.db.GetPaymentSummary(c.Request().Context(), startDate, endDate)
	if err != nil {
		log.Printf("Error from GetPaymentSummary: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get payment summary", "details": err.Error()})
	}
	
	log.Printf("GetPaymentSummary returned %d summaries", len(summaries))
	
	response := models.PaymentSummaryResponse{
		Summary: summaries,
	}
	
	log.Printf("Returning response: %+v", response)
	
	return c.JSON(http.StatusOK, response)
}
