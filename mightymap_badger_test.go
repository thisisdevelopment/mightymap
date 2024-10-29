package mightymap_test

import (
	"testing"

	"github.com/thisisdevelopment/mightymap"
	"github.com/thisisdevelopment/mightymap/storage"
)

func TestMightyMap_BadgerStorage(t *testing.T) {
	// Testing MightyMap with Badger storage implementation

	store := storage.NewMightyMapBadgerStorage[int, string](
		storage.WithMemoryStorage(true),
		storage.WithDetectConflicts(false),
	)
	cm := mightymap.New[int, string](true, store)

	// Repeat the same test cases as in TestMightyMap_DefaultStorage

	t.Run("Store and Load", func(t *testing.T) {
		cm.Store(1, "one")
		value, ok := cm.Load(1)
		if !ok || value != "one" {
			t.Errorf("Expected to load 'one', got '%v'", value)
		}
	})

	// Add other test cases...
}
