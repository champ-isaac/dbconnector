//go:build integration

package tests

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pglogrepl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/champ-isaac/dbconnector/factory"
	"github.com/champ-isaac/dbconnector/replication"
)

// Needs a server that accepts a replication=database connection and a role with the
// REPLICATION attribute (or a superuser). The default wal_level=replica is enough: the test
// only runs IDENTIFY_SYSTEM, and only creating a logical slot needs wal_level=logical.
// Set PG_HOST, PG_DB, PG_USER, PG_PASSWORD (and PG_PORT) to run it.
func TestReplicationConnectInPostgres(t *testing.T) {
	if os.Getenv("PG_HOST") == "" {
		t.Skip("PG_HOST not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cfg, err := factory.FromEnv(ctx, "PG_")
	require.NoError(t, err)
	conn, err := replication.Connect(ctx, cfg)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }() // runs before cancel(), so ctx is still live

	sys, err := pglogrepl.IdentifySystem(ctx, conn)
	require.NoError(t, err)
	require.NotEmpty(t, sys.SystemID)
	require.NotZero(t, sys.Timeline)
}

// Needs wal_level=logical (creating a logical slot fails on a server without it) and the same
// PG_* variables as above. The slot is temporary, so PostgreSQL removes it when the connection
// ends on any path (a failed or timed-out test, a kill, a create whose reply was lost). The test
// also drops it explicitly and checks on the server that it existed and is gone.
//
// The name has no "InPostgres" on purpose: the integration-postgres CI job and the README
// run every such test on a default wal_level=replica server, where this one would fail.
func TestReplicationSlotCreateAndDrop(t *testing.T) {
	if os.Getenv("PG_HOST") == "" {
		t.Skip("PG_HOST not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel) // registered first, so it runs last: the cleanups below still have a live ctx

	cfg, err := factory.FromEnv(ctx, "PG_")
	require.NoError(t, err)
	conn, err := replication.Connect(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(ctx) })

	slotCount := func(slot string) string {
		results, err := conn.Exec(ctx, "SELECT count(*) FROM pg_replication_slots WHERE slot_name = '"+slot+"'").ReadAll()
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Len(t, results[0].Rows, 1)
		return string(results[0].Rows[0][0])
	}

	slot := fmt.Sprintf("dbconnector_test_%d", time.Now().UnixNano())
	res, err := pglogrepl.CreateReplicationSlot(ctx, conn, slot, "pgoutput",
		pglogrepl.CreateReplicationSlotOptions{Temporary: true, Mode: pglogrepl.LogicalReplication})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, pglogrepl.DropReplicationSlot(ctx, conn, slot, pglogrepl.DropReplicationSlotOptions{Wait: true}))
		assert.Equal(t, "0", slotCount(slot), "the slot is gone from the server")
	})

	assert.Equal(t, slot, res.SlotName)
	assert.Equal(t, "pgoutput", res.OutputPlugin)
	assert.NotEmpty(t, res.ConsistentPoint)
	assert.Equal(t, "1", slotCount(slot), "the slot exists on the server")
}
