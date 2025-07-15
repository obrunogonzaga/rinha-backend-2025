package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/joho/godotenv/autoload"
	"rinha-backend-2025/internal/models"
)

// Service represents a service that interacts with a database.
type Service interface {
	// Health returns a map of health status information.
	// The keys and values in the map are service-specific.
	Health() map[string]string

	// Close terminates the database connection.
	// It returns an error if the connection cannot be closed.
	Close() error

	// CreatePayment creates a new payment record
	CreatePayment(ctx context.Context, payment *models.Payment) error
	
	// UpdatePaymentStatus updates the status of a payment
	UpdatePaymentStatus(ctx context.Context, paymentID uuid.UUID, status models.PaymentStatus) error
	
	// CompletePayment updates payment with final processing details
	CompletePayment(ctx context.Context, paymentID uuid.UUID, fee float64, processorType string) error
	
	// GetPaymentSummary returns payment summary grouped by processor type
	GetPaymentSummary(ctx context.Context, startDate, endDate *time.Time) ([]models.PaymentSummary, error)
}

type service struct {
	db *sql.DB
}

var (
	database   = os.Getenv("BLUEPRINT_DB_DATABASE")
	password   = os.Getenv("BLUEPRINT_DB_PASSWORD")
	username   = os.Getenv("BLUEPRINT_DB_USERNAME")
	port       = os.Getenv("BLUEPRINT_DB_PORT")
	host       = os.Getenv("BLUEPRINT_DB_HOST")
	schema     = os.Getenv("BLUEPRINT_DB_SCHEMA")
	dbInstance *service
)

func New() Service {
	// Reuse Connection
	if dbInstance != nil {
		return dbInstance
	}
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable&search_path=%s", username, password, host, port, database, schema)
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		log.Fatal(err)
	}
	dbInstance = &service{
		db: db,
	}
	return dbInstance
}

// Health checks the health of the database connection by pinging the database.
// It returns a map with keys indicating various health statistics.
func (s *service) Health() map[string]string {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	stats := make(map[string]string)

	// Ping the database
	err := s.db.PingContext(ctx)
	if err != nil {
		stats["status"] = "down"
		stats["error"] = fmt.Sprintf("db down: %v", err)
		log.Fatalf("db down: %v", err) // Log the error and terminate the program
		return stats
	}

	// Database is up, add more statistics
	stats["status"] = "up"
	stats["message"] = "It's healthy"

	// Get database stats (like open connections, in use, idle, etc.)
	dbStats := s.db.Stats()
	stats["open_connections"] = strconv.Itoa(dbStats.OpenConnections)
	stats["in_use"] = strconv.Itoa(dbStats.InUse)
	stats["idle"] = strconv.Itoa(dbStats.Idle)
	stats["wait_count"] = strconv.FormatInt(dbStats.WaitCount, 10)
	stats["wait_duration"] = dbStats.WaitDuration.String()
	stats["max_idle_closed"] = strconv.FormatInt(dbStats.MaxIdleClosed, 10)
	stats["max_lifetime_closed"] = strconv.FormatInt(dbStats.MaxLifetimeClosed, 10)

	// Evaluate stats to provide a health message
	if dbStats.OpenConnections > 40 { // Assuming 50 is the max for this example
		stats["message"] = "The database is experiencing heavy load."
	}

	if dbStats.WaitCount > 1000 {
		stats["message"] = "The database has a high number of wait events, indicating potential bottlenecks."
	}

	if dbStats.MaxIdleClosed > int64(dbStats.OpenConnections)/2 {
		stats["message"] = "Many idle connections are being closed, consider revising the connection pool settings."
	}

	if dbStats.MaxLifetimeClosed > int64(dbStats.OpenConnections)/2 {
		stats["message"] = "Many connections are being closed due to max lifetime, consider increasing max lifetime or revising the connection usage pattern."
	}

	return stats
}

// Close closes the database connection.
// It logs a message indicating the disconnection from the specific database.
// If the connection is successfully closed, it returns nil.
// If an error occurs while closing the connection, it returns the error.
func (s *service) Close() error {
	log.Printf("Disconnected from database: %s", database)
	return s.db.Close()
}

// CreatePayment creates a new payment record in the database
func (s *service) CreatePayment(ctx context.Context, payment *models.Payment) error {
	query := `
		INSERT INTO payments (correlation_id, amount, status, requested_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`
	
	err := s.db.QueryRowContext(ctx, query, 
		payment.CorrelationID, 
		payment.Amount, 
		payment.Status, 
		payment.RequestedAt).Scan(
		&payment.ID, 
		&payment.CreatedAt, 
		&payment.UpdatedAt)
	
	if err != nil {
		return fmt.Errorf("failed to create payment: %w", err)
	}
	
	return nil
}

// UpdatePaymentStatus updates the status of a payment
func (s *service) UpdatePaymentStatus(ctx context.Context, paymentID uuid.UUID, status models.PaymentStatus) error {
	query := `UPDATE payments SET status = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`
	
	result, err := s.db.ExecContext(ctx, query, status, paymentID)
	if err != nil {
		return fmt.Errorf("failed to update payment status: %w", err)
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return fmt.Errorf("payment not found: %s", paymentID)
	}
	
	return nil
}

// CompletePayment updates payment with final processing details
func (s *service) CompletePayment(ctx context.Context, paymentID uuid.UUID, fee float64, processorType string) error {
	query := `
		UPDATE payments 
		SET status = $1, fee = $2, processor_type = $3, processed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP 
		WHERE id = $4`
	
	result, err := s.db.ExecContext(ctx, query, models.PaymentStatusCompleted, fee, processorType, paymentID)
	if err != nil {
		return fmt.Errorf("failed to complete payment: %w", err)
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return fmt.Errorf("payment not found: %s", paymentID)
	}
	
	return nil
}

// GetPaymentSummary returns payment summary grouped by processor type
func (s *service) GetPaymentSummary(ctx context.Context, startDate, endDate *time.Time) ([]models.PaymentSummary, error) {
	log.Printf("GetPaymentSummary called with startDate: %v, endDate: %v", startDate, endDate)
	
	// First, test if we can count all payments
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM payments").Scan(&count)
	if err != nil {
		log.Printf("Error counting payments: %v", err)
		return nil, fmt.Errorf("failed to count payments: %w", err)
	}
	
	log.Printf("Found %d payments in database", count)
	
	// If no payments, return empty slice
	if count == 0 {
		log.Printf("No payments found, returning empty slice")
		return []models.PaymentSummary{}, nil
	}
	
	query := `
		SELECT 
			COALESCE(processor_type, 'unknown') as processor_type,
			COALESCE(SUM(amount), 0) as total_amount,
			COALESCE(SUM(fee), 0) as total_fee,
			COUNT(*) as count
		FROM payments 
		GROUP BY processor_type 
		ORDER BY processor_type`
	
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get payment summary: %w", err)
	}
	defer rows.Close()
	
	var summaries []models.PaymentSummary
	for rows.Next() {
		var summary models.PaymentSummary
		err := rows.Scan(
			&summary.ProcessorType,
			&summary.TotalAmount,
			&summary.TotalFee,
			&summary.Count,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan payment summary: %w", err)
		}
		summaries = append(summaries, summary)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate payment summary rows: %w", err)
	}
	
	return summaries, nil
}
