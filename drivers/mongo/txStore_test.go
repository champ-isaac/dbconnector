package mongo

import (
	"bytes"
	"context"
	"log"
	"math"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

func newTestTx(t *testing.T, srv *fakeServer) *TxStore {
	t.Helper()
	tx, err := newTestStore(t, srv).BeginTransaction(context.Background())
	require.NoError(t, err)
	return tx.(*TxStore)
}

func has(raw bson.Raw, key string) bool {
	_, err := raw.LookupErr(key)
	return err == nil
}

func TestTxStore_RunsInOneTransaction(t *testing.T) {
	srv := newFakeServer(t)
	srv.configure(func(s *fakeServer) { s.findBatch = []bson.M{{"_id": 1, "name": "a"}} })
	tx := newTestTx(t, srv)

	require.NoError(t, tx.Create("accounts", bson.M{"_id": 1, "name": "a"}))
	got, err := tx.Read("accounts", bson.M{"_id": 1})
	require.NoError(t, err)
	require.NoError(t, tx.Ping())
	require.NoError(t, tx.Commit())

	rows, ok := got.([]bson.M)
	require.True(t, ok)
	require.Len(t, rows, 1)
	assert.Equal(t, "a", rows[0]["name"])

	insert := srv.commands("insert")[0]
	assert.Equal(t, "testdb", insert.db)
	assert.Equal(t, "accounts", insert.body.Lookup("insert").StringValue())
	assert.True(t, has(insert.body, "lsid"), "the insert carries the session")
	assert.True(t, insert.body.Lookup("startTransaction").Boolean(), "the first command starts the transaction")
	assert.False(t, insert.body.Lookup("autocommit").Boolean())

	find := srv.commands("find")[0]
	assert.Equal(t, insert.body.Lookup("txnNumber"), find.body.Lookup("txnNumber"), "same transaction")
	assert.False(t, has(find.body, "startTransaction"), "only the first command starts it")
	assert.EqualValues(t, 1, find.body.Lookup("filter", "_id").AsInt64())

	assert.True(t, has(srv.commands("ping")[0].body, "txnNumber"), "Ping runs in the transaction too")
	assert.Len(t, srv.commands("commitTransaction"), 1)
	assert.Empty(t, srv.commands("abortTransaction"))
}

func TestTxStore_UpdateAndDelete(t *testing.T) {
	srv := newFakeServer(t)
	tx := newTestTx(t, srv)

	require.NoError(t, tx.Update("accounts", bson.M{"_id": 1}, bson.M{"name": "z"}))
	require.NoError(t, tx.Delete("accounts", bson.M{"_id": 2}))

	update := srv.commands("update")[0]
	assert.Equal(t, "accounts", update.body.Lookup("update").StringValue())
	assert.Equal(t, "z", update.seqs["updates"][0].Lookup("u", "$set", "name").StringValue())
	assert.True(t, update.body.Lookup("startTransaction").Boolean())

	del := srv.commands("delete")[0]
	assert.Equal(t, "accounts", del.body.Lookup("delete").StringValue())
	assert.EqualValues(t, 2, del.seqs["deletes"][0].Lookup("q", "_id").AsInt64())
	assert.Equal(t, update.body.Lookup("txnNumber"), del.body.Lookup("txnNumber"))
}

// The update document sent to the server must be exactly: query = the caller's filter,
// update = {$set: data}. Wrapping the filter in $set makes MongoDB reject the query
// ("unknown top level operator: $set").
func TestTxStore_Update_SendsFilterAndSetUnchanged(t *testing.T) {
	srv := newFakeServer(t)
	tx := newTestTx(t, srv)

	data := bson.D{{Key: "name", Value: "z"}, {Key: "age", Value: 3}} // ordered, so the bytes are comparable
	require.NoError(t, tx.Update("accounts", bson.M{"_id": 1}, data))

	cmds := srv.commands("update")
	require.Len(t, cmds, 1)
	ups := cmds[0].seqs["updates"]
	require.Len(t, ups, 1)

	wantFilter, err := bson.Marshal(bson.M{"_id": 1})
	require.NoError(t, err)
	assert.Equal(t, bson.Raw(wantFilter), ups[0].Lookup("q").Document(), "the filter is sent as is")

	wantUpdate, err := bson.Marshal(bson.D{{Key: "$set", Value: data}})
	require.NoError(t, err)
	assert.Equal(t, bson.Raw(wantUpdate), ups[0].Lookup("u").Document(), "the update is exactly {$set: data}")
	assert.True(t, ups[0].Lookup("multi").Boolean(), "UpdateMany updates every match")
}

// An Inc update sends exactly {$inc: {...}} (no $set) and keeps the filter as is.
func TestTxStore_Update_IncSendsInc(t *testing.T) {
	srv := newFakeServer(t)
	tx := newTestTx(t, srv)

	require.NoError(t, tx.Update("accounts", bson.D{{Key: "id", Value: "A"}}, Inc{"balance": -50.25}))

	cmds := srv.commands("update")
	require.Len(t, cmds, 1)
	ups := cmds[0].seqs["updates"]
	require.Len(t, ups, 1)

	wantFilter, err := bson.Marshal(bson.D{{Key: "id", Value: "A"}})
	require.NoError(t, err)
	assert.Equal(t, bson.Raw(wantFilter), ups[0].Lookup("q").Document())

	wantUpdate, err := bson.Marshal(bson.D{{Key: "$inc", Value: bson.D{{Key: "balance", Value: -50.25}}}})
	require.NoError(t, err)
	assert.Equal(t, bson.Raw(wantUpdate), ups[0].Lookup("u").Document(), "the update is exactly {$inc: delta}")
	_, hasSet := ups[0].Lookup("u").Document().LookupErr("$set")
	assert.Error(t, hasSet, "no $set is sent")
}

func TestTxStore_Update_IncMatchingNothingIsAnError(t *testing.T) {
	srv := newFakeServer(t)
	srv.configure(func(s *fakeServer) { s.noMatch = true })
	tx := newTestTx(t, srv)

	assert.ErrorIs(t, tx.Update("accounts", bson.M{"id": "missing"}, Inc{"balance": 5.0}), ErrNoMatch)
	// the $set path keeps its old behaviour: no error when nothing matches
	assert.NoError(t, tx.Update("accounts", bson.M{"id": "missing"}, bson.M{"balance": 5.0}))
}

// A filter that matches several documents (a duplicate id) is an error, so a transfer rolls back.
func TestTxStore_Update_IncMatchingManyIsAnError(t *testing.T) {
	srv := newFakeServer(t)
	srv.configure(func(s *fakeServer) { s.manyMatch = true })
	tx := newTestTx(t, srv)

	err := tx.Update("accounts", bson.M{"id": "dup"}, Inc{"balance": 5.0})
	assert.ErrorIs(t, err, ErrMultipleMatch)
	assert.NotErrorIs(t, err, ErrNoMatch)
	// the $set path keeps its old behaviour: several matches are fine
	assert.NoError(t, tx.Update("accounts", bson.M{"id": "dup"}, bson.M{"balance": 5.0}))
}

// An Inc without fields would be a silent no-op on MongoDB (it accepts an empty $inc).
func TestTxStore_Update_EmptyIncIsAnError(t *testing.T) {
	srv := newFakeServer(t)
	tx := newTestTx(t, srv)

	for name, inc := range map[string]Inc{"empty": {}, "nil": nil} {
		t.Run(name, func(t *testing.T) {
			err := tx.Update("accounts", bson.M{"id": "A"}, inc)
			assert.ErrorIs(t, err, ErrEmptyInc)
			assert.NotErrorIs(t, err, ErrNoMatch)
			assert.NotErrorIs(t, err, ErrMultipleMatch)
		})
	}
	assert.Empty(t, srv.commands("update"), "nothing reaches the server")

	// one field is enough
	require.NoError(t, tx.Update("accounts", bson.M{"id": "A"}, Inc{"balance": 1.0}))
	assert.Len(t, srv.commands("update"), 1)
}

// Inc validation, before anything is sent: no fields, or a value that is NaN or infinite.
func TestTxStore_Update_IncValidation(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	d128NaN, d128Inf := bson.NewDecimal128(0x7c00000000000000, 0), bson.NewDecimal128(0x7800000000000000, 0)
	d128Ok, err := bson.ParseDecimal128("12.5")
	require.NoError(t, err)

	tests := []struct {
		name    string
		inc     Inc
		wantErr error  // nil = accepted
		wantMsg string // part of the message, e.g. the field name
	}{
		{name: "NaN", inc: Inc{"balance": nan}, wantErr: ErrNonFiniteInc, wantMsg: `"balance"`},
		{name: "+Inf", inc: Inc{"balance": inf}, wantErr: ErrNonFiniteInc},
		{name: "-Inf", inc: Inc{"balance": -inf}, wantErr: ErrNonFiniteInc},
		{name: "float32 NaN", inc: Inc{"balance": float32(nan)}, wantErr: ErrNonFiniteInc},
		{name: "float32 +Inf", inc: Inc{"balance": float32(inf)}, wantErr: ErrNonFiniteInc},
		{name: "Decimal128 NaN", inc: Inc{"balance": d128NaN}, wantErr: ErrNonFiniteInc},
		{name: "Decimal128 Inf", inc: Inc{"balance": d128Inf}, wantErr: ErrNonFiniteInc},
		{name: "bad value among good ones", inc: Inc{"a": 1.0, "z": nan, "m": 2}, wantErr: ErrNonFiniteInc, wantMsg: `"z"`},
		{name: "first bad field by name", inc: Inc{"b": inf, "a": nan}, wantErr: ErrNonFiniteInc, wantMsg: `"a"`},
		{name: "empty", inc: Inc{}, wantErr: ErrEmptyInc},
		{name: "nil", inc: nil, wantErr: ErrEmptyInc},

		{name: "float", inc: Inc{"balance": 12.5}},
		{name: "negative float", inc: Inc{"balance": -12.5}},
		{name: "zero", inc: Inc{"balance": 0.0}},
		{name: "largest finite float", inc: Inc{"balance": math.MaxFloat64}},
		{name: "smallest float", inc: Inc{"balance": math.SmallestNonzeroFloat64}},
		{name: "float32", inc: Inc{"balance": float32(1.5)}},
		{name: "ints", inc: Inc{"a": 1, "b": int32(2), "c": int64(-3)}},
		{name: "Decimal128", inc: Inc{"balance": d128Ok}},
		{name: "several fields", inc: Inc{"a": 1.0, "b": 2.0}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newFakeServer(t)
			tx := newTestTx(t, srv)

			err := tx.Update("accounts", bson.M{"id": "A"}, tc.inc)

			if tc.wantErr == nil {
				require.NoError(t, err)
				assert.Len(t, srv.commands("update"), 1, "a valid Inc is sent")
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
			assert.Contains(t, err.Error(), tc.wantMsg)
			for _, other := range []error{ErrEmptyInc, ErrNonFiniteInc, ErrNoMatch, ErrMultipleMatch} {
				if other != tc.wantErr {
					assert.NotErrorIs(t, err, other)
				}
			}
			assert.Empty(t, srv.commands("update"), "nothing reaches the server")
		})
	}
}

func TestTxStore_Update_IncServerErrorIsReturned(t *testing.T) {
	srv := newFakeServer(t)
	srv.setFail("update")
	tx := newTestTx(t, srv)

	err := tx.Update("accounts", bson.M{"id": "A"}, Inc{"balance": 5.0})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNoMatch)
}

