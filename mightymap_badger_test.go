package mightymap_test

import (
	"context"
	"testing"
	"time"

	"github.com/thisisdevelopment/mightymap"
	"github.com/thisisdevelopment/mightymap/storage"
)

func TestMightyMap_BadgerStorage(t *testing.T) {
	// Testing MightyMap with Badger storage implementation
	ctx := context.Background()

	store := storage.NewMightyMapBadgerStorage[int, string](
		storage.WithMemoryStorage(true),
		storage.WithDetectConflicts(false),
	)
	cm := mightymap.New[int, string](true, store)

	// Repeat the same test cases as in TestMightyMap_DefaultStorage

	t.Run("Store and Load", func(t *testing.T) {
		cm.Store(ctx, 1, "one")
		value, ok := cm.Load(ctx, 1)
		if !ok || value != "one" {
			t.Errorf("Expected to load 'one', got '%v'", value)
		}
	})

	// Add other test cases...
}

func TestMightyMap_BadgerStorage_Encryption(t *testing.T) {
	// Testing MightyMap with Badger storage implementation
	ctx := context.Background()

	store := storage.NewMightyMapBadgerStorage[int, string](
		storage.WithMemoryStorage(true),
		storage.WithDetectConflicts(false),
		storage.WithEncryptionKey("test-key-123456789012345"),
		storage.WithEncryptionKeyRotationDuration(10*24*time.Hour),
	)
	cm := mightymap.New[int, string](true, store)

	// Repeat the same test cases as in TestMightyMap_DefaultStorage

	t.Run("Store and Load", func(t *testing.T) {
		cm.Store(ctx, 1, "one")
		value, ok := cm.Load(ctx, 1)
		if !ok || value != "one" {
			t.Errorf("Expected to load 'one', got '%v'", value)
		}
	})

	err := cm.Close(ctx)
	if err != nil {
		t.Errorf("Error closing map: %v", err)
	}

	// Add other test cases...
}
