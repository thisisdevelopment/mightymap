package mightymap_test

import (
	"testing"

	"github.com/thisisdevelopment/mightymap"
	"github.com/thisisdevelopment/mightymap/storage"
)

func FuzzConcurrentMap(f *testing.F) {
	// Seed the fuzzer with initial inputs
	f.Add(int(1), "one")
	f.Add(int(2), "two")
	f.Add(int(0), "")

	f.Fuzz(func(t *testing.T, key int, value string) {
		// Limit the size of the string value
		if len(value) > 1000 {
			value = value[:1000]
		}

		// Create storages
		storages := []struct {
			name  string
			store storage.IConcurrentMapStorage[int, string]
		}{
			{"DefaultStorage", storage.NewConcurrentMapDefaultStorage[int, string]()},
			{"SwissStorage", storage.NewConcurrentMapSwissStorage[int, string]()},
			{"BadgerStorage", newBadgerStorageForFuzz()},
		}

		for _, s := range storages {
			t.Run(s.name, func(t *testing.T) {
				cm := mightymap.NewConcurrentMap[int, string](true, s.store)

				// Perform operations
				cm.Store(key, value)

				// Load the stored value
				loadedValue, ok := cm.Load(key)
				if !ok {
					t.Errorf("Failed to load key: %d", key)
				}
				if loadedValue != value {
					t.Errorf("Loaded value '%s' does not match stored value '%s'", loadedValue, value)
				}

				// Test Pop
				poppedValue, ok := cm.Pop(key)
				if !ok {
					t.Errorf("Failed to pop key: %d", key)
				}
				if poppedValue != value {
					t.Errorf("Popped value '%s' does not match stored value '%s'", poppedValue, value)
				}

				// Ensure the key is deleted
				_, ok = cm.Load(key)
				if ok {
					t.Errorf("Key '%d' should have been deleted after Pop", key)
				}

				// Clear the map to free up memory
				cm.Clear()
				cm = nil
			})
		}
	})
}

func newBadgerStorageForFuzz() storage.IConcurrentMapStorage[int, string] {
	// Use in-memory Badger storage for fuzzing
	return storage.NewConcurrentMapBadgerStorage[int, string](
		storage.WithMemoryStorage(true),
	)
}
