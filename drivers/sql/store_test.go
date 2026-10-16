package sql

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/champ-isaac/dbconnector/abstract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type harness struct {
	store abstract.Store
	mock  sqlmock.Sqlmock
	tx    bool
}

func newHarness(t *testing.T, tx bool) harness {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual), sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	var s abstract.Store = NewStore(&Driver{Ctx: context.Background(), Db: db})
	if tx {
		mock.ExpectBegin()
		s, err = s.(abstract.TransactionManager).BeginTransaction(context.Background())
		require.NoError(t, err)
	}
	return harness{store: s, mock: mock, tx: tx}
}

// eachStore runs a test against the plain Store and against a TxStore, they must behave the same.
func eachStore(t *testing.T, fn func(t *testing.T, h harness)) {
	for _, tx := range []bool{false, true} {
		t.Run(fmt.Sprintf("tx=%v", tx), func(t *testing.T) {
			h := newHarness(t, tx)
			fn(t, h)
			assert.NoError(t, h.mock.ExpectationsWereMet())
		})
	}
}

func TestStore_Create(t *testing.T) {
	eachStore(t, func(t *testing.T, h harness) {
		h.mock.ExpectExec("INSERT INTO t (a, b) VALUES ($1, $2)").WithArgs(1, "x").WillReturnResult(sqlmock.NewResult(1, 1))
		assert.NoError(t, h.store.Create("t", map[string]any{"b": "x", "a": 1}))

		h.mock.ExpectExec("INSERT INTO t (a) VALUES ($1)").WithArgs(1).WillReturnError(errBoom)
		assert.ErrorIs(t, h.store.Create("t", map[string]any{"a": 1}), errBoom)

		assert.Error(t, h.store.Create("t", "not a map"))
		assert.ErrorIs(t, h.store.Create("bad name", map[string]any{"a": 1}), ErrInvalidIdentifier)
	})
}

func TestStore_Read(t *testing.T) {
	eachStore(t, func(t *testing.T, h harness) {
		h.mock.ExpectQuery("SELECT * FROM t WHERE id = $1").WithArgs(7).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(7, "n").AddRow(7, "m"))
		got, err := h.store.Read("t", map[string]any{"id": 7})
		assert.NoError(t, err)
		assert.Equal(t, []any{
			map[string]any{"id": int64(7), "name": "n"},
			map[string]any{"id": int64(7), "name": "m"},
		}, got)

		h.mock.ExpectQuery("SELECT * FROM t").WillReturnRows(sqlmock.NewRows([]string{"id"}))
		got, err = h.store.Read("t", map[string]any{})
		assert.NoError(t, err)
		assert.Empty(t, got)

		h.mock.ExpectQuery("SELECT * FROM t").WillReturnError(errBoom)
		_, err = h.store.Read("t", map[string]any{})
		assert.ErrorIs(t, err, errBoom)

		_, err = h.store.Read("t", 5)
		assert.Error(t, err)
		_, err = h.store.Read("t", map[string]any{"a;b": 1})
		assert.ErrorIs(t, err, ErrInvalidIdentifier)
	})
}

func TestStore_Update(t *testing.T) {
	eachStore(t, func(t *testing.T, h harness) {
		h.mock.ExpectExec("UPDATE t SET name = $1 WHERE id = $2").WithArgs("n", 7).WillReturnResult(sqlmock.NewResult(0, 1))
		assert.NoError(t, h.store.Update("t", map[string]any{"id": 7}, map[string]any{"name": "n"}))

		h.mock.ExpectExec("UPDATE t SET name = $1 WHERE id = $2").WithArgs("n", 7).WillReturnError(errBoom)
		assert.ErrorIs(t, h.store.Update("t", map[string]any{"id": 7}, map[string]any{"name": "n"}), errBoom)

		assert.Error(t, h.store.Update("t", map[string]any{"id": 7}, "bad data"))
		assert.Error(t, h.store.Update("t", "bad filter", map[string]any{"name": "n"}))
		assert.ErrorIs(t, h.store.Update("t", map[string]any{}, map[string]any{"name": "n"}), ErrEmptyFilter)
	})
}

func TestStore_Delete(t *testing.T) {
	eachStore(t, func(t *testing.T, h harness) {
		h.mock.ExpectExec("DELETE FROM t WHERE id = $1").WithArgs(7).WillReturnResult(sqlmock.NewResult(0, 1))
		assert.NoError(t, h.store.Delete("t", map[string]any{"id": 7}))

		h.mock.ExpectExec("DELETE FROM t WHERE id = $1").WithArgs(7).WillReturnError(errBoom)
		assert.ErrorIs(t, h.store.Delete("t", map[string]any{"id": 7}), errBoom)

		assert.Error(t, h.store.Delete("t", "bad filter"))
		assert.ErrorIs(t, h.store.Delete("t", map[string]any{}), ErrEmptyFilter)
	})
}

