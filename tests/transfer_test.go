package tests

import (
	"context"
	"database/sql/driver"
	"errors"
	"math"
	"testing"

	"github.com/champ-isaac/dbconnector/abstract"
	"github.com/champ-isaac/dbconnector/drivers/mongo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestTransferBalanceInMongo(t *testing.T) {
	ctx := context.Background()
	txStore := new(MockTxStore)
	txMgr := new(MockTxManager)

	txMgr.On("BeginTransaction", ctx).Return(txStore, nil)
	txStore.On("Update", "accounts", map[string]any{"id": "A"}, mongo.Inc{"balance": -100.25}).Return(nil).Once()
	txStore.On("Update", "accounts", map[string]any{"id": "B"}, mongo.Inc{"balance": 100.25}).Return(nil).Once()
	txStore.On("Commit").Return(nil)
	txStore.On("Disconnect").Return(nil)

	err := TransferBalanceInMongo(txMgr, ctx, "A", "B", 100.25)
	assert.NoError(t, err)

	txMgr.AssertExpectations(t)
	txStore.AssertExpectations(t)
	txStore.AssertNotCalled(t, "Read", mock.Anything, mock.Anything)
}

func TestTransferBalanceInMongo_FailOnSecondUpdate(t *testing.T) {
	ctx := context.Background()
	txStore := new(MockTxStore)
	txMgr := new(MockTxManager)

	txMgr.On("BeginTransaction", ctx).Return(txStore, nil)
	txStore.On("Update", "accounts", mock.Anything, mock.Anything).Return(nil).Once()
	txStore.On("Update", "accounts", mock.Anything, mock.Anything).Return(errors.New("db error")).Once()

	txStore.On("Rollback").Return(nil)
	txStore.On("Disconnect").Return(nil)

	err := TransferBalanceInMongo(txMgr, ctx, "A", "B", 50.0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "db error")

	txMgr.AssertExpectations(t)
	txStore.AssertExpectations(t)
	txStore.AssertNotCalled(t, "Commit")
}

// When the first update fails nothing else is written and nothing is committed.
func TestTransferBalanceInMongo_FailOnFirstUpdate(t *testing.T) {
	ctx := context.Background()
	txStore := new(MockTxStore)
	txMgr := new(MockTxManager)
	txMgr.On("BeginTransaction", ctx).Return(txStore, nil)
	txStore.On("Update", "accounts", map[string]any{"id": "A"}, mongo.Inc{"balance": -5.0}).Return(mongo.ErrNoMatch).Once()
	txStore.On("Rollback").Return(nil)
	txStore.On("Disconnect").Return(nil)

	err := TransferBalanceInMongo(txMgr, ctx, "A", "B", 5.0)
	assert.ErrorIs(t, err, mongo.ErrNoMatch, "callers can match the cause")
	assert.NotContains(t, err.Error(), "<nil>", "a successful rollback adds nothing to the message")
	txStore.AssertNumberOfCalls(t, "Update", 1)
	txStore.AssertNotCalled(t, "Commit")
}

func TestTransferBalanceInMongo_BeginTransactionError(t *testing.T) {
	ctx := context.Background()
	txMgr := new(MockTxManager)
	txMgr.On("BeginTransaction", ctx).Return(new(MockTxStore), errors.New("no session"))

	err := TransferBalanceInMongo(txMgr, ctx, "A", "B", 1.0)
	assert.EqualError(t, err, "no session")
}

// A transfer of NaN, infinity or a negative amount is refused before any store call.
func TestTransfer_RejectsInvalidAmounts(t *testing.T) {
	transfers := map[string]func(abstract.TransactionManager, context.Context, string, string, float64) error{
		"mongo": TransferBalanceInMongo,
		"sql":   TransferBalanceInSql,
	}
	amounts := map[string]float64{
		"NaN": math.NaN(), "+Inf": math.Inf(1), "-Inf": math.Inf(-1),
		"negative": -5.0, "tiny negative": -0.01, "min float": -math.MaxFloat64,
	}
	for tname, transfer := range transfers {
		for aname, amount := range amounts {
			t.Run(tname+"/"+aname, func(t *testing.T) {
				txMgr := new(MockTxManager) // no expectations: any store call would panic

				err := transfer(txMgr, context.Background(), "A", "B", amount)

				assert.ErrorIs(t, err, ErrInvalidAmount)
				txMgr.AssertNotCalled(t, "BeginTransaction", mock.Anything)
			})
		}
	}
}

