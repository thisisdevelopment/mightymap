package mightymap_test

import (
	"testing"

	"github.com/thisisdevelopment/mightymap"
)

func TestConcurrentMap_DefaultStorage(t *testing.T) {
	// Testing ConcurrentMap with default storage (map[K]V)

	cm := mightymap.NewConcurrentMap[int, string](true)

	t.Run("Store and Load", func(t *testing.T) {
		cm.Store(1, "one")
		value, ok := cm.Load(1)
		if !ok || value != "one" {
			t.Errorf("Expected to load 'one', got '%v'", value)
		}
	})

	t.Run("Has", func(t *testing.T) {
		if !cm.Has(1) {
			t.Errorf("Expected key 1 to exist")
		}
		if cm.Has(2) {
			t.Errorf("Did not expect key 2 to exist")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		cm.Delete(1)
		if cm.Has(1) {
			t.Errorf("Expected key 1 to be deleted")
		}
	})

	t.Run("Range", func(t *testing.T) {
		cm.Store(2, "two")
		cm.Store(3, "three")
		keys := make(map[int]bool)
		cm.Range(func(key int, value string) bool {
			keys[key] = true
			return true
		})
		if len(keys) != 2 {
			t.Errorf("Expected to range over 2 keys, got %d", len(keys))
		}
	})

	t.Run("Pop", func(t *testing.T) {
		value, ok := cm.Pop(2)
		if !ok || value != "two" {
			t.Errorf("Expected to pop 'two', got '%v'", value)
		}
		if cm.Has(2) {
			t.Errorf("Expected key 2 to be deleted after Pop")
		}
	})

	t.Run("Pop Nonexistent Key", func(t *testing.T) {
		value, ok := cm.Pop(42)
		if ok {
			t.Errorf("Expected to pop 'nil', got '%v'", value)
		}
	})

	t.Run("Next", func(t *testing.T) {
		cm.Store(4, "four")
		value, key, ok := cm.Next()
		if !ok {
			t.Errorf("Expected to get next value")
		}
		if value != "three" && value != "four" {
			t.Errorf("Unexpected value '%v'", value)
		}
		if cm.Has(key) {
			t.Errorf("Expected key %d to be deleted after Next", key)
		}
	})

	t.Run("Len", func(t *testing.T) {
		if cm.Len() != 1 {
			t.Errorf("Expected map to have 1 item, got %d", cm.Len())
		}
	})

	t.Run("Clear", func(t *testing.T) {
		cm.Store(5, "five")
		cm.Clear()
		if cm.Len() != 0 {
			t.Errorf("Expected map to be cleared")
		}
	})
}
