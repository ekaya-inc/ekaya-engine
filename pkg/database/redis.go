package database

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/ekaya-inc/ekaya-engine/pkg/config"
)

// NewRedisClient creates a new Redis client with the given configuration.
// Returns nil if Redis is not configured (host is empty).
func NewRedisClient(cfg *config.RedisConfig) (*redis.Client, error) {
	if cfg.Host == "" {
		return nil, nil
	}

	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Test connection
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return client, nil
}
