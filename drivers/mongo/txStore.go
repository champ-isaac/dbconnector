package mongo

import (
	"context"
	"log"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"tw.com.championtek.storage/abstract"
)

type TxStore struct {
	ctx    mongo.SessionContext
	sess   mongo.Session
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

func (t *TxStore) Update(collection string, filter any, data any) error {
	coll := t.driver.Client.Database(t.db).Collection(collection)
	update := bson.M{"$set": data}
	_, err := coll.UpdateMany(t.ctx, bson.M{"$set": filter}, update)
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
