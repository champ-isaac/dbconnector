package tests

import (
	"context"
	"fmt"

	"github.com/champ-isaac/dbconnector/abstract"
)

// TransferBalanceInSql simulate transfer balance process in sql database
func TransferBalanceInSql(mgr abstract.TransactionManager, ctx context.Context, from, to string, amount float64) error {
	tx, err := mgr.BeginTransaction(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err = tx.Disconnect(); err != nil {
			panic(err)
		}
	}()
	_, err = tx.ExecWithSql("update accounts set balance = balance - $1 where id = $2", amount, from)
	if err != nil {
		e := tx.Rollback()
		return fmt.Errorf("%v:%v", err, e)

	}
	_, err = tx.ExecWithSql("update accounts set balance = balance + $1 where id = $2", amount, to)
	if err != nil {
		e := tx.Rollback()
		return fmt.Errorf("%v:%v", err, e)

	}
	return tx.Commit()
}

// TransferBalanceInMongo simulate transfer balance process in mongo database
func TransferBalanceInMongo(mgr abstract.TransactionManager, ctx context.Context, from, to string, amount float64) error {
	tx, err := mgr.BeginTransaction(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err = tx.Disconnect(); err != nil {
			panic(err)
		}
	}()
	err = tx.Update("accounts", map[string]any{"id": from}, map[string]any{"balance": fmt.Sprintf("balance - %f", amount)})
	if err != nil {
		e := tx.Rollback()
		return fmt.Errorf("%v:%v", err, e)
	}
	err = tx.Update("accounts", map[string]any{"id": to}, map[string]any{"balance": fmt.Sprintf("balance + %f", amount)})
	if err != nil {
		e := tx.Rollback()
		return fmt.Errorf("%v:%v", err, e)
	}
	return tx.Commit()
}
