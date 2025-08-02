package redis

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/redis"
)

func TestRedisIntegration(t *testing.T) {
	ctx := context.Background()

	// Start Redis container using testcontainers Redis module
	redisContainer, err := redis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("Failed to start Redis container: %v", err)
	}
	defer redisContainer.Terminate(ctx)

	// Get connection string
	connectionString, err := redisContainer.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	// Parse connection info
	host, err := redisContainer.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get container host: %v", err)
	}

	port, err := redisContainer.MappedPort(ctx, "6379")
	if err != nil {
		t.Fatalf("Failed to get container port: %v", err)
	}

	t.Logf("Redis connection string: %s", connectionString)

	// Create Redis client
	config := Config{
		Host:     host,
		Port:     port.Port(),
		Password: "",
		DB:       0,
	}

	client := NewClient(config)
	service := NewService(client)

	// Test Redis connectivity
	t.Run("TestPing", func(t *testing.T) {
		err := service.Ping(ctx)
		if err != nil {
			t.Errorf("Ping failed: %v", err)
		}
	})

	// Test health caching
	t.Run("TestHealthCaching", func(t *testing.T) {
		processorType := "test-processor"

		// Cache healthy status
		err := service.CacheProcessorHealth(ctx, processorType, true)
		if err != nil {
			t.Errorf("Failed to cache health status: %v", err)
		}

		// Retrieve cached status
		healthy, exists, err := service.GetProcessorHealth(ctx, processorType)
		if err != nil {
			t.Errorf("Failed to get health status: %v", err)
		}
		if !exists {
			t.Error("Health status should exist")
		}
		if !healthy {
			t.Error("Health status should be healthy")
		}

		// Wait for TTL expiry and test again
		time.Sleep(31 * time.Second) // DefaultHealthTTL is 30 seconds

		_, exists, err = service.GetProcessorHealth(ctx, processorType)
		if err != nil {
			t.Errorf("Failed to get health status after TTL: %v", err)
		}
		if exists {
			t.Error("Health status should have expired")
		}
	})

	// Test queue operations
	t.Run("TestQueueOperations", func(t *testing.T) {
		// Test queue length (should be 0 initially)
		length, err := service.GetPaymentQueueLength(ctx)
		if err != nil {
			t.Errorf("Failed to get queue length: %v", err)
		}
		if length != 0 {
			t.Errorf("Expected queue length 0, got %d", length)
		}

		// Note: We can't easily test the full payment job flow without
		// setting up the complete models, but we've tested the core Redis operations
	})

	// Cleanup
	service.Close()
}