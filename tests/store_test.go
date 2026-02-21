package tests

import (
	"fmt"
	"testing"

	"github.com/champ-isaac/dbconnector/abstract"
	"github.com/stretchr/testify/assert"
)

// ------ Logics ------
func CreateUser(store abstract.Store, userData map[string]any) error {
	return store.Create("users", userData)
}
func ReadUser(store abstract.Store, filter map[string]any) (any, error) {
	return store.Read("users", filter)
}
func DeleteUser(store abstract.Store, filter map[string]any) error {
	return store.Delete("users", filter)
}

func UpdateUser(store abstract.Store, filter, userData map[string]any) error {
	return store.Update("users", filter, userData)
}

func Ping(store abstract.Store) error {
	return store.Ping()
}

// ------ Tests ------
func TestCreateUser(t *testing.T) {
	mockStore := new(MockStore)
	userData := map[string]any{
		"name":  "John Doe",
		"age":   30,
		"email": "john.doe@example.com",
	}
	mockStore.On("Create", "users", userData).Return(nil)
	err := mockStore.Create("users", userData)
	assert.NoError(t, err)
	mockStore.AssertExpectations(t)
}

func TestCreateUser_Failure(t *testing.T) {
	mockStore := new(MockStore)
	userData := map[string]any{
		"name":  "John Doe",
		"age":   30,
		"email": "john.doe@example.com",
	}
	errMsg := fmt.Errorf("create user %s error", userData["name"])
	mockStore.On("Create", "users", userData).Return(errMsg)
	err := mockStore.Create("users", userData)
	assert.Equal(t, err, errMsg)
	mockStore.AssertExpectations(t)
}

func TestReadUser(t *testing.T) {
	mockStore := new(MockStore)
	filter := map[string]any{
		"name": "John Doe",
	}
	expected := map[string]any{
		"name":  "John Doe",
		"age":   30,
		"email": "john.doe@example.com",
	}
	mockStore.On("Read", "users", filter).Return(expected, nil)
	result, err := mockStore.Read("users", filter)
	assert.NoError(t, err)
	assert.Equal(t, expected, result)
	mockStore.AssertExpectations(t)
}

func TestReadUser_Failure(t *testing.T) {
	mockStore := new(MockStore)
	filter := map[string]any{
		"name": "John Doe",
	}
	errMsg := fmt.Errorf("read user %s error", "John Doe")
	mockStore.On("Read", "users", filter).Return([]any{}, errMsg)
	result, err := mockStore.Read("users", filter)
	assert.Equal(t, err, errMsg)
	assert.Empty(t, result)
	mockStore.AssertExpectations(t)
}

func TestDeleteUser(t *testing.T) {
	mockStore := new(MockStore)
	filter := map[string]any{
		"name": "John Doe",
	}
	mockStore.On("Delete", "users", filter).Return(nil)
	err := mockStore.Delete("users", filter)
	assert.NoError(t, err)
	mockStore.AssertExpectations(t)
}

func TestDeleteUser_Failure(t *testing.T) {
	mockStore := new(MockStore)
	filter := map[string]any{
		"name": "John Doe",
	}
	errMsg := fmt.Errorf("delete user %s error", "John Doe")
	mockStore.On("Delete", "users", filter).Return(errMsg)
	err := mockStore.Delete("users", filter)
	assert.Equal(t, err, errMsg)
	mockStore.AssertExpectations(t)
}

func TestUpdateUser(t *testing.T) {
	mockStore := new(MockStore)
	filter := map[string]any{
		"name": "John Doe",
	}
	update := map[string]any{
		"email": "new.john.doe@example.com",
	}
	mockStore.On("Update", "users", filter, update).Return(nil)
	err := mockStore.Update("users", filter, update)
	assert.NoError(t, err)
	mockStore.AssertExpectations(t)
}

func TestUpdateUser_Failure(t *testing.T) {
	mockStore := new(MockStore)
	filter := map[string]any{
		"name": "John Doe",
	}
	update := map[string]any{
		"email": "wrong.john.doe@example.com",
	}
	errMsg := fmt.Errorf("update user %s error", "John Doe")
	mockStore.On("Update", "users", filter, update).Return(errMsg)
	err := mockStore.Update("users", filter, update)
	assert.Equal(t, err, errMsg)
	mockStore.AssertExpectations(t)
}

func TestPing(t *testing.T) {
	mockStore := new(MockStore)
	mockStore.On("Ping").Return(nil)
	err := mockStore.Ping()
	assert.NoError(t, err)
	mockStore.AssertExpectations(t)
}
