package sql

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDriver(t *testing.T) {
	ctx := context.Background()

	// sql.Open does not connect, so a well formed DSN works without a server
	d, err := NewDriver(ctx, "postgres://u:p@127.0.0.1:1/db?sslmode=disable", 5, 2, time.Minute, time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 5, d.Db.Stats().MaxOpenConnections)
	assert.NoError(t, d.Close())

	old := sqlOpen
	defer func() { sqlOpen = old }()
	sqlOpen = func(string, string) (*sql.DB, error) { return nil, errBoom }
	_, err = NewDriver(ctx, "ignored", 5, 2, time.Minute, time.Hour)
	assert.ErrorIs(t, err, errBoom)
}

func TestDriver_ExecQueryClose(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	d := &Driver{Ctx: context.Background(), Db: db}

	mock.ExpectExec("UPDATE t SET a = 1").WillReturnResult(sqlmock.NewResult(0, 3))
	res, err := d.Exec("UPDATE t SET a = 1")
	require.NoError(t, err)
	n, _ := res.RowsAffected()
	assert.Equal(t, int64(3), n)

	mock.ExpectQuery("SELECT a FROM t").WillReturnRows(sqlmock.NewRows([]string{"a"}).AddRow(1))
	rows, err := d.Query("SELECT a FROM t")
	require.NoError(t, err)
	assert.True(t, rows.Next())
	require.NoError(t, rows.Close())

	mock.ExpectClose()
	assert.NoError(t, d.Close())
	assert.NoError(t, mock.ExpectationsWereMet())
}

var errBoom = errors.New("boom")
