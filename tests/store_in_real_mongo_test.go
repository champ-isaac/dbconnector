package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/champ-isaac/dbconnector/abstract"
	"github.com/champ-isaac/dbconnector/factory"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type AccountOfMongo struct {
	Id        bson.ObjectID   `bson:"_id,omitempty"`
	Firstname string          `bson:"first_name,omitempty"`
	Lastname  string          `bson:"last_name,omitempty"`
	Age       int             `bson:"age,omitempty"`
	Amount    bson.Decimal128 `bson:"amount,omitempty"`
	CreatedAt time.Time       `bson:"created_at,omitempty"`
	UpdatedAt time.Time       `bson:"updated_at,omitempty"`
}

type TransformedAccountOfMongo struct {
	Id            bson.ObjectID `bson:"_id,omitempty"`
	AccountId     bson.ObjectID `bson:"account_id,omitempty"`
	RelatedFields string        `bson:"related_fields,omitempty"`
}

func TestStore_NewStore_InMongo(t *testing.T) {
	store, err := initMongoStore()
	assert.NoError(t, err)
	assert.NotNil(t, store)

	err = store.Ping()
	assert.NoError(t, err)
}

func TestStore_CreateAccount_InMongo(t *testing.T) {
	store, err := initMongoStore()
	assert.NoError(t, err)
	assert.NotNil(t, store)

	accountId, err := createAccountInMongo(store)
	assert.NoError(t, err)
	assert.NotEqual(t, bson.NilObjectID, accountId)

	//remove created account
	filter := map[string]any{
		"_id": accountId,
	}
	transFilter := map[string]any{
		"account_id": accountId,
	}
	err = deleteAccountInMongo(store, collection, transformed, filter, transFilter)
	assert.NoError(t, err)
	t.Log("remove account id: ", accountId.String())
}

func TestStore_ReadAccount_InMongo(t *testing.T) {
	store, err := initMongoStore()
	assert.NoError(t, err)
	assert.NotNil(t, store)

	accountId, err := createAccountInMongo(store)
	assert.NoError(t, err)
	assert.NotEqual(t, bson.NilObjectID, accountId)

	filter := map[string]any{
		"_id": accountId,
	}

	rows, err := readAccountInMongo(store, collection, filter)
	assert.NoError(t, err)
	assert.NotEmpty(t, rows)

	for _, row := range rows {
		assert.Equal(t, first_name_1, row.Firstname)
		assert.Equal(t, last_name_1, row.Lastname)
		assert.Equal(t, accountId, row.Id)
		t.Log("read account id: ", row.Id.String())
	}

	transFilter := map[string]any{
		"account_id": accountId,
	}
	err = deleteAccountInMongo(store, collection, transformed, filter, transFilter)
	assert.NoError(t, err)
	t.Log("remove account id: ", accountId.String())
}

func TestStore_UpdateAccount_InMongo(t *testing.T) {
	store, err := initMongoStore()
	assert.NoError(t, err)
	assert.NotNil(t, store)

	accountId, err := createAccountInMongo(store)
	assert.NoError(t, err)

	filter := map[string]any{
		"_id": accountId,
	}
	rows, err := readAccountInMongo(store, collection, filter)
	assert.NoError(t, err)
	assert.NotEmpty(t, rows)

	amount, _ := bson.ParseDecimal128("4244.56")
	data := AccountOfMongo{
		Amount: amount,
	}
	err = store.Update(collection, filter, data)
	assert.NoError(t, err)

	rows, err = readAccountInMongo(store, collection, filter)
	assert.NoError(t, err)
	assert.NotEmpty(t, rows)
	assert.Equal(t, accountId, rows[0].Id)
	for _, row := range rows {
		t.Log("updated read account id: ", row.Id.String())
		t.Log("updated read amount: ", row.Amount.String())
	}

	transFilter := map[string]any{
		"account_id": accountId,
	}

	err = deleteAccountInMongo(store, collection, transformed, filter, transFilter)
	assert.NoError(t, err)
	t.Log("remove account id: ", accountId.String())
	rows, err = readAccountInMongo(store, collection, filter)
	assert.NoError(t, err)
	assert.Empty(t, rows)
}

func TestStore_DeleteAccount_InMongo(t *testing.T) {
	store, err := initMongoStore()
	assert.NoError(t, err)
	assert.NotNil(t, store)

	accountId, err := createAccountInMongo(store)
	assert.NoError(t, err)

	filter := map[string]any{
		"_id": accountId,
	}
	transFilter := map[string]any{
		"account_id": accountId,
	}

	err = deleteAccountInMongo(store, collection, transformed, filter, transFilter)
	assert.NoError(t, err)
	t.Log("remove account id: ", accountId.String())

	rows, err := readAccountInMongo(store, collection, filter)
	assert.NoError(t, err)
	assert.Empty(t, rows)
}

func initMongoStore() (abstract.Store, error) {
	config := factory.Config{
		Type:            factory.DbTypeMongo,
		Ctx:             context.Background(),
		Schema:          "mongodb",
		Db:              database,
		Username:        username,
		Password:        password,
		Host:            host,
		Port:            mongodbPort,
		SslMode:         false,
		ReplicaMaster:   "", // if you're testing on local, keep this empty
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 2 * time.Second,
		ConnMaxIdleTime: 2 * time.Second,
	}
	return factory.NewStore(config)
}

func createAccountInMongo(store abstract.Store) (bson.ObjectID, error) {
	accountId := bson.NewObjectID()
	amount, _ := bson.ParseDecimal128("1250.75")
	account := AccountOfMongo{
		Id:        accountId,
		Firstname: first_name_1,
		Lastname:  last_name_1,
		Age:       45,
		Amount:    amount,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.Create(collection, account)
	if err != nil {
		return bson.NilObjectID, err
	}
	err = store.Create(transformed, TransformedAccountOfMongo{
		AccountId:     accountId,
		RelatedFields: "masked_email",
	})
	if err != nil {
		//remove main record
		_ = store.Delete(collection, map[string]any{"_id": accountId})
		return bson.NilObjectID, err
	}

	err = store.Create(transformed, TransformedAccountOfMongo{
		AccountId:     accountId,
		RelatedFields: "masked_address",
	})
	if err != nil {
		//remove main record
		_ = store.Delete(collection, map[string]any{"_id": accountId})
		return bson.NilObjectID, err
	}
	return accountId, err
}

func readAccountInMongo(store abstract.Store, collection string, filter map[string]any) ([]AccountOfMongo, error) {
	var result []AccountOfMongo

	rows, err := store.Read(collection, filter)
	if err != nil {
		return nil, err
	}
	data, ok := rows.([]bson.M)
	if !ok {
		return nil, errors.New("failed to convert []bson.M type")
	}
	for _, m := range data {
		b, err := bson.Marshal(m)
		if err != nil {
			return nil, err
		}
		var acc AccountOfMongo
		err = bson.Unmarshal(b, &acc)
		if err != nil {
			return nil, err
		}
		result = append(result, acc)
	}
	return result, nil
}

func deleteAccountInMongo(store abstract.Store, collection, transformed string, filter, transFilter map[string]any) error {
	// delete records in transformed table
	err := store.Delete(transformed, transFilter)
	if err != nil {
		return err
	}
	//delete records in main table
	err = store.Delete(collection, filter)
	if err != nil {
		return err
	}
	return nil
}
