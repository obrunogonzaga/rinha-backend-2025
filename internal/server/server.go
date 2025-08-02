package server

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/joho/godotenv/autoload"

	"rinha-backend-2025/internal/database"
	"rinha-backend-2025/internal/healthmonitor"
	"rinha-backend-2025/internal/processors"
	"rinha-backend-2025/internal/redis"
	"rinha-backend-2025/internal/workers"
)

type Server struct {
	port           int
	db             database.Service
	redisService   *redis.Service
	healthMonitor  *healthmonitor.HealthMonitor
	workerPool     *workers.PaymentWorkerPool
	retryProcessor *workers.RetryProcessor
}

func NewServer() (*http.Server, *Server) {
	port, _ := strconv.Atoi(os.Getenv("PORT"))
	
	dbService := database.New()
	
	// Initialize Redis
	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}
	
	redisPort := os.Getenv("REDIS_PORT")
	if redisPort == "" {
		redisPort = "6379"
	}
	
	redisConfig := redis.Config{
		Host:     redisHost,
		Port:     redisPort,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	}
	
	redisClient := redis.NewClient(redisConfig)
	redisService := redis.NewService(redisClient)
	
	defaultURL := os.Getenv("PAYMENT_PROCESSOR_URL_DEFAULT")
	if defaultURL == "" {
		defaultURL = "http://payment-processor-default:8080"
	}
	
	fallbackURL := os.Getenv("PAYMENT_PROCESSOR_URL_FALLBACK")
	if fallbackURL == "" {
		fallbackURL = "http://payment-processor-fallback:8080"
	}
	
	processorService := processors.NewProcessorService(defaultURL, fallbackURL, redisService)
	
	// Initialize health monitor
	healthMonitorConfig := healthmonitor.Config{
		CheckInterval: 5 * time.Second,
		HealthTimeout: 3 * time.Second,
	}
	
	// We need the processor client for health monitoring
	processorClient := processors.NewClient(defaultURL, fallbackURL)
	healthMonitor := healthmonitor.NewHealthMonitor(processorClient, redisService, healthMonitorConfig)
	healthMonitor.Start()
	
	workerPool := workers.NewPaymentWorkerPool(5, processorService, dbService, redisService)
	workerPool.Start()
	
	// Initialize retry processor
	retryProcessor := workers.NewRetryProcessor(redisService)
	retryProcessor.Start()
	
	appServer := &Server{
		port:           port,
		db:             dbService,
		redisService:   redisService,
		healthMonitor:  healthMonitor,
		workerPool:     workerPool,
		retryProcessor: retryProcessor,
	}

	// Declare Server config optimized for high throughput
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", appServer.port),
		Handler:      appServer.RegisterRoutes(),
		IdleTimeout:  30 * time.Second,  // Reduced from 1 minute
		ReadTimeout:  5 * time.Second,   // Reduced from 10 seconds
		WriteTimeout: 10 * time.Second,  // Reduced from 30 seconds
	}

	return httpServer, appServer
}

func (s *Server) Shutdown() {
	if s.healthMonitor != nil {
		s.healthMonitor.Stop()
	}
	if s.retryProcessor != nil {
		s.retryProcessor.Stop()
	}
	if s.workerPool != nil {
		s.workerPool.Stop()
	}
	if s.redisService != nil {
		s.redisService.Close()
	}
}
