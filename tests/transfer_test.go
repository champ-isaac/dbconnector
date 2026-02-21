package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestTransferBalanceInMongo(t *testing.T) {
	ctx := context.Background()
	txStore := new(MockTxStore)
	txMgr := new(MockTxManager)

	txMgr.On("BeginTransaction", ctx).Return(txStore, nil)
	txStore.On("Update", "accounts", map[string]any{"id": "A"}, mock.Anything).Return(nil).Once()
	txStore.On("Update", "accounts", map[string]any{"id": "B"}, mock.Anything).Return(nil).Once()
	txStore.On("Commit").Return(nil)
	txStore.On("Disconnect").Return(nil)

	err := TransferBalanceInMongo(txMgr, ctx, "A", "B", 100.0)
	assert.NoError(t, err)

	txMgr.AssertExpectations(t)
	txStore.AssertExpectations(t)
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
