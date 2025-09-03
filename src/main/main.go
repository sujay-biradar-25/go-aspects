package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"

	"github.com/example/go-aspects/src/utils"
)

// Application represents the main application
type Application struct {
	logger    *logrus.Logger
	limiter   *rate.Limiter
	requestID string
}

// Config holds application configuration
type Config struct {
	Port          int
	RateLimit     int
	EnableMetrics bool
	EnableTracing bool
}

func main() {
	logger := utils.Logger()
	logger.Info("Starting Complex Go Bazel Aspects Demo Application")

	// Print welcome message with request tracking
	welcome := utils.GetWelcomeMessage()
	logger.Info(welcome)

	// Initialize application configuration
	config := Config{
		Port:          8080,
		RateLimit:     100,
		EnableMetrics: true,
		EnableTracing: true,
	}

	app, err := NewApplication(config, logger)
	if err != nil {
		logger.WithError(err).Fatal("Failed to initialize application")
	}

	// Demo complex dependency usage
	if err := app.demonstrateComplexOperations(); err != nil {
		logger.WithError(err).Error("Failed to demonstrate complex operations")
	}

	// Start the application
	if err := app.Start(); err != nil {
		logger.WithError(err).Fatal("Failed to start application")
	}
}

// NewApplication creates a new application instance
func NewApplication(config Config, logger *logrus.Logger) (*Application, error) {
	// Initialize rate limiter using utils
	limiter := utils.NewRateLimiter(config.RateLimit)

	// Generate application-wide request ID
	requestID := utils.GenerateRequestID()

	logger.WithFields(logrus.Fields{
		"request_id":     requestID,
		"rate_limit":     config.RateLimit,
		"enable_metrics": config.EnableMetrics,
		"enable_tracing": config.EnableTracing,
	}).Info("Application components initialized successfully")

	return &Application{
		logger:    logger,
		limiter:   limiter,
		requestID: requestID,
	}, nil
}

// demonstrateComplexOperations shows complex dependency interactions
func (app *Application) demonstrateComplexOperations() error {
	app.logger.WithField("request_id", app.requestID).Info("Demonstrating complex operations...")

	// Test rate limiting
	ctx := context.Background()
	if err := utils.WaitForRateLimit(ctx, app.limiter); err != nil {
		return fmt.Errorf("rate limiting failed: %w", err)
	}

	// Test data processing with timeout - multiple scenarios
	testData := []string{
		"user_input_1",
		"user_input_2",
		"complex_data_structure_with_special_chars_!@#$%^&*()",
		"json_payload_simulation",
		"long_text_that_should_be_processed_correctly",
		"unicode_data_测试数据",
	}

	for i, data := range testData {
		processed, err := utils.ValidateAndProcess(data)
		if err != nil {
			app.logger.WithError(err).WithFields(logrus.Fields{
				"data":       data,
				"request_id": app.requestID,
			}).Error("Processing failed")
			continue
		}

		hash := utils.HashData([]byte(processed))
		sessionID := uuid.New().String()

		app.logger.WithFields(logrus.Fields{
			"iteration":  i + 1,
			"original":   data,
			"processed":  processed,
			"hash":       hash[:16], // First 16 chars of hash
			"session_id": sessionID,
			"request_id": app.requestID,
		}).Info("Data processing completed")

		// Simulate rate limiting between operations
		time.Sleep(10 * time.Millisecond)
	}

	// Test complex validation scenarios with different edge cases
	validationTests := []struct {
		input       string
		expected    bool
		description string
	}{
		{"valid input", true, "normal valid input"},
		{"", false, "empty string"},
		{"   ", false, "whitespace only"},
		{"complex data with special chars !@#$%", true, "special characters"},
		{"very_long_input_" + string(make([]byte, 100)), true, "long input"},
		{"\n\t\r", false, "control characters only"},
		{"mixed content 123 !@# abc", true, "mixed alphanumeric"},
	}

	for _, test := range validationTests {
		result := utils.IsValidInput(test.input)
		testID := uuid.New().String()

		app.logger.WithFields(logrus.Fields{
			"input":       test.input,
			"expected":    test.expected,
			"actual":      result,
			"passed":      result == test.expected,
			"description": test.description,
			"test_id":     testID,
			"request_id":  app.requestID,
		}).Info("Validation test completed")
	}

	// Test concurrent processing simulation
	concurrentData := []string{"data1", "data2", "data3", "data4", "data5"}
	results := make(chan string, len(concurrentData))

	for _, data := range concurrentData {
		go func(d string) {
			processed, err := utils.ValidateAndProcess(d)
			if err != nil {
				results <- fmt.Sprintf("error: %s", err.Error())
			} else {
				results <- processed
			}
		}(data)
	}

	// Collect results
	for i := 0; i < len(concurrentData); i++ {
		result := <-results
		app.logger.WithFields(logrus.Fields{
			"result":     result,
			"goroutine":  i + 1,
			"request_id": app.requestID,
		}).Info("Concurrent processing result")
	}

	app.logger.WithField("request_id", app.requestID).Info("Complex operations demonstration completed")
	return nil
}

