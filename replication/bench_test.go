package replication

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/champ-isaac/dbconnector/factory"
)

// Memory benchmarks: go test ./replication -run '^$' -bench . -benchmem
// Connect needs a server, so the stubbed connect measures only what this package adds
// around it: building the replication=database connection string.

func benchConfig() factory.Config {
	return factory.Config{Type: factory.DbTypeSql, Schema: "postgres", Host: "db.internal", Port: 5432, Db: "data_privacy", Username: "user", Password: "user!@#", SslMode: true}
}

func BenchmarkConnString(b *testing.B) {
	cfg := benchConfig()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ConnString(cfg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConnectStubbed(b *testing.B) {
	orig := connect
	connect = func(context.Context, string) (*pgconn.PgConn, error) { return nil, nil }
	b.Cleanup(func() { connect = orig })
	cfg, ctx := benchConfig(), context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Connect(ctx, cfg); err != nil {
			b.Fatal(err)
		}
	}
}
