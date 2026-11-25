package replication

import (
	"testing"

	"github.com/champ-isaac/dbconnector/factory"
)

// Memory benchmark: go test ./replication -run '^$' -bench . -benchmem
// Measures building the replication=database connection string; Connect itself needs a server.

func BenchmarkConnString(b *testing.B) {
	cfg := factory.Config{Type: factory.DbTypeSql, Schema: "postgres", Host: "db.internal", Port: 5432, Db: "data_privacy", Username: "user", Password: "user!@#", SslMode: true}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ConnString(cfg); err != nil {
			b.Fatal(err)
		}
	}
}