// Start starts the application
func (app *Application) Start() error {
	app.logger.WithField("request_id", app.requestID).Info("Starting application services...")

	// Setup HTTP router with multiple endpoints
	router := mux.NewRouter()

	// Add middleware for request tracking
	router.Use(app.requestTrackingMiddleware)

	router.HandleFunc("/health", app.healthHandler).Methods("GET")
	router.HandleFunc("/process", app.processHandler).Methods("GET", "POST")
	router.HandleFunc("/stats", app.statsHandler).Methods("GET")
	router.HandleFunc("/validate", app.validateHandler).Methods("POST")
	router.HandleFunc("/uuid", app.uuidHandler).Methods("GET")
	router.HandleFunc("/hash/{data}", app.hashHandler).Methods("GET")

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		app.logger.WithField("request_id", app.requestID).Info("Received shutdown signal")
		cancel()
	}()

	server := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	go func() {
		app.logger.WithFields(logrus.Fields{
			"port":       8080,
			"request_id": app.requestID,
		}).Info("Starting HTTP server")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			app.logger.WithError(err).Fatal("Server failed to start")
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	app.logger.WithField("request_id", app.requestID).Info("Shutting down gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		app.logger.WithError(err).Error("Server shutdown failed")
	}

	app.logger.WithField("request_id", app.requestID).Info("Application stopped successfully")
	return nil
}

// HTTP Middleware

func (app *Application) requestTrackingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := utils.GenerateRequestID()
		w.Header().Set("X-Request-ID", requestID)

		app.logger.WithFields(logrus.Fields{
			"method":     r.Method,
			"path":       r.URL.Path,
			"request_id": requestID,
			"user_agent": r.UserAgent(),
		}).Info("HTTP request received")

		next.ServeHTTP(w, r)
	})
}

// HTTP Handlers

func (app *Application) healthHandler(w http.ResponseWriter, r *http.Request) {
	requestID := w.Header().Get("X-Request-ID")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	response := fmt.Sprintf(`{
		"status": "healthy",
		"service": "complex-go-bazel-aspects-demo",
		"timestamp": %d,
		"request_id": "%s",
		"version": "2.0.0",
		"features": ["rate_limiting", "uuid_generation", "data_hashing", "validation"]
	}`, time.Now().Unix(), requestID)

	fmt.Fprint(w, response)
}

func (app *Application) processHandler(w http.ResponseWriter, r *http.Request) {
	requestID := w.Header().Get("X-Request-ID")

	// Rate limiting
	ctx := context.Background()
	if err := utils.WaitForRateLimit(ctx, app.limiter); err != nil {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	var data string
	if r.Method == "GET" {
		data = r.URL.Query().Get("data")
		if data == "" {
			data = "default data"
		}
	} else {
		// POST method - read from body (simplified)
		data = r.FormValue("data")
		if data == "" {
			data = "POST data received"
		}
	}

	processed, err := utils.ValidateAndProcess(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hash := utils.HashData([]byte(processed))
	sessionID := uuid.New().String()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	response := fmt.Sprintf(`{
		"input": "%s",
		"output": "%s",
		"request_id": "%s",
		"session_id": "%s",
		"timestamp": %d,
		"hash": "%s"
	}`, data, processed, requestID, sessionID, time.Now().Unix(), hash[:16])

	fmt.Fprint(w, response)
}

func (app *Application) statsHandler(w http.ResponseWriter, r *http.Request) {
	requestID := w.Header().Get("X-Request-ID")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	response := fmt.Sprintf(`{
		"stats": {
			"uptime_seconds": %d,
			"app_request_id": "%s",
			"rate_limiter": "active",
			"components": ["utils", "mux", "logrus", "uuid", "time/rate"],
			"dependencies_count": 5,
			"complex_features": ["concurrent_processing", "rate_limiting", "uuid_generation", "data_validation"]
		},
		"request_id": "%s",
		"timestamp": %d
	}`, int64(time.Since(time.Now()).Seconds()), app.requestID, requestID, time.Now().Unix())

	fmt.Fprint(w, response)
}

func (app *Application) validateHandler(w http.ResponseWriter, r *http.Request) {
	requestID := w.Header().Get("X-Request-ID")

	data := r.FormValue("data")
	isValid := utils.IsValidInput(data)
	validationID := uuid.New().String()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	response := fmt.Sprintf(`{
		"input": "%s",
		"valid": %t,
		"validation_id": "%s",
		"request_id": "%s",
		"timestamp": %d
	}`, data, isValid, validationID, requestID, time.Now().Unix())

	fmt.Fprint(w, response)
}

func (app *Application) uuidHandler(w http.ResponseWriter, r *http.Request) {
	requestID := w.Header().Get("X-Request-ID")

	// Generate multiple UUIDs to show functionality
	uuids := make([]string, 5)
	for i := range uuids {
		uuids[i] = uuid.New().String()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	response := fmt.Sprintf(`{
		"generated_uuids": ["%s", "%s", "%s", "%s", "%s"],
		"request_id": "%s",
		"timestamp": %d
	}`, uuids[0], uuids[1], uuids[2], uuids[3], uuids[4], requestID, time.Now().Unix())

	fmt.Fprint(w, response)
}

func (app *Application) hashHandler(w http.ResponseWriter, r *http.Request) {
	requestID := w.Header().Get("X-Request-ID")
	vars := mux.Vars(r)
	data := vars["data"]

	if data == "" {
		http.Error(w, "data parameter required", http.StatusBadRequest)
		return
	}

	hash := utils.HashData([]byte(data))
	hashID := uuid.New().String()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	response := fmt.Sprintf(`{
		"original_data": "%s",
		"sha256_hash": "%s",
		"hash_id": "%s",
		"request_id": "%s",
		"timestamp": %d
	}`, data, hash, hashID, requestID, time.Now().Unix())

	fmt.Fprint(w, response)
}
