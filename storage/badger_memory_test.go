package storage_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/thisisdevelopment/mightymap"
	"github.com/thisisdevelopment/mightymap/storage"
)

func TestBadgerMemoryStorageWithString(t *testing.T) {
	ctx := context.Background()

	// Create a complete mightymap with in-memory BadgerDB
	// This is the most likely way it would be used in the application
	store := storage.NewMightyMapBadgerStorage[string, string](
		storage.WithMemoryStorage(true),
	)
	mm := mightymap.New[string, string](true, store)
	defer mm.Close(ctx)

	// Define test key and value
	testKey := "test-key-123"
	testValue := "this is a test string value"

	// Store the string value
	mm.Store(ctx, testKey, testValue)

	// Check if key exists
	hasKey := mm.Has(ctx, testKey)
	t.Logf("Has key: %v", hasKey)

	// Retrieve the value
	value, ok := mm.Load(ctx, testKey)
	if !ok {
		t.Fatalf("Failed to load value for key: %s", testKey)
	}

	t.Logf("Retrieved value: %q", value)

	if value != testValue {
		t.Errorf("Loaded value does not match stored value.\nExpected: %q\nGot: %q", testValue, value)
	}
}

// A solution for applications that need to store string values via 'any'
func Example_workingWithStringAndAnyValues() {
	// Create a type-safe wrapper for storing string values
	type StringValueMap struct {
		storage mightymap.Map[string, string]
	}

	// Constructor function for the wrapper
	newStringValueMap := func() *StringValueMap {
		store := storage.NewMightyMapBadgerStorage[string, string](
			storage.WithMemoryStorage(true),
		)
		mm := mightymap.New[string, string](true, store)
		return &StringValueMap{storage: *mm}
	}

	// Create a new instance
	ctx := context.Background()
	valueMap := newStringValueMap()
	defer valueMap.storage.Close(ctx)

	// Store string values
	valueMap.storage.Store(ctx, "key1", "value1")

	// Retrieve without type assertion
	value, ok := valueMap.storage.Load(ctx, "key1")
	if ok {
		fmt.Printf("String value: %s\n", value)
	}

	// Output:
	// String value: value1
}

// Custom wrapper for a map that stores multiple value types
func Example_multiTypeStorage() {
	// Create a struct that contains multiple typed maps
	type MultiTypeStore struct {
		strings *mightymap.Map[string, string]
		ints    *mightymap.Map[string, int]
		bools   *mightymap.Map[string, bool]
	}

	// Constructor
	newMultiTypeStore := func() *MultiTypeStore {
		strStore := storage.NewMightyMapBadgerStorage[string, string](
			storage.WithMemoryStorage(true),
		)
		intStore := storage.NewMightyMapBadgerStorage[string, int](
			storage.WithMemoryStorage(true),
		)
		boolStore := storage.NewMightyMapBadgerStorage[string, bool](
			storage.WithMemoryStorage(true),
		)

		return &MultiTypeStore{
			strings: mightymap.New[string, string](true, strStore),
			ints:    mightymap.New[string, int](true, intStore),
			bools:   mightymap.New[string, bool](true, boolStore),
		}
	}

	// Create a new store
	ctx := context.Background()
	store := newMultiTypeStore()
	defer func() {
		store.strings.Close(ctx)
		store.ints.Close(ctx)
		store.bools.Close(ctx)
	}()

	// Store different types of values
	store.strings.Store(ctx, "name", "John")
	store.ints.Store(ctx, "age", 30)
	store.bools.Store(ctx, "active", true)

	// Retrieve values with correct types
	name, _ := store.strings.Load(ctx, "name")
	age, _ := store.ints.Load(ctx, "age")
	active, _ := store.bools.Load(ctx, "active")

	fmt.Printf("Name: %s, Age: %d, Active: %v\n", name, age, active)

	// Output:
	// Name: John, Age: 30, Active: true
}
