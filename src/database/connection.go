package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// Config holds database configuration
type Config struct {
	Host     string
	Port     int
	Database string
	Username string
	Password string
	SSLMode  string
}

// Connection wraps database connection with additional functionality
type Connection struct {
	db     *sql.DB
	config Config
	logger *logrus.Logger
}

// NewConnection creates a new database connection
func NewConnection(config Config) (*Connection, error) {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})

	dsn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		config.Host, config.Port, config.Database, config.Username, config.Password, config.SSLMode)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	conn := &Connection{
		db:     db,
		config: config,
		logger: logger,
	}

	return conn, nil
}

// Ping checks if the database connection is alive
func (c *Connection) Ping(ctx context.Context) error {
	c.logger.Debug("Pinging database")

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return c.db.PingContext(ctx)
}

// Close closes the database connection
func (c *Connection) Close() error {
	c.logger.Info("Closing database connection")
	return c.db.Close()
}

// ExecuteQuery executes a query with parameters
func (c *Connection) ExecuteQuery(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	c.logger.WithFields(logrus.Fields{
		"query": query,
		"args":  args,
	}).Debug("Executing query")

	return c.db.QueryContext(ctx, query, args...)
}

// GetStats returns database connection statistics
func (c *Connection) GetStats() sql.DBStats {
	return c.db.Stats()
}

// HealthCheck performs a comprehensive health check
func (c *Connection) HealthCheck(ctx context.Context) error {
	// Check basic connectivity
	if err := c.Ping(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	// Check if we can execute a simple query
	rows, err := c.ExecuteQuery(ctx, "SELECT 1")
	if err != nil {
		return fmt.Errorf("test query failed: %w", err)
	}
	defer rows.Close()

	// Log connection stats
	stats := c.GetStats()
	c.logger.WithFields(logrus.Fields{
		"open_connections": stats.OpenConnections,
		"idle_connections": stats.Idle,
		"in_use":           stats.InUse,
	}).Info("Database health check passed")

	return nil
}
