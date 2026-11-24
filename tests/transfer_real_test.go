//go:build integration

package tests

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/champ-isaac/dbconnector/abstract"
	"github.com/champ-isaac/dbconnector/drivers/mongo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	mdb "go.mongodb.org/mongo-driver/v2/mongo"
)

// Needs the docker compose mongodb (a replica set: transactions need one).
// All amounts below are exact in binary floating point, so == is a fair check.

func transferFixture(t *testing.T) (abstract.Store, abstract.TransactionManager) {
	t.Helper()
	store, err := initMongoStore()
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Disconnect() })
	mgr, ok := store.(abstract.TransactionManager)
	require.True(t, ok, "the mongo store must support transactions")
	return store, mgr
}

// seedAccount inserts an account with a unique id and removes it when the test ends.
func seedAccount(t *testing.T, store abstract.Store, prefix string, balance float64) string {
	t.Helper()
	id := prefix + "-" + bson.NewObjectID().Hex()
	require.NoError(t, store.Create(collection, map[string]any{"id": id, "balance": balance}))
	t.Cleanup(func() { _ = store.Delete(collection, map[string]any{"id": id}) })
	return id
}

func balanceOfAccount(t *testing.T, store abstract.Store, id string) float64 {
	t.Helper()
	rows, err := store.Read(collection, map[string]any{"id": id})
	require.NoError(t, err)
	docs, ok := rows.([]bson.M)
	require.True(t, ok)
	require.Len(t, docs, 1, "account %s", id)
	balance, ok := docs[0]["balance"].(float64)
	require.True(t, ok, "balance of %s is %T, want float64", id, docs[0]["balance"])
	return balance
}

func TestTransferBalanceInMongo_InMongo(t *testing.T) {
	store, mgr := transferFixture(t)
	from := seedAccount(t, store, "transfer-from", 200.5)
	to := seedAccount(t, store, "transfer-to", 100.25)

	require.NoError(t, TransferBalanceInMongo(mgr, context.Background(), from, to, 50.25))

	assert.Equal(t, 150.25, balanceOfAccount(t, store, from), "the sender loses exactly the amount")
	assert.Equal(t, 150.5, balanceOfAccount(t, store, to), "the receiver gains exactly the amount")
}

func TestTransferBalanceInMongo_InMongo_RoundTrip(t *testing.T) {
	store, mgr := transferFixture(t)
	a := seedAccount(t, store, "round-a", 80.75)
	b := seedAccount(t, store, "round-b", 19.25)

	require.NoError(t, TransferBalanceInMongo(mgr, context.Background(), a, b, 30.5))
	require.NoError(t, TransferBalanceInMongo(mgr, context.Background(), b, a, 30.5))

	assert.Equal(t, 80.75, balanceOfAccount(t, store, a))
	assert.Equal(t, 19.25, balanceOfAccount(t, store, b))
}

// The sender is updated first. When the receiver does not exist the whole transfer is
// rolled back, so the sender keeps its money.
func TestTransferBalanceInMongo_InMongo_RollsBackWhenReceiverIsMissing(t *testing.T) {
	store, mgr := transferFixture(t)
	from := seedAccount(t, store, "rollback-from", 200.5)

	err := TransferBalanceInMongo(mgr, context.Background(), from, "no-such-account", 50.25)

	assert.ErrorIs(t, err, mongo.ErrNoMatch)
	assert.Equal(t, 200.5, balanceOfAccount(t, store, from), "nothing was committed")
}

