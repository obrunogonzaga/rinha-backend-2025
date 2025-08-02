package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps redis client with application-specific methods
type Client struct {
	rdb *redis.Client
}

// Config holds Redis configuration
type Config struct {
	Host     string
	Port     string
	Password string
	DB       int
}

// NewClient creates a new Redis client
func NewClient(config Config) *Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", config.Host, config.Port),
		Password: config.Password,
		DB:       config.DB,
	})

	return &Client{
		rdb: rdb,
	}
}

// Ping checks Redis connectivity
func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// Close closes Redis connection
func (c *Client) Close() error {
	return c.rdb.Close()
}

// PublishJob publishes a job to a Redis list (queue)
func (c *Client) PublishJob(ctx context.Context, queueName string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal job data: %w", err)
	}

	return c.rdb.LPush(ctx, queueName, jsonData).Err()
}

// ConsumeJob consumes a job from a Redis list (blocking operation)
func (c *Client) ConsumeJob(ctx context.Context, queueName string, timeout time.Duration) ([]byte, error) {
	result, err := c.rdb.BRPop(ctx, timeout, queueName).Result()
	if err != nil {
		return nil, err
	}

	if len(result) < 2 {
		return nil, fmt.Errorf("unexpected result format from BRPop")
	}

	return []byte(result[1]), nil
}

// SetWithExpiration sets a key-value pair with expiration
func (c *Client) SetWithExpiration(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return c.rdb.Set(ctx, key, value, expiration).Err()
}

// Get retrieves a value by key
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.rdb.Get(ctx, key).Result()
}

// Exists checks if a key exists
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	count, err := c.rdb.Exists(ctx, key).Result()
	return count > 0, err
}

// Delete removes a key
func (c *Client) Delete(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}

// QueueLength returns the length of a list (queue)
func (c *Client) QueueLength(ctx context.Context, queueName string) (int64, error) {
	return c.rdb.LLen(ctx, queueName).Result()
}