// Zero is allowed, and so is the largest finite amount (the check is not too strict).
func TestTransferBalanceInMongo_AcceptsZeroAndLargeAmounts(t *testing.T) {
	for _, amount := range []float64{0, 0.01, math.MaxFloat64} {
		ctx := context.Background()
		txStore := new(MockTxStore)
		txMgr := new(MockTxManager)
		txMgr.On("BeginTransaction", ctx).Return(txStore, nil)
		txStore.On("Update", "accounts", mock.Anything, mock.Anything).Return(nil).Twice()
		txStore.On("Commit").Return(nil)
		txStore.On("Disconnect").Return(nil)

		assert.NoError(t, TransferBalanceInMongo(txMgr, ctx, "A", "B", amount), "amount %v", amount)
		txStore.AssertExpectations(t)
	}
}

// The cause survives errors.Is, and a failing rollback is reported next to it.
func TestTransferBalanceInMongo_ErrorsKeepTheirCause(t *testing.T) {
	boom, rollbackBoom := errors.New("db error"), errors.New("rollback error")
	for name, tc := range map[string]struct {
		firstFails bool
		rollback   error
	}{
		"first update, rollback ok":     {true, nil},
		"second update, rollback ok":    {false, nil},
		"first update, rollback fails":  {true, rollbackBoom},
		"second update, rollback fails": {false, rollbackBoom},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			txStore := new(MockTxStore)
			txMgr := new(MockTxManager)
			txMgr.On("BeginTransaction", ctx).Return(txStore, nil)
			if tc.firstFails {
				txStore.On("Update", "accounts", mock.Anything, mock.Anything).Return(boom).Once()
			} else {
				txStore.On("Update", "accounts", mock.Anything, mock.Anything).Return(nil).Once()
				txStore.On("Update", "accounts", mock.Anything, mock.Anything).Return(boom).Once()
			}
			txStore.On("Rollback").Return(tc.rollback)
			txStore.On("Disconnect").Return(nil)

			err := TransferBalanceInMongo(txMgr, ctx, "A", "B", 5.0)

			assert.ErrorIs(t, err, boom)
			if tc.rollback != nil {
				assert.ErrorIs(t, err, rollbackBoom)
				assert.ErrorContains(t, err, "rollback failed")
			} else {
				assert.Equal(t, boom, err, "no rollback error: the cause is returned as is")
			}
			txStore.AssertNotCalled(t, "Commit")
		})
	}
}

// The SQL transfer reports and wraps its errors the same way.
func TestTransferBalanceInSql_ErrorsKeepTheirCause(t *testing.T) {
	boom, rollbackBoom := errors.New("sql error"), errors.New("rollback error")
	for name, rb := range map[string]error{"rollback ok": nil, "rollback fails": rollbackBoom} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			txStore := new(MockTxStore)
			txMgr := new(MockTxManager)
			txMgr.On("BeginTransaction", ctx).Return(txStore, nil)
			txStore.On("ExecWithSql",
				"update accounts set balance = balance - $1 where id = $2", []any{5.0, "A"}).Return(nil).Once()
			txStore.On("ExecWithSql",
				"update accounts set balance = balance + $1 where id = $2", []any{5.0, "B"}).Return(boom).Once()
			txStore.On("Rollback").Return(rb)
			txStore.On("Disconnect").Return(nil)

			err := TransferBalanceInSql(txMgr, ctx, "A", "B", 5.0)

			assert.ErrorIs(t, err, boom)
			assert.Equal(t, rb != nil, errors.Is(err, rollbackBoom))
			assert.NotContains(t, err.Error(), "<nil>")
			txStore.AssertNotCalled(t, "Commit")
		})
	}
}