func TestTxStore_Rollback(t *testing.T) {
	srv := newFakeServer(t)
	tx := newTestTx(t, srv)
	require.NoError(t, tx.Create("accounts", bson.M{"_id": 1}))

	require.NoError(t, tx.Rollback())

	assert.Len(t, srv.commands("abortTransaction"), 1)
	assert.Empty(t, srv.commands("commitTransaction"))
	assert.Error(t, tx.Commit(), "a rolled-back transaction cannot be committed")
}

func TestTxStore_Disconnect(t *testing.T) {
	srv := newFakeServer(t)
	tx := newTestTx(t, srv)
	require.NoError(t, tx.Create("accounts", bson.M{"_id": 1}))

	require.NoError(t, tx.Disconnect())

	assert.Len(t, srv.commands("abortTransaction"), 1, "Disconnect rolls the open transaction back")
	assert.Error(t, tx.Commit(), "the session is over")
}

// Disconnect always succeeds: a failed abort is ignored on purpose.
func TestTxStore_Disconnect_IgnoresAbortError(t *testing.T) {
	srv := newFakeServer(t)
	tx := newTestTx(t, srv)
	require.NoError(t, tx.Create("accounts", bson.M{"_id": 1}))
	srv.setFail("abortTransaction")

	assert.NoError(t, tx.Disconnect())
	assert.Len(t, srv.commands("abortTransaction"), 1)
}

