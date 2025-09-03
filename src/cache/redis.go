package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/example/go-aspects/src/utils"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

// RedisConfig holds Redis configuration
type RedisConfig struct {
	Address  string
	Password string
	Database int
}

// RedisCache wraps Redis client with additional functionality
type RedisCache struct {
	client *redis.Client
	logger *logrus.Logger
}

// NewRedisCache creates a new Redis cache client
func NewRedisCache(config RedisConfig) (*RedisCache, error) {
	logger := utils.Logger()

	client := redis.NewClient(&redis.Options{
		Addr:     config.Address,
		Password: config.Password,
		DB:       config.Database,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logger.Info("Connected to Redis successfully")

	return &RedisCache{
		client: client,
		logger: logger,
	}, nil
}

// Set stores a value with expiration
func (r *RedisCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	requestID := utils.GenerateRequestID()
	r.logger.WithFields(logrus.Fields{
		"key":        key,
		"expiration": expiration,
		"request_id": requestID,
	}).Debug("Setting cache value")

	return r.client.Set(ctx, key, data, expiration).Err()
}

// Get retrieves a value from cache
func (r *RedisCache) Get(ctx context.Context, key string, dest interface{}) error {
	requestID := utils.GenerateRequestID()
	r.logger.WithFields(logrus.Fields{
		"key":        key,
		"request_id": requestID,
	}).Debug("Getting cache value")

	data, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return fmt.Errorf("key %s not found", key)
		}
		return fmt.Errorf("failed to get value: %w", err)
	}

	if err := json.Unmarshal([]byte(data), dest); err != nil {
		return fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return nil
}

// Delete removes a key from cache
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	requestID := utils.GenerateRequestID()
	r.logger.WithFields(logrus.Fields{
		"key":        key,
		"request_id": requestID,
	}).Debug("Deleting cache value")

	return r.client.Del(ctx, key).Err()
}

// Exists checks if a key exists in cache
func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	count, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// SetMultiple stores multiple key-value pairs
func (r *RedisCache) SetMultiple(ctx context.Context, pairs map[string]interface{}, expiration time.Duration) error {
	pipe := r.client.Pipeline()

	for key, value := range pairs {
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("failed to marshal value for key %s: %w", key, err)
		}
		pipe.Set(ctx, key, data, expiration)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to execute pipeline: %w", err)
	}

	r.logger.WithField("count", len(pairs)).Info("Set multiple cache values")
	return nil
}

// GetMultiple retrieves multiple values from cache
func (r *RedisCache) GetMultiple(ctx context.Context, keys []string) (map[string]interface{}, error) {
	pipe := r.client.Pipeline()

	cmds := make(map[string]*redis.StringCmd)
	for _, key := range keys {
		cmds[key] = pipe.Get(ctx, key)
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to execute pipeline: %w", err)
	}

	results := make(map[string]interface{})
	for key, cmd := range cmds {
		data, err := cmd.Result()
		if err == redis.Nil {
			continue // Key not found, skip
		}
		if err != nil {
			r.logger.WithError(err).WithField("key", key).Warn("Failed to get key")
			continue
		}

		var value interface{}
		if err := json.Unmarshal([]byte(data), &value); err != nil {
			r.logger.WithError(err).WithField("key", key).Warn("Failed to unmarshal value")
			continue
		}

		results[key] = value
	}

	return results, nil
}

// Increment atomically increments a counter
func (r *RedisCache) Increment(ctx context.Context, key string) (int64, error) {
	return r.client.Incr(ctx, key).Result()
}

// SetWithHash stores a value with a hash key for versioning
func (r *RedisCache) SetWithHash(ctx context.Context, baseKey string, value interface{}, expiration time.Duration) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("failed to marshal value: %w", err)
	}

	hash := utils.HashData(data)
	key := fmt.Sprintf("%s:%s", baseKey, hash[:8])

	if err := r.Set(ctx, key, value, expiration); err != nil {
		return "", err
	}

	return key, nil
}

// GetStats returns Redis connection statistics
func (r *RedisCache) GetStats(ctx context.Context) (map[string]interface{}, error) {
	info, err := r.client.Info(ctx, "stats").Result()
	if err != nil {
		return nil, err
	}

	poolStats := r.client.PoolStats()

	return map[string]interface{}{
		"redis_info": info,
		"pool_stats": map[string]interface{}{
			"hits":        poolStats.Hits,
			"misses":      poolStats.Misses,
			"timeouts":    poolStats.Timeouts,
			"total_conns": poolStats.TotalConns,
			"idle_conns":  poolStats.IdleConns,
			"stale_conns": poolStats.StaleConns,
		},
	}, nil
}

// Close closes the Redis connection
func (r *RedisCache) Close() error {
	r.logger.Info("Closing Redis connection")
	return r.client.Close()
}

// HealthCheck performs a Redis health check
func (r *RedisCache) HealthCheck(ctx context.Context) error {
	testKey := "health_check:" + utils.GenerateRequestID()
	testValue := map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"test":      true,
	}

	// Test set
	if err := r.Set(ctx, testKey, testValue, time.Minute); err != nil {
		return fmt.Errorf("health check set failed: %w", err)
	}

	// Test get
	var retrieved map[string]interface{}
	if err := r.Get(ctx, testKey, &retrieved); err != nil {
		return fmt.Errorf("health check get failed: %w", err)
	}

	// Test delete
	if err := r.Delete(ctx, testKey); err != nil {
		return fmt.Errorf("health check delete failed: %w", err)
	}

	r.logger.Info("Redis health check passed")
	return nil
}
