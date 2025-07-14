package server

import (
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
	
	response := models.PaymentResponse{
		Message: "Payment accepted for processing",
	}
	
	return c.JSON(http.StatusAccepted, response)
}
