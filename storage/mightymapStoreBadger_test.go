package storage

import (
	"context"
	"testing"
)

func TestMightyMapBadgerStorageDelete(t *testing.T) {
	store := NewMightyMapBadgerStorage[string, int](
		WithMemoryStorage(true),
	)
	defer store.Close(context.Background())

	ctx := context.Background()

	// Test Delete
	t.Run("Delete", func(t *testing.T) {
		store.Store(ctx, "key1", 1)
		store.Delete(ctx, "key1")
		_, ok := store.Load(ctx, "key1")
		if ok {
			t.Error("Delete() did not remove the key")
		}
	})

	// Test Delete non-existent key
	t.Run("Delete non-existent key", func(t *testing.T) {
		store.Delete(ctx, "nonexistent")
		// Should not panic
	})
}

func TestMightyMapBadgerStorageRange(t *testing.T) {
	store := NewMightyMapBadgerStorage[string, int](
		WithMemoryStorage(true),
	)
	defer store.Close(context.Background())

	ctx := context.Background()

	// Test Range
	t.Run("Range", func(t *testing.T) {
		store.Store(ctx, "key1", 1)
		store.Store(ctx, "key2", 2)
		count := 0
		store.Range(ctx, func(key string, value int) bool {
			count++
			return true
		})
		if count != 2 {
			t.Errorf("Range() visited %d items; want 2", count)
		}
	})

	// Test Range with empty store
	t.Run("Range empty store", func(t *testing.T) {
		store.Clear(ctx)
		count := 0
		store.Range(ctx, func(key string, value int) bool {
			count++
			return true
		})
		if count != 0 {
			t.Errorf("Range() visited %d items; want 0", count)
		}
	})

	// Test Range with early return
	t.Run("Range early return", func(t *testing.T) {
		store.Store(ctx, "key3", 3)
		store.Store(ctx, "key4", 4)
		count := 0
		store.Range(ctx, func(key string, value int) bool {
			count++
			return false // Stop after first item
		})
		if count != 1 {
			t.Errorf("Range() visited %d items; want 1", count)
		}
	})
}

func TestMightyMapBadgerStorageLen(t *testing.T) {
	store := NewMightyMapBadgerStorage[string, int](
		WithMemoryStorage(true),
	)
	defer store.Close(context.Background())

	ctx := context.Background()

	// Test Len
	t.Run("Len", func(t *testing.T) {
		store.Clear(ctx)
		store.Store(ctx, "key1", 1)
		store.Store(ctx, "key2", 2)
		if store.Len(ctx) != 2 {
			t.Errorf("Len() = %d; want 2", store.Len(ctx))
		}
	})

	// Test Len with empty store
	t.Run("Len empty store", func(t *testing.T) {
		store.Clear(ctx)
		if store.Len(ctx) != 0 {
			t.Errorf("Len() = %d; want 0", store.Len(ctx))
		}
	})
}

func TestMightyMapBadgerStorageClear(t *testing.T) {
	store := NewMightyMapBadgerStorage[string, int](
		WithMemoryStorage(true),
	)
	defer store.Close(context.Background())

	ctx := context.Background()

	// Test Clear
	t.Run("Clear", func(t *testing.T) {
		store.Store(ctx, "key1", 1)
		store.Store(ctx, "key2", 2)
		store.Clear(ctx)
		if store.Len(ctx) != 0 {
			t.Error("Clear() did not remove all items")
		}
	})

	// Test Clear empty store
	t.Run("Clear empty store", func(t *testing.T) {
		store.Clear(ctx)
		if store.Len(ctx) != 0 {
			t.Error("Clear() did not maintain empty state")
		}
	})
}

func TestMightyMapBadgerStorageNext(t *testing.T) {
	store := NewMightyMapBadgerStorage[string, int](
		WithMemoryStorage(true),
	)
	defer store.Close(context.Background())

	ctx := context.Background()

	// Test Next
	t.Run("Next", func(t *testing.T) {
		store.Store(ctx, "key1", 1)
		key, value, ok := store.Next(ctx)
		if !ok || key != "key1" || value != 1 {
			t.Errorf("Next() = %v, %v, %v; want key1, 1, true", key, value, ok)
		}
	})

	// Test Next with empty store
	t.Run("Next empty store", func(t *testing.T) {
		store.Clear(ctx)
		_, _, ok := store.Next(ctx)
		if ok {
			t.Error("Next() returned true for empty store")
		}
	})

	// Test Next with multiple items
	t.Run("Next multiple items", func(t *testing.T) {
		store.Clear(ctx)
		store.Store(ctx, "key2", 2)
		store.Store(ctx, "key3", 3)
		seen := make(map[string]bool)
		for i := 0; i < 2; i++ {
			key, value, ok := store.Next(ctx)
			if !ok {
				t.Error("Next() returned false before all items were seen")
			}
			seen[key] = true
			if value != i+2 {
				t.Errorf("Next() value = %v; want %v", value, i+2)
			}
		}
		if len(seen) != 2 {
			t.Error("Next() did not return all items")
		}
	})
}
