package mongo

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"math"
	"slices"

	"github.com/champ-isaac/dbconnector/abstract"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type TxStore struct {
	ctx    context.Context
	sess   *mongo.Session
	driver *Driver
	db     string
}

func (s *Store) BeginTransaction(ctx context.Context) (abstract.TxStore, error) {
	sess, err := s.driver.Client.StartSession()
	if err != nil {
		return nil, err
	}
	sc := mongo.NewSessionContext(ctx, sess)
	if err = sess.StartTransaction(); err != nil {
		sess.EndSession(ctx)
		return nil, err
	}
	return &TxStore{
		ctx:    sc,
		sess:   sess,
		driver: s.driver,
		db:     s.database,
	}, nil
}

func (t *TxStore) Create(collection string, data any) error {
	coll := t.driver.Client.Database(t.db).Collection(collection)
	//work in transaction context
	_, err := coll.InsertOne(t.ctx, data)
	return err
}

func (t *TxStore) Read(collection string, filter any) (any, error) {
	coll := t.driver.Client.Database(t.db).Collection(collection)
	cursor, err := coll.Find(t.ctx, filter)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err = cursor.Close(t.ctx); err != nil {
			log.Printf("error when cursor closing: %v", err)
		}
	}()
	var results []bson.M
	for cursor.Next(t.ctx) {
		var doc bson.M
		if err = cursor.Decode(&doc); err != nil {
			return nil, err
		}
		results = append(results, doc)
	}
	return results, nil
}

// Inc is a relative update for TxStore.Update: each field is changed by its value
// (negative to subtract) with MongoDB's $inc, instead of being set to it. The fields
// must be numeric; MongoDB rejects the update otherwise. It needs at least one field
// (ErrEmptyInc), and a float value must be finite (ErrNonFiniteInc). The filter must match
// exactly one document (ErrNoMatch, ErrMultipleMatch). Only a TxStore accepts an Inc.
type Inc map[string]any

// ErrEmptyInc is returned by an Inc update that has no fields. MongoDB accepts an empty
// $inc and does nothing, so without this check the caller would think a change was made.
var ErrEmptyInc = errors.New("mongo: an Inc update has no fields")

// ErrNonFiniteInc is returned by an Inc update with a NaN or infinite value. MongoDB accepts
// them and the field would become NaN or infinite for good. The error names the field.
var ErrNonFiniteInc = errors.New("mongo: an Inc value is NaN or infinite")

// ErrNoMatch is returned by an Inc update whose filter matched no document. A delta
// applied to nothing is a lost change (a transfer would debit one account and credit none).
var ErrNoMatch = errors.New("mongo: relative update matched no document")

// ErrMultipleMatch is returned by an Inc update whose filter matched more than one
// document (for example a duplicate id). The delta was applied to every match inside
// the transaction, so the caller must roll back.
var ErrMultipleMatch = errors.New("mongo: relative update matched more than one document")

// ErrIncNeedsTransaction is returned by Store.Update when it is given an Inc. Without a
// transaction a wrong match count cannot be undone, and passing the Inc on as a $set
// would store the delta as the new value.
var ErrIncNeedsTransaction = errors.New("mongo: an Inc update needs a transaction, use BeginTransaction")

// checkFinite reports the first field (in name order, so the message is stable) whose
// float or Decimal128 value is NaN or infinite. Integers cannot be.
func checkFinite(inc Inc) error {
	for _, k := range slices.Sorted(maps.Keys(inc)) {
		bad := false
		switch v := inc[k].(type) {
		case float64:
			bad = math.IsNaN(v) || math.IsInf(v, 0)
		case float32:
			bad = math.IsNaN(float64(v)) || math.IsInf(float64(v), 0)
		case bson.Decimal128:
			bad = v.IsNaN() || v.IsInf() != 0
		}
		if bad {
			return fmt.Errorf("%w: field %q", ErrNonFiniteInc, k)
		}
	}
	return nil
}

// Update sets the fields in data. When data is an Inc it changes them by a delta instead.
func (t *TxStore) Update(collection string, filter any, data any) error {
	coll := t.driver.Client.Database(t.db).Collection(collection)
	if inc, ok := data.(Inc); ok {
		if len(inc) == 0 {
			return ErrEmptyInc
		}
		if err := checkFinite(inc); err != nil {
			return err
		}
		res, err := coll.UpdateMany(t.ctx, filter, bson.M{"$inc": map[string]any(inc)})
		if err != nil {
			return err
		}
		switch {
		case res.MatchedCount == 0:
			return ErrNoMatch
		case res.MatchedCount > 1:
			return ErrMultipleMatch
		}
		return nil
	}
	update := bson.M{"$set": data}
	_, err := coll.UpdateMany(t.ctx, filter, update)
	return err
}

func (t *TxStore) Delete(collection string, filter any) error {
	coll := t.driver.Client.Database(t.db).Collection(collection)
	_, err := coll.DeleteMany(t.ctx, filter)
	return err
}

func (t *TxStore) ExecWithSql(sql string, args ...any) (any, error) {
	//TODO implement me
	panic("ExecWithSql does not implement for mongo yet")
}

func (t *TxStore) Disconnect() error {
	_ = t.sess.AbortTransaction(t.ctx)
	t.sess.EndSession(t.ctx)
	return nil
}

func (t *TxStore) Ping() error {
	return t.driver.Client.Ping(t.ctx, nil)
}

func (t *TxStore) Commit() error {
	return t.sess.CommitTransaction(t.ctx)
}

func (t *TxStore) Rollback() error {
	return t.sess.AbortTransaction(t.ctx)
}

var _ abstract.TransactionManager = (*Store)(nil)
var _ abstract.TxStore = (*TxStore)(nil)
