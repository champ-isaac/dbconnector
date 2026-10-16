package sql

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidIdent(t *testing.T) {
	for _, ok := range []string{"accounts", "_t1", "public.accounts", "A_b9"} {
		assert.NoError(t, validIdent(ok), ok)
	}
	for _, bad := range []string{"", "1abc", "a b", "a;drop table x", "a.b.c", "a-b", "a.", ".a", `"quoted"`, "x'--"} {
		assert.ErrorIs(t, validIdent(bad), ErrInvalidIdentifier, bad)
	}
}

func TestAsMap(t *testing.T) {
	m, err := asMap(map[string]any{"a": 1}, "data", "create")
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{"a": 1}, m)

	_, err = asMap("nope", "filter", "read")
	assert.EqualError(t, err, "invalid filter format to exec read, expecting map[string]any")
}

func TestBuildInsertQuery(t *testing.T) {
	q, args, err := buildInsertQuery("public.t", map[string]any{"b": 2, "a": 1})
	assert.NoError(t, err)
	assert.Equal(t, "INSERT INTO public.t (a, b) VALUES ($1, $2)", q)
	assert.Equal(t, []any{1, 2}, args, "values follow the sorted column order")

	_, _, err = buildInsertQuery("t", nil)
	assert.Error(t, err)
	_, _, err = buildInsertQuery("t; drop table x", map[string]any{"a": 1})
	assert.ErrorIs(t, err, ErrInvalidIdentifier)
	_, _, err = buildInsertQuery("t", map[string]any{"a) values (1); --": 1})
	assert.ErrorIs(t, err, ErrInvalidIdentifier)
}

func TestBuildSelectQuery(t *testing.T) {
	q, args, err := buildSelectQuery("t", map[string]any{"b": 2, "a": 1})
	assert.NoError(t, err)
	assert.Equal(t, "SELECT * FROM t WHERE a = $1 AND b = $2", q)
	assert.Equal(t, []any{1, 2}, args)

	q, args, err = buildSelectQuery("t", nil)
	assert.NoError(t, err)
	assert.Equal(t, "SELECT * FROM t", q, "no filter selects everything")
	assert.Empty(t, args)

	_, _, err = buildSelectQuery("t", map[string]any{"a or 1=1": 1})
	assert.ErrorIs(t, err, ErrInvalidIdentifier)
}

func TestBuildUpdateQuery(t *testing.T) {
	q, args, err := buildUpdateQuery("t", map[string]any{"id": 7, "ok": true}, map[string]any{"name": "n", "age": 3})
	assert.NoError(t, err)
	assert.Equal(t, "UPDATE t SET age = $1, name = $2 WHERE id = $3 AND ok = $4", q)
	assert.Equal(t, []any{3, "n", 7, true}, args)

	_, _, err = buildUpdateQuery("t", map[string]any{"id": 1}, nil)
	assert.Error(t, err)
	_, _, err = buildUpdateQuery("t", nil, map[string]any{"a": 1})
	assert.ErrorIs(t, err, ErrEmptyFilter)
	_, _, err = buildUpdateQuery("t", map[string]any{"id": 1}, map[string]any{"a b": 1})
	assert.ErrorIs(t, err, ErrInvalidIdentifier)
	_, _, err = buildUpdateQuery("t", map[string]any{"i d": 1}, map[string]any{"a": 1})
	assert.ErrorIs(t, err, ErrInvalidIdentifier)
}

func TestBuildDeleteQuery(t *testing.T) {
	q, args, err := buildDeleteQuery("t", map[string]any{"id": 7})
	assert.NoError(t, err)
	assert.Equal(t, "DELETE FROM t WHERE id = $1", q)
	assert.Equal(t, []any{7}, args)

	_, _, err = buildDeleteQuery("t", nil)
	assert.ErrorIs(t, err, ErrEmptyFilter, "delete without a filter would wipe the table")
	_, _, err = buildDeleteQuery("t", map[string]any{"x;": 1})
	assert.ErrorIs(t, err, ErrInvalidIdentifier)
}

type fakeRows struct {
	cols    []string
	colsErr error
	left    int
	scanErr error
	err     error
}

func (f *fakeRows) Columns() ([]string, error) { return f.cols, f.colsErr }
func (f *fakeRows) Next() bool                 { f.left--; return f.left >= 0 }
func (f *fakeRows) Scan(dest ...any) error {
	for i := range dest {
		*(dest[i].(*any)) = i
	}
	return f.scanErr
}
func (f *fakeRows) Err() error { return f.err }

func TestScanRows(t *testing.T) {
	got, err := scanRows(&fakeRows{cols: []string{"a", "b"}, left: 2})
	assert.NoError(t, err)
	assert.Equal(t, []any{map[string]any{"a": 0, "b": 1}, map[string]any{"a": 0, "b": 1}}, got)

	_, err = scanRows(&fakeRows{colsErr: errors.New("closed")})
	assert.EqualError(t, err, "closed")
	_, err = scanRows(&fakeRows{cols: []string{"a"}, left: 1, scanErr: errors.New("scan")})
	assert.EqualError(t, err, "scan")
	_, err = scanRows(&fakeRows{cols: []string{"a"}, err: errors.New("iteration")})
	assert.EqualError(t, err, "iteration")
}
