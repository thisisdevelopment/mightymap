package mightymap_test

import (
	"testing"

	"github.com/thisisdevelopment/mightymap"
	"github.com/thisisdevelopment/mightymap/storage"
)

func TestMightyMap_SwissStorage(t *testing.T) {
	// Testing MightyMap with SwissMap storage implementation

	store := storage.NewMightyMapSwissStorage[int, string]()
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
