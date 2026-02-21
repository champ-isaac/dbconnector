package tests

import (
	"context"
	"fmt"
	"strings"

	"github.com/champ-isaac/dbconnector/abstract"
	"github.com/stretchr/testify/mock"
)

type MockTxStore struct {
	mock.Mock
}

func (m *MockTxStore) ExecWithSql(sql string, values ...any) (any, error) {
	args := m.Called(sql, values)
	if strings.HasPrefix(strings.ToLower(sql), "insert") ||
		strings.HasPrefix(strings.ToLower(sql), "update") ||
		strings.HasPrefix(strings.ToLower(sql), "delete") {
		return nil, args.Error(0)
	}
	if strings.HasPrefix(strings.ToLower(sql), "select") {
		return args.Get(0).(any), args.Error(1)
	}
	return nil, fmt.Errorf("unexpected query: %s", sql)
}

func (m *MockTxStore) Create(collection string, data any) error {
	args := m.Called(collection, data)
	return args.Error(0)
}

func (m *MockTxStore) Read(collection string, filter any) (any, error) {
	args := m.Called(collection, filter)
	return args.Get(0).(any), args.Error(1)
}

func (m *MockTxStore) Update(collection string, filter, data any) error {
	args := m.Called(collection, filter, data)
	return args.Error(0)
}

func (m *MockTxStore) Delete(collection string, filter any) error {
	args := m.Called(collection, filter)
	return args.Error(0)
}

func (m *MockTxStore) Commit() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockTxStore) Rollback() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockTxStore) Disconnect() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockTxStore) Ping() error {
	args := m.Called()
	return args.Error(0)
}

//>>>>>>>>>>>>>>>>>>>>>>>>

type MockTxManager struct {
	mock.Mock
}

func (m *MockTxManager) BeginTransaction(ctx context.Context) (abstract.TxStore, error) {
	args := m.Called(ctx)
	return args.Get(0).(abstract.TxStore), args.Error(1)
}
