package storage

import "context"

// DB defines the common database operations interface that can be implemented
// by different database drivers (PostgreSQL, MySQL, etc.)
type DB interface {
	// Query operations
	QueryStruct(ctx context.Context, dst interface{}, sql string, args ...interface{}) error
	QueryStructs(ctx context.Context, dst interface{}, sql string, args ...interface{}) error

	// Exec executes a query without returning any rows
	Exec(ctx context.Context, sql string, args ...interface{}) error

	// Close releases any resources
	Close()
}
