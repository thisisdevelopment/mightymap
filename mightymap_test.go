package mightymap_test

import (
	"context"
	"testing"

	"github.com/thisisdevelopment/mightymap"
)

func TestMightyMap_DefaultStorage(t *testing.T) {
	// Testing MightyMap with default storage (map[K]V)
	ctx := context.Background()
	cm := mightymap.New[int, string](true)

	t.Run("Store and Load", func(t *testing.T) {
		cm.Store(ctx, 1, "one")
		value, ok := cm.Load(ctx, 1)
		if !ok || value != "one" {
			t.Errorf("Expected to load 'one', got '%v'", value)
		}
	})

	t.Run("Has", func(t *testing.T) {
		if !cm.Has(ctx, 1) {
			t.Errorf("Expected key 1 to exist")
		}
		if cm.Has(ctx, 2) {
			t.Errorf("Did not expect key 2 to exist")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		cm.Delete(ctx, 1)
		if cm.Has(ctx, 1) {
			t.Errorf("Expected key 1 to be deleted")
		}
	})

	t.Run("Range", func(t *testing.T) {
		cm.Store(ctx, 2, "two")
		cm.Store(ctx, 3, "three")
		keys := make(map[int]bool)
		cm.Range(ctx, func(key int, value string) bool {
			keys[key] = true
			return true
		})
		if len(keys) != 2 {
			t.Errorf("Expected to range over 2 keys, got %d", len(keys))
		}
	})

	t.Run("Pop", func(t *testing.T) {
		value, ok := cm.Pop(ctx, 2)
		if !ok || value != "two" {
			t.Errorf("Expected to pop 'two', got '%v'", value)
		}
		if cm.Has(ctx, 2) {
			t.Errorf("Expected key 2 to be deleted after Pop")
		}
	})

	t.Run("Pop Nonexistent Key", func(t *testing.T) {
		value, ok := cm.Pop(ctx, 42)
		if ok {
			t.Errorf("Expected to pop 'nil', got '%v'", value)
		}
	})

	t.Run("Next", func(t *testing.T) {
		cm.Store(ctx, 4, "four")
		value, key, ok := cm.Next(ctx)
		if !ok {
			t.Errorf("Expected to get next value")
		}
		if value != "three" && value != "four" {
			t.Errorf("Unexpected value '%v'", value)
		}
		if cm.Has(ctx, key) {
			t.Errorf("Expected key %d to be deleted after Next", key)
		}
	})

	t.Run("Len", func(t *testing.T) {
		if cm.Len(ctx) != 1 {
			t.Errorf("Expected map to have 1 item, got %d", cm.Len(ctx))
		}
	})

	t.Run("Clear", func(t *testing.T) {
		cm.Store(ctx, 5, "five")
		cm.Clear(ctx)
		if cm.Len(ctx) != 0 {
			t.Errorf("Expected map to be cleared")
		}
	})
}
