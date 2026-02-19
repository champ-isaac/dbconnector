package mongo

import (
	"log"

	"go.mongodb.org/mongo-driver/bson"
	"tw.com.championtek.storage/abstract"
)

type Store struct {
	driver   *Driver
	database string
}

func NewStore(driver *Driver, database string) abstract.Store {
	return &Store{driver: driver, database: database}
}

func (s *Store) ExecWithSql(sql string, args ...any) (any, error) {
	//TODO implement me
	panic("ExecWithSql does not implement for mongo yet")
}

func (s *Store) Create(collection string, data any) error {
	coll := s.driver.Client.Database(s.database).Collection(collection)
	_, err := coll.InsertOne(s.driver.Ctx, data)
	return err
}

func (s *Store) Read(collection string, filter any) (any, error) {
	coll := s.driver.Client.Database(s.database).Collection(collection)
	cursor, err := coll.Find(s.driver.Ctx, filter)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err = cursor.Close(s.driver.Ctx); err != nil {
			log.Printf("error when cursor closing: %v", err)
		}
	}()
	var results []bson.M
	for cursor.Next(s.driver.Ctx) {
		var doc bson.M
		if err = cursor.Decode(&doc); err != nil {
			return nil, err
		}
		results = append(results, doc)
	}
	return results, nil
}

func (s *Store) Update(collection string, filter any, data any) error {
	coll := s.driver.Client.Database(s.database).Collection(collection)
	update := bson.D{{"$set", data}}
	_, err := coll.UpdateMany(s.driver.Ctx, filter, update)
	return err
}

func (s *Store) Delete(collection string, filter any) error {
	coll := s.driver.Client.Database(s.database).Collection(collection)
	_, err := coll.DeleteMany(s.driver.Ctx, filter)
	return err
}

func (s *Store) Ping() error {
	return s.driver.Client.Ping(s.driver.Ctx, nil)
}

func (s *Store) Disconnect() error {
	return s.driver.Disconnect()
}
