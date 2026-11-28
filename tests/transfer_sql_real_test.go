//go:build integration

package tests

import (
	"context"
	"testing"

	"github.com/champ-isaac/dbconnector/abstract"
	"github.com/champ-isaac/dbconnector/factory"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Needs the docker compose postgres (docker/init.sql loaded).
//
// TransferBalanceInSql updates accounts.balance, and docker/init.sql creates no such column
// (accounts has amount NUMERIC(12,2)). So the fixture adds the column, with a CHECK that makes
// an overdraft fail, and drops it again when the test ends. init.sql stays as it is. The
// amounts below are exact in binary floating point.

func sqlTransferFixture(t *testing.T) (abstract.Store, abstract.CommandStore) {
	t.Helper()
	skipIfNoServer(t, "PostgreSQL", postgresPort)
	store, err := initPostgresStore()
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Disconnect() }) // registered first, so it runs last

	_, err = store.ExecWithSql("ALTER TABLE accounts ADD COLUMN IF NOT EXISTS balance NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (balance >= 0)")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = store.ExecWithSql("ALTER TABLE accounts DROP COLUMN IF EXISTS balance") })

	cmd, err := factory.NewCommandStore(postgresConfig())
	require.NoError(t, err)
	t.Cleanup(func() { _ = cmd.Disconnect() })
	return store, cmd
}

// seedSqlAccount inserts an account with the given balance and removes it when the test ends.
func seedSqlAccount(t *testing.T, store abstract.Store, balance float64) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, store.Create(collection, map[string]any{
		"id": id, "first_name": "transfer", "last_name": "InPostgres", "balance": balance,
	}))
	t.Cleanup(func() { _ = store.Delete(collection, map[string]any{"id": id}) })
	return id
}

func sqlBalanceOf(t *testing.T, store abstract.Store, id string) decimal.Decimal {
	t.Helper()
	res, err := store.ExecWithSql("select balance::text as b from accounts where id = $1", id)
	require.NoError(t, err)
	rows, ok := res.([]any)
	require.True(t, ok, "rows of %s: got %T", id, res)
	require.Len(t, rows, 1, "account %s", id)
	b := rows[0].(map[string]any)["b"]
	if raw, isBytes := b.([]byte); isBytes {
		b = string(raw)
	}
	d, err := decimal.NewFromString(b.(string))
	require.NoError(t, err)
	return d
}

func assertSqlBalance(t *testing.T, store abstract.Store, id, want string) {
	t.Helper()
	got := sqlBalanceOf(t, store, id)
	assert.True(t, decimal.RequireFromString(want).Equal(got), "balance of %s: got %s, want %s", id, got, want)
}

func TestTransferBalanceInSql_InPostgres(t *testing.T) {
	store, cmd := sqlTransferFixture(t)
	from := seedSqlAccount(t, store, 200.5)
	to := seedSqlAccount(t, store, 100.25)

	require.NoError(t, TransferBalanceInSql(cmd, context.Background(), from, to, 50.25))

	assertSqlBalance(t, store, from, "150.25")
	assertSqlBalance(t, store, to, "150.50")
	total := sqlBalanceOf(t, store, from).Add(sqlBalanceOf(t, store, to))
	assert.True(t, decimal.RequireFromString("300.75").Equal(total), "the total is unchanged, got %s", total)
}

// The receiver id is not a UUID, so the second update fails after the first one has already
// run inside the transaction. The rollback must undo that first update.
func TestTransferBalanceInSql_InPostgres_RollsBackOnInvalidUuid(t *testing.T) {
	store, cmd := sqlTransferFixture(t)
	from := seedSqlAccount(t, store, 200.5)
	to := seedSqlAccount(t, store, 100.25)

	err := TransferBalanceInSql(cmd, context.Background(), from, "not-a-uuid", 50.25)

	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid input syntax for type uuid")
	assertSqlBalance(t, store, from, "200.50")
	assertSqlBalance(t, store, to, "100.25")
}

// The receiver id is a valid UUID that matches no account. Its update changes 0 rows, which
// used to commit the debit and lose the money. Now the transfer fails and rolls back.
func TestTransferBalanceInSql_InPostgres_RejectsUnknownReceiver(t *testing.T) {
	store, cmd := sqlTransferFixture(t)
	from := seedSqlAccount(t, store, 200.5)
	unknown := uuid.NewString()

	err := TransferBalanceInSql(cmd, context.Background(), from, unknown, 50.25)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAccountNotFound)
	assertSqlBalance(t, store, from, "200.50")
}

// The sender id is a valid UUID that matches no account, so the debit changes 0 rows. The
// transfer fails and rolls back, and the receiver is not credited.
func TestTransferBalanceInSql_InPostgres_RejectsUnknownSender(t *testing.T) {
	store, cmd := sqlTransferFixture(t)
	unknown := uuid.NewString()
	to := seedSqlAccount(t, store, 100.25)

	err := TransferBalanceInSql(cmd, context.Background(), unknown, to, 50.25)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAccountNotFound)
	assertSqlBalance(t, store, to, "100.25")
}

// The sender holds 20.00, so taking 50.25 breaks the CHECK (balance >= 0) and PostgreSQL
// rejects the debit. Nothing moves.
func TestTransferBalanceInSql_InPostgres_RejectsOverdraft(t *testing.T) {
	store, cmd := sqlTransferFixture(t)
	from := seedSqlAccount(t, store, 20)
	to := seedSqlAccount(t, store, 100.25)

	err := TransferBalanceInSql(cmd, context.Background(), from, to, 50.25)

	require.Error(t, err)
	assert.ErrorContains(t, err, "violates check constraint")
	assertSqlBalance(t, store, from, "20.00")
	assertSqlBalance(t, store, to, "100.25")
}

// The sender and the receiver are the same account. The debit and the credit both match its
// row and cancel out, so the transfer succeeds and the balance is unchanged.
func TestTransferBalanceInSql_InPostgres_SameAccountKeepsBalance(t *testing.T) {
	store, cmd := sqlTransferFixture(t)
	id := seedSqlAccount(t, store, 200.5)

	err := TransferBalanceInSql(cmd, context.Background(), id, id, 50.25)

	require.NoError(t, err)
	assertSqlBalance(t, store, id, "200.50")
}
