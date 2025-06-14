//go:generate mockgen -source=contract.go -destination=mocks_test.go -package=postgres
package postgres

import (
	"context"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type (
	trManager interface {
		DefaultTrOrDB(ctx context.Context, db trmpgx.Tr) trmpgx.Tr
	}
	Queryer interface {
		Exec(ctx context.Context, sql string, arguments ...interface{}) (commandTag pgconn.CommandTag, err error)
		Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
		QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	}
)
