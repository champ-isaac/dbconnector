package tests

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/champ-isaac/dbconnector/abstract"
	"github.com/champ-isaac/dbconnector/factory"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	database     = "data_privacy"
	collection   = "accounts"
	username     = "user"
	password     = "user!@#"
	host         = "127.0.0.1"
	postgresPort = 5432
	mongodbPort  = 27017

	//test data
	first_name_1 = "John"
	first_name_2 = "Smith"

	last_name_1 = "Doe"
	last_name_2 = "Black"
)

type AccountOfSql struct {
	Id        uuid.UUID       `json:"id,omitempty"`
	Firstname string          `json:"first_name,omitempty"`
	Lastname  string          `json:"last_name,omitempty"`
	Age       int             `json:"age,omitempty"`
	Amount    decimal.Decimal `json:"amount,omitempty"`
	CreatedAt time.Time       `json:"created_at,omitempty"`
	UpdatedAt time.Time       `json:"updated_at,omitempty"`
}

type TransformedAccountOfSql struct {
	Id            uuid.UUID `json:"id,omitempty"`
	AccountId     uuid.UUID `json:"account_id,omitempty"`
	RelatedFields string    `json:"related_fields,omitempty"`
}

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

func TestStore_NewStore_InPostgres(t *testing.T) {
	store, err := initPostgresStore()
	assert.NoError(t, err)
	assert.NotNil(t, store)

	err = store.Ping()
	assert.NoError(t, err)
}

func TestStore_CreateAccount_InPostgres(t *testing.T) {
	store, err := initPostgresStore()
	assert.NoError(t, err)
	assert.NotNil(t, store)

	err = createAccount(store)
	assert.NoError(t, err)

	err = truncateAccountsTable(store)
	assert.NoError(t, err)
}

func TestStore_ReadAccount_InPostgres(t *testing.T) {
	store, err := initPostgresStore()
	assert.NoError(t, err)
	assert.NotNil(t, store)

	err = createAccount(store)
	assert.NoError(t, err)

	filter := map[string]any{
		"first_name": first_name_1,
		"last_name":  last_name_1,
	}
	account, err := readAccount(store, collection, filter)
	assert.NoError(t, err)
	assert.NotNil(t, account.Id)
	t.Log("created account id: ", account.Id.String())

	err = truncateAccountsTable(store)
	assert.NoError(t, err)
	t.Log("truncate accounts table")
}

func TestStore_UpdateAccount_InPostgres(t *testing.T) {
	store, err := initPostgresStore()
	assert.NoError(t, err)
	assert.NotNil(t, store)

	err = createAccount(store)
	assert.NoError(t, err)

	filter := map[string]any{
		"first_name": first_name_1,
		"last_name":  last_name_1,
	}
	account, err := readAccount(store, collection, filter)
	assert.NoError(t, err)
	assert.NotEmpty(t, account)
	t.Log(account)

	filter = map[string]any{
		"id": account.Id.String(),
	}
	t.Logf("created account id: %s, amount: %s ", account.Id.String(), account.Amount.String())

	amount, _ := decimal.NewFromString("12500.75")
	data := map[string]any{
		"amount":     amount,
		"updated_at": time.Now(),
	}
	err = store.Update(collection, filter, data)
	assert.NoError(t, err)

	account, err = readAccount(store, collection, filter)
	assert.NoError(t, err)
	t.Logf("updated account id: %s, amount: %s ", account.Id.String(), account.Amount.String())

	err = truncateAccountsTable(store)
	assert.NoError(t, err)
	t.Log("truncate accounts table")
}

