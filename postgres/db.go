package postgres

import (
	"context"
	"errors"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	tracerName = "github.com/larek-tech/storage/postgres"
)

type (
	Config interface {
		DSN() string
	}
)

type DB struct {
	pool    *pgxpool.Pool
	getter  trManager
	tracer  trace.Tracer
	withTel bool
}

type DBOption func(*DB)

func WithTelemetry(enabled bool) DBOption {
	return func(db *DB) {
		db.withTel = enabled
	}
}

func WithTracer(tracer trace.Tracer) DBOption {
	return func(db *DB) {
		db.tracer = tracer
	}
}

func New(ctx context.Context, cfg Config, opts ...DBOption) (*DB, *manager.Manager, error) {
	pool, err := pgxpool.New(ctx, cfg.DSN())
	if err != nil {
		return nil, nil, err
	}

	if err = pool.Ping(ctx); err != nil {
		return nil, nil, err
	}

	trManager := manager.Must(trmpgx.NewDefaultFactory(pool))

	db := &DB{
		pool:    pool,
		getter:  trmpgx.DefaultCtxGetter,
		tracer:  otel.Tracer(tracerName),
		withTel: false,
	}

	for _, opt := range opts {
		opt(db)
	}

	return db, trManager, nil
}

func (db *DB) startSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if !db.withTel {
		return ctx, trace.SpanFromContext(ctx)
	}
	return db.tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

func (db *DB) Close() {
	db.pool.Close()
}

func (db *DB) GetPool() *pgxpool.Pool {
	return db.pool
}

func (db *DB) query(ctx context.Context, mapper func(rows pgx.Rows) error, sql string, args ...interface{}) error {
	ctx, span := db.startSpan(ctx, "DB.Query",
		attribute.String("sql", sql),
		attribute.Int("args_count", len(args)))
	defer span.End()

	conn := db.getter.DefaultTrOrDB(ctx, db.pool)
	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return err
	}
	defer rows.Close()
	return mapper(rows)
}

func (db *DB) Exec(ctx context.Context, sql string, args ...interface{}) error {
	ctx, span := db.startSpan(ctx, "DB.Exec",
		attribute.String("sql", sql),
		attribute.Int("args_count", len(args)))
	defer span.End()

	conn := db.getter.DefaultTrOrDB(ctx, db.pool)
	result, err := conn.Exec(ctx, sql, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetAttributes(attribute.Int64("affected_rows", result.RowsAffected()))
	}
	return err
}

func (db *DB) QueryStruct(ctx context.Context, dst interface{}, sql string, args ...interface{}) error {
	ctx, span := db.startSpan(ctx, "DB.QueryStruct",
		attribute.String("sql", sql),
		attribute.Int("args_count", len(args)))
	defer span.End()

	conn := db.getter.DefaultTrOrDB(ctx, db.pool)
	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	defer rows.Close()

	err = pgxscan.ScanOne(dst, rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (db *DB) QueryStructs(ctx context.Context, dst interface{}, sql string, args ...interface{}) error {
	ctx, span := db.startSpan(ctx, "DB.QueryStructs",
		attribute.String("sql", sql),
		attribute.Int("args_count", len(args)))
	defer span.End()

	err := db.query(ctx, func(rows pgx.Rows) error {
		return pgxscan.ScanAll(dst, rows)
	}, sql, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return err
}
