package server

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/joho/godotenv/autoload"

	"rinha-backend-2025/internal/database"
	"rinha-backend-2025/internal/processors"
	"rinha-backend-2025/internal/workers"
)

type Server struct {
	port        int
	db          database.Service
	workerPool  *workers.PaymentWorkerPool
}

func NewServer() (*http.Server, *Server) {
	port, _ := strconv.Atoi(os.Getenv("PORT"))
	
	dbService := database.New()
	
	defaultURL := os.Getenv("PAYMENT_PROCESSOR_URL_DEFAULT")
	if defaultURL == "" {
		defaultURL = "http://payment-processor-default:8080"
	}
	
	fallbackURL := os.Getenv("PAYMENT_PROCESSOR_URL_FALLBACK")
	if fallbackURL == "" {
		fallbackURL = "http://payment-processor-fallback:8080"
	}
	
	processorService := processors.NewProcessorService(defaultURL, fallbackURL)
	workerPool := workers.NewPaymentWorkerPool(5, 1000, processorService, dbService)
	workerPool.Start()
	
	appServer := &Server{
		port:       port,
		db:         dbService,
		workerPool: workerPool,
	}

	// Declare Server config
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", appServer.port),
		Handler:      appServer.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	return httpServer, appServer
}

func (s *Server) Shutdown() {
	if s.workerPool != nil {
		s.workerPool.Stop()
	}
}
