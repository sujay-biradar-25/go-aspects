package web

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"

	"github.com/example/go-aspects/src/database"
	"github.com/example/go-aspects/src/utils"
)

// Server represents the web server
type Server struct {
	engine  *gin.Engine
	logger  *logrus.Logger
	db      *database.Connection
	jwtKey  []byte
	metrics *ServerMetrics
}

// ServerMetrics holds Prometheus metrics
type ServerMetrics struct {
	RequestsTotal   *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec
	ActiveUsers     prometheus.Gauge
}

// Config holds server configuration
type Config struct {
	Port      int
	JWTSecret string
	Database  database.Config
}

// NewServer creates a new web server
func NewServer(config Config) (*Server, error) {
	logger := utils.Logger()

	// Setup database connection
	db, err := database.NewConnection(config.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to create database connection: %w", err)
	}

	// Setup metrics
	metrics := &ServerMetrics{
		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "endpoint", "status"},
		),
		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "http_request_duration_seconds",
				Help: "HTTP request duration in seconds",
			},
			[]string{"method", "endpoint"},
		),
		ActiveUsers: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "active_users",
				Help: "Number of active users",
			},
		),
	}

	// Register metrics
	prometheus.MustRegister(metrics.RequestsTotal)
	prometheus.MustRegister(metrics.RequestDuration)
	prometheus.MustRegister(metrics.ActiveUsers)

	engine := gin.New()
	server := &Server{
		engine:  engine,
		logger:  logger,
		db:      db,
		jwtKey:  []byte(config.JWTSecret),
		metrics: metrics,
	}

	server.setupRoutes()
	return server, nil
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes() {
	// Middleware
	s.engine.Use(s.loggingMiddleware())
	s.engine.Use(s.metricsMiddleware())
	s.engine.Use(gin.Recovery())

	// Public routes
	s.engine.GET("/health", s.healthHandler)
	s.engine.GET("/metrics", gin.WrapH(promhttp.Handler()))
	s.engine.POST("/auth/login", s.loginHandler)

	// Protected routes
	protected := s.engine.Group("/api")
	protected.Use(s.authMiddleware())
	{
		protected.GET("/profile", s.profileHandler)
		protected.POST("/data/process", s.processDataHandler)
		protected.GET("/stats", s.statsHandler)
	}
}

// loggingMiddleware logs HTTP requests
func (s *Server) loggingMiddleware() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		s.logger.WithFields(logrus.Fields{
			"client_ip":  param.ClientIP,
			"method":     param.Method,
			"path":       param.Path,
			"status":     param.StatusCode,
			"latency":    param.Latency,
			"user_agent": param.Request.UserAgent(),
		}).Info("HTTP request")
		return ""
	})
}

// metricsMiddleware collects Prometheus metrics
func (s *Server) metricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		duration := time.Since(start).Seconds()
		status := fmt.Sprintf("%d", c.Writer.Status())

		s.metrics.RequestsTotal.WithLabelValues(c.Request.Method, c.FullPath(), status).Inc()
		s.metrics.RequestDuration.WithLabelValues(c.Request.Method, c.FullPath()).Observe(duration)
	}
}

// authMiddleware validates JWT tokens
func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := c.GetHeader("Authorization")
		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		// Remove "Bearer " prefix
		if len(tokenString) > 7 && tokenString[:7] == "Bearer " {
			tokenString = tokenString[7:]
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return s.jwtKey, nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			c.Set("user_id", claims["user_id"])
		}

		c.Next()
	}
}

// healthHandler checks system health
func (s *Server) healthHandler(c *gin.Context) {
	ctx := context.Background()

	// Check database health
	if err := s.db.HealthCheck(ctx); err != nil {
		s.logger.WithError(err).Error("Database health check failed")
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unhealthy",
			"error":  "database connection failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"version":   "1.0.0",
	})
}

// loginHandler handles user authentication
func (s *Server) loginHandler(c *gin.Context) {
	var loginReq struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&loginReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Simulate password verification
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	if err := bcrypt.CompareHashAndPassword(hashedPassword, []byte(loginReq.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// Generate JWT
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": loginReq.Username,
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
	})

	tokenString, err := token.SignedString(s.jwtKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":   tokenString,
		"expires": time.Now().Add(time.Hour * 24).Unix(),
		"user_id": loginReq.Username,
	})
}

// profileHandler returns user profile
func (s *Server) profileHandler(c *gin.Context) {
	userID := c.GetString("user_id")

	c.JSON(http.StatusOK, gin.H{
		"user_id":    userID,
		"profile":    "User Profile Data",
		"timestamp":  time.Now().Unix(),
		"request_id": utils.GenerateRequestID(),
	})
}

// processDataHandler processes data using utils
func (s *Server) processDataHandler(c *gin.Context) {
	var req struct {
		Data string `json:"data" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	processed, err := utils.ValidateAndProcess(req.Data)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"original":  req.Data,
		"processed": processed,
		"timestamp": time.Now().Unix(),
	})
}

// statsHandler returns system statistics
func (s *Server) statsHandler(c *gin.Context) {
	dbStats := s.db.GetStats()

	c.JSON(http.StatusOK, gin.H{
		"database": map[string]interface{}{
			"open_connections": dbStats.OpenConnections,
			"idle":             dbStats.Idle,
			"in_use":           dbStats.InUse,
			"wait_count":       dbStats.WaitCount,
		},
		"server": map[string]interface{}{
			"uptime":    time.Since(time.Now()).Seconds(), // This would be actual uptime in real code
			"timestamp": time.Now().Unix(),
		},
	})
}

// Start starts the HTTP server
func (s *Server) Start(port int) error {
	s.logger.WithField("port", port).Info("Starting web server")
	return s.engine.Run(fmt.Sprintf(":%d", port))
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down web server")
	return s.db.Close()
}