func TestTxStore_ServerErrorsAreReturned(t *testing.T) {
	for _, cmd := range []string{"insert", "find", "update", "delete", "ping", "commitTransaction"} {
		t.Run(cmd, func(t *testing.T) {
			srv := newFakeServer(t)
			tx := newTestTx(t, srv)
			if cmd == "commitTransaction" {
				// with no command sent yet the driver ends the transaction without asking the server
				require.NoError(t, tx.Create("c", bson.M{"a": 1}))
			}
			srv.setFail(cmd)
			var err error
			switch cmd {
			case "insert":
				err = tx.Create("c", bson.M{"a": 1})
			case "find":
				var got any
				got, err = tx.Read("c", bson.M{})
				assert.Nil(t, got)
			case "update":
				err = tx.Update("c", bson.M{}, bson.M{"a": 1})
			case "delete":
				err = tx.Delete("c", bson.M{})
			case "ping":
				err = tx.Ping()
			case "commitTransaction":
				err = tx.Commit()
			}
			assert.ErrorContains(t, err, "fake failure")
		})
	}
}

// The driver never reports a failed abortTransaction (the spec says to ignore it), so
// neither does Rollback. The abort is still sent.
func TestTxStore_Rollback_IgnoresServerError(t *testing.T) {
	srv := newFakeServer(t)
	tx := newTestTx(t, srv)
	require.NoError(t, tx.Create("c", bson.M{"a": 1}))
	srv.setFail("abortTransaction")

	assert.NoError(t, tx.Rollback())
	assert.Len(t, srv.commands("abortTransaction"), 1)
}