func TestStore_DeleteAccount_InPostgres(t *testing.T) {
	store, err := initPostgresStore()
	assert.NoError(t, err)
	assert.NotNil(t, store)

	err = createAccount(store)
	assert.NoError(t, err)

	filter := map[string]any{
		"first_name": first_name_1,
		"last_name":  last_name_1,
	}
	account, err := readAccount(store, collection, filter)
	assert.NoError(t, err)
	assert.NotEmpty(t, account)

	filter = map[string]any{
		"id": account.Id.String(),
	}
	t.Log("created account id: ", account.Id.String())

	err = store.Delete(collection, filter)
	assert.NoError(t, err)

	account, err = readAccount(store, collection, filter)
	assert.NoError(t, err)
	assert.Empty(t, account)
	t.Log("delete account id is empty: ", account.Id.String())
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

	accountId := bson.NewObjectID()
	amount, _ := bson.ParseDecimal128("2500.75")
	account := AccountOfMongo{
		Id:        accountId,
		Firstname: "apple",
		Lastname:  "seed",
		Age:       40,
		Amount:    amount,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.Create("accounts", account)
	assert.NoError(t, err)
	err = store.Create("transformed_accounts", TransformedAccountOfMongo{
		AccountId:     accountId,
		RelatedFields: "masked_email",
	})
	assert.NoError(t, err)

	err = store.Create("transformed_accounts", TransformedAccountOfMongo{
		AccountId:     accountId,
		RelatedFields: "masked_address",
	})
	assert.NoError(t, err)
}

func initPostgresStore() (abstract.Store, error) {
	config := factory.Config{
		Type:            factory.DbTypeSql,
		Ctx:             context.Background(),
		Schema:          "postgres",
		Db:              database,
		Username:        username,
		Password:        password,
		Host:            host,
		Port:            postgresPort,
		SslMode:         false,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxIdleTime: 2 * time.Second,
		ConnMaxLifetime: 2 * time.Second,
	}
	return factory.NewStore(config)
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

func structToMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	err = json.Unmarshal(b, &m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func createAccount(store abstract.Store) error {
	accountIdOfSql := uuid.New()
	amount, _ := decimal.NewFromString("2500.75")
	account := AccountOfSql{
		Id:        accountIdOfSql,
		Firstname: first_name_1,
		Lastname:  last_name_1,
		Age:       40,
		Amount:    amount,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m, err := structToMap(account)
	if err != nil {
		return err
	}

	err = store.Create("accounts", m)
	if err != nil {
		return err
	}

	tm1, err := structToMap(TransformedAccountOfSql{
		Id:            uuid.New(),
		AccountId:     accountIdOfSql,
		RelatedFields: "masked_email",
	})
	if err != nil {
		return err
	}

	err = store.Create("transformed_accounts", tm1)
	if err != nil {
		return err
	}

	tm2, err := structToMap(TransformedAccountOfSql{
		Id:            uuid.New(),
		AccountId:     accountIdOfSql,
		RelatedFields: "masked_address",
	})
	if err != nil {
		return err
	}
	err = store.Create("transformed_accounts", tm2)
	if err != nil {
		return err
	}
	return nil
}

func readAccount(store abstract.Store, collection string, filter map[string]any) (AccountOfSql, error) {
	rows, err := store.Read(collection, filter)
	if err != nil {
		return AccountOfSql{}, err
	}
	//convert to decimal first
	for _, row := range rows.([]any) {
		data, ok := row.(map[string]any)
		if !ok {
			err = errors.New("data is not map[string]any type")
			break
		}
		u := string(data["id"].([]uint8))
		data["id"] = u
		s := string(data["amount"].([]uint8))
		amount, err := decimal.NewFromString(s)
		if err != nil {
			break
		}
		data["amount"] = amount
	}
	if err != nil {
		return AccountOfSql{}, err
	}
	var accounts []AccountOfSql
	bs, err := json.Marshal(rows)
	if err != nil {
		return AccountOfSql{}, err
	}
	err = json.Unmarshal(bs, &accounts)
	if err != nil {
		return AccountOfSql{}, err
	}
	for _, account := range accounts {
		return account, nil
	}
	return AccountOfSql{}, err
}

func truncateAccountsTable(store abstract.Store) error {
	_, err := store.ExecWithSql("TRUNCATE TABLE accounts CASCADE;")
	return err
}
