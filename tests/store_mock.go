package tests

import (
	"github.com/stretchr/testify/mock"
)

type MockStore struct {
	mock.Mock
}

func (s *MockStore) Create(collection string, data any) error {
	args := s.Called(collection, data)
	return args.Error(0)
}

func (s *MockStore) Update(collection string, filter, data any) error {
	args := s.Called(collection, filter, data)
	return args.Error(0)
}

func (s *MockStore) Delete(collection string, filter any) error {
	args := s.Called(collection, filter)
	return args.Error(0)
}

func (s *MockStore) Read(collection string, filter any) (any, error) {
	args := s.Called(collection, filter)
	return args.Get(0).(any), args.Error(1)
}

func (s *MockStore) Ping() error {
	args := s.Called()
	return args.Error(0)
}
