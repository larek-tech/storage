module github.com/larek-tech/storage/postgres

go 1.26

require (
	github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2 v2.0.2
	github.com/avito-tech/go-transaction-manager/trm/v2 v2.0.2
	github.com/georgysavva/scany/v2 v2.1.4
	github.com/jackc/pgx/v5 v5.9.2
	go.opentelemetry.io/otel v1.43.0
	go.opentelemetry.io/otel/trace v1.43.0
)

// CVE: bump transitive golang.org/x/crypto to address GHSA in ssh/agent
// (not used at runtime — pulled by pgxv5 driver).
replace golang.org/x/crypto => golang.org/x/crypto v0.50.0

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/metric v1.43.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/text v0.36.0 // indirect
)