func TestTxStore_Read_DecodeErrorClosesCursor(t *testing.T) {
	srv := newFakeServer(t)
	srv.configure(func(s *fakeServer) { s.badBatch, s.cursorID = true, 77 })
	srv.setFail("killCursors")
	tx := newTestTx(t, srv)

	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)

	got, err := tx.Read("c", bson.M{})
	assert.Nil(t, got)
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "fake failure", "the cursor-close error is logged, not returned")
	assert.Len(t, srv.commands("killCursors"), 1)
	assert.Contains(t, logs.String(), "error when cursor closing")
}

func TestStore_BeginTransaction_StartTransactionError(t *testing.T) {
	srv := newFakeServer(t)
	// the session inherits the client's write concern, and transactions need an acknowledged one
	s := newTestStore(t, srv, func(o *options.ClientOptions) { o.SetWriteConcern(writeconcern.Unacknowledged()) })

	tx, err := s.BeginTransaction(context.Background())
	assert.ErrorContains(t, err, "unacknowledged")
	assert.Nil(t, tx)
}

func TestTxStore_ExecWithSql_NotSupported(t *testing.T) {
	assert.PanicsWithValue(t, "ExecWithSql does not implement for mongo yet", func() {
		_, _ = (&TxStore{}).ExecWithSql("select 1")
	})
}
