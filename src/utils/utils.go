package utils

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

// Logger creates a new logger instance
func Logger() *logrus.Logger {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	return logger
}

// GetWelcomeMessage returns a welcome message with current timestamp
func GetWelcomeMessage() string {
	logrus.Debug("Generating welcome message")
	requestID := GenerateRequestID()
	return fmt.Sprintf("Welcome to the Go Bazel Aspects Demo! Request ID: %s, Time: %s",
		requestID, time.Now().Format(time.RFC3339))
}

// ProcessData simulates some data processing with enhanced functionality
func ProcessData(data string) string {
	logrus.WithField("input", data).Info("Processing data")
	hash := HashData([]byte(data))
	return fmt.Sprintf("Processed: %s | Hash: %s", data, hash[:8])
}

// FormatMessage formats a message with a prefix
func FormatMessage(prefix, message string) string {
	return fmt.Sprintf("[%s] %s", strings.ToUpper(prefix), message)
}

// IsValidInput checks if input string is valid
func IsValidInput(input string) bool {
	return len(strings.TrimSpace(input)) > 0
}

// HashData creates a SHA256 hash of the input data
func HashData(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

// GenerateRequestID creates a unique request ID
func GenerateRequestID() string {
	return uuid.New().String()
}

// NewRateLimiter creates a rate limiter for API requests
func NewRateLimiter(requestsPerSecond int) *rate.Limiter {
	return rate.NewLimiter(rate.Limit(requestsPerSecond), requestsPerSecond)
}

// WaitForRateLimit waits for rate limit permission
func WaitForRateLimit(ctx context.Context, limiter *rate.Limiter) error {
	return limiter.Wait(ctx)
}

// ProcessWithTimeout processes data with a timeout
func ProcessWithTimeout(ctx context.Context, timeout time.Duration, data string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultChan := make(chan string, 1)
	errorChan := make(chan error, 1)

	go func() {
		// Simulate processing
		time.Sleep(100 * time.Millisecond)
		processed := strings.ToUpper(data)
		hash := HashData([]byte(processed))
		resultChan <- fmt.Sprintf("%s (hash: %s)", processed, hash[:8])
	}()

	select {
	case result := <-resultChan:
		return result, nil
	case err := <-errorChan:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// ValidateAndProcess combines validation and processing
func ValidateAndProcess(input string) (string, error) {
	if !IsValidInput(input) {
		return "", fmt.Errorf("invalid input: %s", input)
	}

	ctx := context.Background()
	result, err := ProcessWithTimeout(ctx, 5*time.Second, input)
	if err != nil {
		return "", fmt.Errorf("processing failed: %w", err)
	}

	return result, nil
}