// Two accounts with the same id: the transfer must fail and roll back, never debit both.
func TestTransferBalanceInMongo_InMongo_RollsBackOnDuplicateId(t *testing.T) {
	store, mgr := transferFixture(t)
	dup := "dup-" + bson.NewObjectID().Hex()
	require.NoError(t, store.Create(collection, map[string]any{"id": dup, "balance": 100.0}))
	require.NoError(t, store.Create(collection, map[string]any{"id": dup, "balance": 100.0}))
	t.Cleanup(func() { _ = store.Delete(collection, map[string]any{"id": dup}) })
	to := seedAccount(t, store, "dup-to", 100.0)

	err := TransferBalanceInMongo(mgr, context.Background(), dup, to, 10.0)

	assert.ErrorIs(t, err, mongo.ErrMultipleMatch)
	rows, err := store.Read(collection, map[string]any{"id": dup})
	require.NoError(t, err)
	for _, d := range rows.([]bson.M) {
		assert.Equal(t, 100.0, d["balance"], "neither duplicate was debited")
	}
	assert.Equal(t, 100.0, balanceOfAccount(t, store, to), "the receiver was not credited")
}

// skipIfNoMongo skips the test when nothing listens where initMongoStore connects. The other
// tests in this file fail instead, because there a missing database is a setup error.
func skipIfNoMongo(t *testing.T) { t.Helper(); skipIfNoServer(t, "MongoDB", mongodbPort) }

// skipIfNoServer skips the test when nothing listens on host:port.
func skipIfNoServer(t *testing.T, what string, port int) {
	t.Helper()
	addr := net.JoinHostPort(host, fmt.Sprint(port))
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Skipf("no %s at %s (start the compose service): %v", what, addr, err)
	}
	_ = conn.Close()
}

// isTransient reports a MongoDB write conflict or other error that says "run the
// transaction again". TransferBalanceInMongo does not retry, so a caller sees these.
func isTransient(err error) bool {
	var labeled mdb.LabeledError
	return errors.As(err, &labeled) && labeled.HasErrorLabel("TransientTransactionError")
}

// Transfers in both directions at once, on the same two accounts. MongoDB aborts a
// transaction that writes a document another open transaction has written (a write
// conflict). Such a transfer fails whole and changes nothing, so the total must not move,
// and each account must change by exactly the transfers that succeeded.
func TestTransferBalanceInMongo_InMongo_ParallelTransfersConserveTotal(t *testing.T) {
	skipIfNoMongo(t)
	store, mgr := transferFixture(t)
	const (
		workers = 8
		rounds  = 10
		start   = 1000.0
		amount  = 1.0 // whole numbers: exact in binary, so == is a fair check
	)
	a := seedAccount(t, store, "par-a", start)
	b := seedAccount(t, store, "par-b", start)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var (
		okAB, okBA, conflicts atomic.Int64
		mu                    sync.Mutex
		unexpected            []error
		wg                    sync.WaitGroup
	)
	for w := 0; w < workers; w++ {
		from, to, ok := a, b, &okAB
		if w%2 == 1 {
			from, to, ok = b, a, &okBA // odd workers go the other way
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				switch err := TransferBalanceInMongo(mgr, ctx, from, to, amount); {
				case err == nil:
					ok.Add(1)
				case isTransient(err):
					conflicts.Add(1)
				default:
					mu.Lock()
					unexpected = append(unexpected, err)
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()

	t.Logf("%d transfers: %d a->b and %d b->a succeeded, %d aborted by write conflicts",
		workers*rounds, okAB.Load(), okBA.Load(), conflicts.Load())
	require.Empty(t, unexpected, "every failure must be a retryable transaction error")
	require.Positive(t, okAB.Load()+okBA.Load(), "no transfer succeeded, so nothing was checked")
	require.Equal(t, int64(workers*rounds), okAB.Load()+okBA.Load()+conflicts.Load())

	balA, balB := balanceOfAccount(t, store, a), balanceOfAccount(t, store, b)
	net := float64(okAB.Load()-okBA.Load()) * amount // money that moved from a to b
	assert.Equal(t, 2*start, balA+balB, "the total balance is conserved")
	assert.Equal(t, start-net, balA, "a changed by exactly the transfers that succeeded")
	assert.Equal(t, start+net, balB, "b changed by exactly the transfers that succeeded")
}
