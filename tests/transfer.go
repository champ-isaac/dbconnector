package tests

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/champ-isaac/dbconnector/abstract"
	"github.com/champ-isaac/dbconnector/drivers/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ErrInvalidAmount is returned, before any store call, for a transfer amount that is
// NaN, infinite or negative. A negative amount would move the money the other way.
var ErrInvalidAmount = errors.New("invalid transfer amount")

func checkAmount(amount float64) error {
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 {
		return fmt.Errorf("%w: %v", ErrInvalidAmount, amount)
	}
	return nil
}

// rollback undoes the transaction after err. The result still matches err with
// errors.Is, and also the rollback error when the rollback fails too.
func rollback(tx abstract.TxStore, err error) error {
	if e := tx.Rollback(); e != nil {
		return fmt.Errorf("%w (rollback failed: %w)", err, e)
	}
	return err
}

// TransferBalanceInSql simulate transfer balance process in sql database. It is the command
// side: it needs only a TransactionManager, such as a factory.CommandStore.
func TransferBalanceInSql(mgr abstract.TransactionManager, ctx context.Context, from, to string, amount float64) error {
	if err := checkAmount(amount); err != nil {
		return err
	}
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
		return rollback(tx, err)
	}
	_, err = tx.ExecWithSql("update accounts set balance = balance + $1 where id = $2", amount, to)
	if err != nil {
		return rollback(tx, err)
	}
	return tx.Commit()
}

// TransferBalanceInMongo simulate transfer balance process in mongo database. It is the
// command side: it needs only a TransactionManager, such as a factory.CommandStore.
func TransferBalanceInMongo(mgr abstract.TransactionManager, ctx context.Context, from, to string, amount float64) error {
	if err := checkAmount(amount); err != nil {
		return err
	}
	tx, err := mgr.BeginTransaction(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err = tx.Disconnect(); err != nil {
			panic(err)
		}
	}()
	err = tx.Update("accounts", map[string]any{"id": from}, mongo.Inc{"balance": -amount})
	if err != nil {
		return rollback(tx, err)
	}
	err = tx.Update("accounts", map[string]any{"id": to}, mongo.Inc{"balance": amount})
	if err != nil {
		return rollback(tx, err)
	}
	return tx.Commit()
}

// ReadBalanceInMongo is the query side of the transfer: it reads one account's balance and
// changes nothing, so a factory.QueryStore is enough. Behind a replica it can lag a commit.
func ReadBalanceInMongo(r abstract.Reader, id string) (float64, error) {
	rows, err := r.Read("accounts", map[string]any{"id": id})
	if err != nil {
		return 0, err
	}
	docs, ok := rows.([]bson.M)
	if !ok {
		return 0, fmt.Errorf("account %q: unexpected read result %T", id, rows)
	}
	if len(docs) != 1 {
		return 0, fmt.Errorf("account %q: want exactly 1 row, found %d", id, len(docs))
	}
	switch v := docs[0]["balance"].(type) {
	case float64:
		return v, nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("account %q: balance is %T, want a number", id, v)
	}
}
