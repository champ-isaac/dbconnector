package sql

import (
	"fmt"
	"testing"
	"time"
)

// Memory benchmarks: go test ./drivers/sql -run '^$' -bench . -benchmem
// They need no database. Query building and row scanning are the code in this package
// that runs on every call; the driver below it (lib/pq) is not measured here.

// cols returns n distinct column names that pass validIdent.
func cols(n int) map[string]any {
	m := make(map[string]any, n)
	for i := range n {
		m[fmt.Sprintf("col_%02d", i)] = i
	}
	return m
}

func BenchmarkBuildQuery(b *testing.B) {
	for _, n := range []int{1, 8, 32} {
		data, filter := cols(n), cols(1)
		b.Run(fmt.Sprintf("insert/cols=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, _, err := buildInsertQuery("public.accounts", data); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("select/filter=%d", n), func(b *testing.B) {
			f := cols(n)
			b.ReportAllocs()
			for b.Loop() {
				if _, _, err := buildSelectQuery("public.accounts", f); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("update/set=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, _, err := buildUpdateQuery("public.accounts", filter, data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
	b.Run("delete/filter=1", func(b *testing.B) {
		f := cols(1)
		b.ReportAllocs()
		for b.Loop() {
			if _, _, err := buildDeleteQuery("public.accounts", f); err != nil {
				b.Fatal(err)
			}
		}
	})
}

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

func BenchmarkExecOrQueryRouting(b *testing.B) {
	// queryRe runs on every ExecWithSql call, before the statement is sent.
	for _, q := range []string{"UPDATE accounts SET balance = balance - $1 WHERE id = $2", "SELECT id, balance FROM accounts WHERE id = $1"} {
		b.Run(q[:6], func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = queryRe.MatchString(q)
			}
		})
	}
}
