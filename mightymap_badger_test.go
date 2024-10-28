package mightymap_test

import (
	"testing"

	"github.com/thisisdevelopment/mightymap"
	"github.com/thisisdevelopment/mightymap/storage"
)

func TestConcurrentMap_BadgerStorage(t *testing.T) {
	// Testing ConcurrentMap with Badger storage implementation

	store := storage.NewConcurrentMapBadgerStorage[int, string]()
	cm := mightymap.NewConcurrentMap[int, string](true, store)

	// Repeat the same test cases as in TestConcurrentMap_DefaultStorage

	t.Run("Store and Load", func(t *testing.T) {
		cm.Store(1, "one")
		value, ok := cm.Load(1)
		if !ok || value != "one" {
			t.Errorf("Expected to load 'one', got '%v'", value)
		}
	})

	// Add other test cases...
}