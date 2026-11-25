package factory

import (
	"context"
	"testing"
)

// Memory benchmarks: go test ./factory -run '^$' -bench . -benchmem

func benchConfig() Config {
	// A password with characters that need percent-encoding, as production passwords have.
	return Config{Type: DbTypeSql, Schema: "postgres", Host: "db.internal", Port: 5432, Db: "data_privacy", Username: "user", Password: "user!@#", SslMode: true}
}

func BenchmarkConnString(b *testing.B) {
	cfg := benchConfig()
	b.Run("ConnString", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = ConnString(cfg)
		}
	})
	b.Run("QueryConnString", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = QueryConnString(cfg)
		}
	})
}

func BenchmarkFromLookup(b *testing.B) {
	full := map[string]string{
		"PG_HOST": "db.internal", "PG_PORT": "5432", "PG_DB": "data_privacy", "PG_USER": "user", "PG_PASSWORD": " user!@# ",
		"PG_SSLMODE": "require", "PG_MAX_OPEN_CONNS": "20", "PG_MAX_IDLE_CONNS": "10", "PG_CONN_MAX_LIFETIME": "30m", "PG_CONN_MAX_IDLE_TIME": "5m",
	}
	minimal := map[string]string{"PG_HOST": "db.internal", "PG_DB": "data_privacy", "PG_USER": "user"}
	invalid := map[string]string{"PG_PORT": "x", "PG_SSLMODE": "maybe", "PG_MAX_OPEN_CONNS": "0", "PG_CONN_MAX_LIFETIME": "soon"}
	for name, env := range map[string]map[string]string{"all-set": full, "defaults": minimal, "invalid": invalid} {
		get := func(k string) string { return env[k] }
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = fromLookup(context.Background(), "PG_", get) // the invalid case returns an error on purpose
			}
		})
	}
}
