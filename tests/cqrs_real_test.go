//go:build integration

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/champ-isaac/dbconnector/abstract"
	"github.com/champ-isaac/dbconnector/drivers/sql"
	"github.com/champ-isaac/dbconnector/factory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The command side moves the money, the query side reads the result. Both are separate
// connections made from their own Config (here the same server; in production the query
// side would point at a replica, where a read can lag a commit).
func TestCQRS_TransferThenQuery_InMongo(t *testing.T) {
	skipIfNoMongo(t)
	cmd, err := factory.NewCommandStore(mongoConfig())
	require.NoError(t, err)
	t.Cleanup(func() { _ = cmd.Disconnect() })
	query, err := factory.NewQueryStore(mongoConfig())
	require.NoError(t, err)
	t.Cleanup(func() { _ = query.Disconnect() })

	_, canWrite := any(query).(abstract.Writer)
	require.False(t, canWrite, "the query side must not offer writes")

	from, to := "cqrs-from-"+t.Name(), "cqrs-to-"+t.Name()
	require.NoError(t, cmd.Create(collection, map[string]any{"id": from, "balance": 200.5}))
	require.NoError(t, cmd.Create(collection, map[string]any{"id": to, "balance": 100.25}))
	t.Cleanup(func() {
		_ = cmd.Delete(collection, map[string]any{"id": from})
		_ = cmd.Delete(collection, map[string]any{"id": to})
	})

	require.NoError(t, TransferBalanceInMongo(cmd, context.Background(), from, to, 50.25))

	gotFrom, err := ReadBalanceInMongo(query, from)
	require.NoError(t, err)
	gotTo, err := ReadBalanceInMongo(query, to)
	require.NoError(t, err)
	assert.Equal(t, 150.25, gotFrom)
	assert.Equal(t, 150.5, gotTo)
	assert.NoError(t, query.Ping())
	assert.NoError(t, cmd.Ping())
}

// PostgreSQL refuses a plain write on a QueryConnString connection, and reads work. This is
// a guard against accidents only: BEGIN READ WRITE overrides it (use a read-only role to enforce).
func TestQueryConnString_RefusesWrites_InPostgres(t *testing.T) {
	skipIfNoServer(t, "PostgreSQL", postgresPort)
	cfg := postgresConfig()
	drv, err := sql.NewDriver(context.Background(), factory.QueryConnString(cfg), 1, 1, time.Second, time.Minute)
	require.NoError(t, err)
	t.Cleanup(func() { _ = drv.Close() })

	_, err = drv.Exec("INSERT INTO accounts (first_name, last_name) VALUES ('cqrs', 'blocked')")
	assert.ErrorContains(t, err, "read-only transaction")
	_, err = drv.Exec("DELETE FROM accounts WHERE first_name = 'cqrs'")
	assert.ErrorContains(t, err, "read-only transaction")

	rows, err := drv.Query("SELECT 1")
	require.NoError(t, err)
	assert.NoError(t, rows.Close())

	// and the factory's query side works against the same server
	q, err := factory.NewQueryStore(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Disconnect() })
	_, err = q.Read("accounts", map[string]any{"first_name": "cqrs"})
	assert.NoError(t, err)
	assert.NoError(t, q.Ping())
}