func TestTransferBalanceInSql(t *testing.T) {
	ctx := context.Background()
	txStore := new(MockTxStore)
	txMgr := new(MockTxManager)
	txMgr.On("BeginTransaction", ctx).Return(txStore, nil)
	txStore.On("ExecWithSql",
		"update accounts set balance = balance - $1 where id = $2", []any{100.0, "A"}).Return(nil).Once()
	txStore.On("ExecWithSql",
		"update accounts set balance = balance + $1 where id = $2", []any{100.0, "B"}).Return(nil).Once()
	txStore.On("Commit").Return(nil)
	txStore.On("Disconnect").Return(nil)

	err := TransferBalanceInSql(txMgr, ctx, "A", "B", 100.0)
	assert.NoError(t, err)
	txMgr.AssertExpectations(t)
	txStore.AssertExpectations(t)
}

// resultTx is a TxStore whose updates report fixed rows-affected counts as a sql.Result.
// MockTxStore cannot do that: it returns a nil result for an update. The embedded TxStore is
// nil, so any method not defined here would panic.
type resultTx struct {
	abstract.TxStore
	rows       []int64 // rows affected by the 1st, 2nd, ... update
	execs      int
	committed  bool
	rolledBack bool
}

func (r *resultTx) ExecWithSql(string, ...any) (any, error) {
	n := r.rows[r.execs]
	r.execs++
	return driver.RowsAffected(n), nil
}
func (r *resultTx) Commit() error     { r.committed = true; return nil }
func (r *resultTx) Rollback() error   { r.rolledBack = true; return nil }
func (r *resultTx) Disconnect() error { return nil }

// An update that matches no row (0 rows affected) fails the transfer with ErrAccountNotFound
// and rolls it back. When the sender is missing, the receiver is never updated.
func TestTransferBalanceInSql_UnknownAccountRollsBack(t *testing.T) {
	cases := map[string]struct {
		rows      []int64
		wantExecs int
		wantID    string
	}{
		"sender matches no row":   {rows: []int64{0, 1}, wantExecs: 1, wantID: "A"},
		"receiver matches no row": {rows: []int64{1, 0}, wantExecs: 2, wantID: "B"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			tx := &resultTx{rows: tc.rows}
			txMgr := new(MockTxManager)
			txMgr.On("BeginTransaction", ctx).Return(tx, nil)

			err := TransferBalanceInSql(txMgr, ctx, "A", "B", 5.0)

			assert.ErrorIs(t, err, ErrAccountNotFound)
			assert.ErrorContains(t, err, tc.wantID)
			assert.Equal(t, tc.wantExecs, tx.execs)
			assert.True(t, tx.rolledBack)
			assert.False(t, tx.committed)
			txMgr.AssertExpectations(t)
		})
	}
}

// The query side of the transfer reads one balance and writes nothing. A MockStore is
// only used as a Reader here: any write call would panic, as it has no expectation.
func TestReadBalanceInMongo(t *testing.T) {
	boom := errors.New("read boom")
	cases := map[string]struct {
		rows    any
		readErr error
		want    float64
		wantErr string
	}{
		"float64":          {rows: []bson.M{{"balance": 12.5}}, want: 12.5},
		"int32":            {rows: []bson.M{{"balance": int32(7)}}, want: 7},
		"int64":            {rows: []bson.M{{"balance": int64(-3)}}, want: -3},
		"zero":             {rows: []bson.M{{"balance": 0.0}}, want: 0},
		"read error":       {readErr: boom, wantErr: "read boom"},
		"account missing":  {rows: []bson.M{}, wantErr: "found 0"},
		"nil rows":         {rows: []bson.M(nil), wantErr: "found 0"},
		"duplicate rows":   {rows: []bson.M{{"balance": 1.0}, {"balance": 2.0}}, wantErr: "found 2"},
		"wrong result":     {rows: "not rows", wantErr: "unexpected read result string"},
		"balance is text":  {rows: []bson.M{{"balance": "12"}}, wantErr: "want a number"},
		"balance is unset": {rows: []bson.M{{"id": "A"}}, wantErr: "want a number"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			store := new(MockStore)
			store.On("Read", "accounts", map[string]any{"id": "A"}).Return(c.rows, c.readErr).Once()

			got, err := ReadBalanceInMongo(store, "A")

			if c.wantErr != "" {
				assert.ErrorContains(t, err, c.wantErr)
				assert.Zero(t, got)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, c.want, got)
			}
			store.AssertExpectations(t)
		})
	}
}