func TestStore_ExecWithSql(t *testing.T) {
	eachStore(t, func(t *testing.T, h harness) {
		// statements that return rows are read through the query path
		h.mock.ExpectQuery("SELECT id FROM t").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
		got, err := h.store.ExecWithSql("SELECT id FROM t")
		assert.NoError(t, err)
		assert.Equal(t, []any{map[string]any{"id": int64(1)}}, got)

		h.mock.ExpectQuery("INSERT INTO t (a) VALUES (1) RETURNING id").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
		got, err = h.store.ExecWithSql("INSERT INTO t (a) VALUES (1) RETURNING id")
		assert.NoError(t, err)
		assert.Len(t, got, 1)

		// everything else is an exec
		h.mock.ExpectExec("TRUNCATE t").WillReturnResult(sqlmock.NewResult(0, 0))
		got, err = h.store.ExecWithSql("TRUNCATE t")
		assert.NoError(t, err)
		assert.Implements(t, (*sql.Result)(nil), got)

		h.mock.ExpectExec("DELETE FROM t WHERE id = $1").WithArgs(1).WillReturnError(errBoom)
		_, err = h.store.ExecWithSql("DELETE FROM t WHERE id = $1", 1)
		assert.ErrorIs(t, err, errBoom)
	})
}

func TestStore_QueryResultErrors(t *testing.T) {
	eachStore(t, func(t *testing.T, h harness) {
		rows := sqlmock.NewRows([]string{"id"}).AddRow(1).RowError(0, errBoom)
		h.mock.ExpectQuery("SELECT * FROM t").WillReturnRows(rows)
		_, err := h.store.Read("t", map[string]any{})
		assert.ErrorIs(t, err, errBoom, "an error while iterating rows is not swallowed")

		// database/sql reports a close error at the end of the rows through rows.Err()
		h.mock.ExpectQuery("SELECT * FROM t").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1).CloseError(errBoom))
		_, err = h.store.Read("t", map[string]any{})
		assert.ErrorIs(t, err, errBoom)
	})
}

func TestStore_PingAndDisconnect(t *testing.T) {
	h := newHarness(t, false)
	h.mock.ExpectPing()
	assert.NoError(t, h.store.Ping())
	h.mock.ExpectPing().WillReturnError(errBoom)
	assert.ErrorIs(t, h.store.Ping(), errBoom)
	h.mock.ExpectClose()
	assert.NoError(t, h.store.Disconnect())
	assert.NoError(t, h.mock.ExpectationsWereMet())
}

func TestStore_BeginTransactionError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin().WillReturnError(errBoom)
	_, err = NewStore(&Driver{Ctx: context.Background(), Db: db}).(abstract.TransactionManager).BeginTransaction(context.Background())
	assert.ErrorIs(t, err, errBoom)
}

func TestTxStore_Lifecycle(t *testing.T) {
	t.Run("ping is not supported in a transaction", func(t *testing.T) {
		h := newHarness(t, true)
		assert.Error(t, h.store.Ping())
	})
	t.Run("commit", func(t *testing.T) {
		h := newHarness(t, true)
		h.mock.ExpectCommit()
		assert.NoError(t, h.store.(abstract.TxStore).Commit())
		assert.NoError(t, h.mock.ExpectationsWereMet())
	})
	t.Run("rollback", func(t *testing.T) {
		h := newHarness(t, true)
		h.mock.ExpectRollback()
		assert.NoError(t, h.store.(abstract.TxStore).Rollback())
		assert.NoError(t, h.mock.ExpectationsWereMet())
	})
	t.Run("disconnect rolls back an open transaction", func(t *testing.T) {
		h := newHarness(t, true)
		h.mock.ExpectRollback()
		assert.NoError(t, h.store.Disconnect())
		assert.NoError(t, h.mock.ExpectationsWereMet())
	})
	t.Run("disconnect after commit is fine", func(t *testing.T) {
		h := newHarness(t, true)
		h.mock.ExpectCommit()
		require.NoError(t, h.store.(abstract.TxStore).Commit())
		assert.NoError(t, h.store.Disconnect())
	})
	t.Run("disconnect reports a real rollback error", func(t *testing.T) {
		h := newHarness(t, true)
		h.mock.ExpectRollback().WillReturnError(errBoom)
		assert.ErrorIs(t, h.store.Disconnect(), errBoom)
	})
}
