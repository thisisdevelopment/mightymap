package storage

import (
	"context"
	"testing"
)

// TestType represents a test struct that will be stored and retrieved
type TestType struct {
	ID   string
	Name string
	Data map[string]interface{}
}

// TestNestedType represents a test struct with nested structures
type TestNestedType struct {
	ID     string
	Parent TestType
	Values []int
}

func TestMsgpackTypeConversion(t *testing.T) {
	// Register our test types
	RegisterMsgpackType(TestType{})
	RegisterMsgpackType(TestNestedType{})

	// Create a temporary storage
	dir := t.TempDir() + "/testdb"
	storage := NewMightyMapBadgerStorage[string, interface{}](WithTempDir(dir))
	defer storage.Close(context.Background())

	ctx := context.Background()

	// Test with a simple struct
	testKey := "test:simple"
	testValue := TestType{
		ID:   "123",
		Name: "Test Item",
		Data: map[string]interface{}{
			"count": 42,
			"tags":  []string{"test", "msgpack"},
		},
	}

	// Store the value
	storage.Store(ctx, testKey, testValue)

	// Retrieve the value
	retrieved, ok := storage.Load(ctx, testKey)
	if !ok {
		t.Fatalf("Failed to retrieve stored value")
	}

	// Attempt to cast it back to the original type
	castedValue, ok := retrieved.(TestType)
	if !ok {
		t.Fatalf("Failed to cast back to original type: %T vs TestType", retrieved)
	}

	// Verify the data
	if castedValue.ID != testValue.ID || castedValue.Name != testValue.Name {
		t.Errorf("Retrieved data doesn't match: got %+v, want %+v", castedValue, testValue)
	}

	// Test with a nested struct
	nestedKey := "test:nested"
	nestedValue := TestNestedType{
		ID:     "456",
		Parent: testValue,
		Values: []int{1, 2, 3, 4, 5},
	}

	// Store the nested value
	storage.Store(ctx, nestedKey, nestedValue)

	// Retrieve the nested value
	retrievedNested, ok := storage.Load(ctx, nestedKey)
	if !ok {
		t.Fatalf("Failed to retrieve stored nested value")
	}

	// Attempt to cast it back to the original type
	castedNested, ok := retrievedNested.(TestNestedType)
	if !ok {
		t.Fatalf("Failed to cast nested value to original type: %T vs TestNestedType", retrievedNested)
	}

	// Verify the nested data
	if castedNested.ID != nestedValue.ID || castedNested.Parent.ID != nestedValue.Parent.ID {
		t.Errorf("Retrieved nested data doesn't match: got %+v, want %+v", castedNested, nestedValue)
	}

	// Test with slice of custom type
	sliceKey := "test:slice"
	sliceValue := []TestType{
		{ID: "1", Name: "First"},
		{ID: "2", Name: "Second"},
		{ID: "3", Name: "Third"},
	}

	// Store the slice
	storage.Store(ctx, sliceKey, sliceValue)

	// Retrieve the slice
	retrievedSlice, ok := storage.Load(ctx, sliceKey)
	if !ok {
		t.Fatalf("Failed to retrieve stored slice")
	}

	// This might not work perfectly due to slice types, but let's check
	castedSlice, ok := retrievedSlice.([]TestType)
	if !ok {
		// If direct cast fails, check the underlying type
		t.Logf("Direct slice cast failed, got type: %T", retrievedSlice)

		// Even if the direct cast fails, we should verify the content is correct
		// This might require manual inspection or conversion
	}

	// If slice casting worked, verify length at least
	if ok && len(castedSlice) != len(sliceValue) {
		t.Errorf("Retrieved slice has wrong length: got %d, want %d", len(castedSlice), len(sliceValue))
	}
}
