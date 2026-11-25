package sql

import (
	"fmt"
	"testing"
	"time"
)

// Memory benchmarks: go test ./drivers/sql -run '^$' -bench . -benchmem
// They need no database. Row scanning runs on every query; the driver below it (lib/pq)
// is not measured here.

// benchRows is a rowScanner over rows of realistic values. query_test.go's fakeRows is not
// used because it fills every cell with a small int, and Go does not allocate for those,
// which would hide what strings, times and byte slices cost.
type benchRows struct {
	cols []string
	data [][]any
	i    int
}

func (f *benchRows) Columns() ([]string, error) { return f.cols, nil }
func (f *benchRows) Next() bool                 { f.i++; return f.i <= len(f.data) }
func (f *benchRows) Err() error                 { return nil }
func (f *benchRows) Scan(dest ...any) error {
	for j, d := range dest {
		*(d.(*any)) = f.data[f.i-1][j]
	}
	return nil
}

// mixedRow has the column types a PostgreSQL table usually has.
func mixedRow(i int) []any {
	return []any{int64(i), "account-" + fmt.Sprint(i), 12.5, true, time.Unix(1_700_000_000, 0), []byte("payload"), nil, "EUR"}
}

func BenchmarkScanRows(b *testing.B) {
	names := []string{"id", "name", "balance", "active", "created_at", "meta", "note", "currency"}
	for _, rows := range []int{1, 100, 10_000} {
		data := make([][]any, rows)
		for i := range data {
			data[i] = mixedRow(i)
		}
		b.Run(fmt.Sprintf("rows=%d/cols=%d", rows, len(names)), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := scanRows(&benchRows{cols: names, data: data}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
