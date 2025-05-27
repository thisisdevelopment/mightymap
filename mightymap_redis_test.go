package mightymap_test

import (
	"context"
	"testing"
	"time"

	"github.com/thisisdevelopment/mightymap"
	"github.com/thisisdevelopment/mightymap/storage"
)

func TestMightyMap_RedisStorage(t *testing.T) {
	ctx := context.Background()

	store := storage.NewMightyMapRedisStorage[int, string](
		storage.WithRedisExpire(1*time.Hour),
		storage.WithRedisTimeout(5*time.Second),
		storage.WithRedisMock(t),
	)
	cm := mightymap.New[int, string](true, store)

	// Clean up any existing data
	cm.Clear(ctx)

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
			t.Errorf("Expected pop to return false for nonexistent key, got value '%v'", value)
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
		// Clear first to have a known state
		cm.Clear(ctx)
		cm.Store(ctx, 1, "one")
		if cm.Len(ctx) != 1 {
			t.Errorf("Expected map to have 1 item, got %d", cm.Len(ctx))
		}
	})

	t.Run("Clear", func(t *testing.T) {
		cm.Store(ctx, 5, "five")
		cm.Clear(ctx)
		if cm.Len(ctx) != 0 {
			t.Errorf("Expected map to be empty after Clear, got length %d", cm.Len(ctx))
		}
	})

	t.Run("Close", func(t *testing.T) {
		err := cm.Close(ctx)
		if err != nil {
			t.Errorf("Error closing map: %v", err)
		}
	})
}

func TestMightyMap_RedisStorage_Configuration(t *testing.T) {
	ctx := context.Background()

	t.Run("Custom Configuration", func(t *testing.T) {
		store := storage.NewMightyMapRedisStorage[int, string](
			storage.WithRedisAddr("localhost:6379"),
			storage.WithRedisPassword(""),
			storage.WithRedisDB(0),
			storage.WithRedisPoolSize(10),
			storage.WithRedisMaxRetries(5),
			storage.WithRedisPrefix("test_"),
			storage.WithRedisExpire(1*time.Hour),
			storage.WithRedisTimeout(5*time.Second),
			storage.WithRedisMock(t),
		)
		cm := mightymap.New[int, string](true, store)

		cm.Store(ctx, 1, "one")
		value, ok := cm.Load(ctx, 1)
		if !ok || value != "one" {
			t.Errorf("Expected to load 'one', got '%v'", value)
		}

		err := cm.Close(ctx)
		if err != nil {
			t.Errorf("Error closing map: %v", err)
		}
	})
}

func TestMightyMap_RedisStorage_Concurrency(t *testing.T) {
	ctx := context.Background()

	store := storage.NewMightyMapRedisStorage[int, string](
		storage.WithRedisMock(t),
	)
	cm := mightymap.New[int, string](true, store)

	// Reference to existing concurrency test pattern
	t.Run("Concurrent Operations", func(t *testing.T) {
		const numOperations = 100
		done := make(chan bool)

		// Concurrent stores
		for i := 0; i < numOperations; i++ {
			go func(val int) {
				cm.Store(ctx, val, "value")
				done <- true
			}(i)
		}

		// Wait for all stores to complete
		for i := 0; i < numOperations; i++ {
			<-done
		}

		// Verify length
		if cm.Len(ctx) != numOperations {
			t.Errorf("Expected length %d, got %d", numOperations, cm.Len(ctx))
		}

		// Concurrent loads
		for i := 0; i < numOperations; i++ {
			go func(val int) {
				_, ok := cm.Load(ctx, val)
				if !ok {
					t.Errorf("Failed to load value for key %d", val)
				}
				done <- true
			}(i)
		}

		// Wait for all loads to complete
		for i := 0; i < numOperations; i++ {
			<-done
		}

		cm.Clear(ctx)
		err := cm.Close(ctx)
		if err != nil {
			t.Errorf("Error closing map: %v", err)
		}
	})
}

type Session struct {
	UserID string
}

func TestMightyMap_RedisStorage_ComplexObjects(t *testing.T) {
	ctx := context.Background()

	// Register the complex type with gob
	mightymap.RegisterType(Session{})

	store := storage.NewMightyMapRedisStorage[string, Session](
		storage.WithRedisMock(t),
	)
	cm := mightymap.New[string, Session](true, store)

	session := Session{UserID: "user123"}
	cm.Store(ctx, "session1", session)

	loaded, ok := cm.Load(ctx, "session1")
	if !ok {
		t.Errorf("Failed to load complex object")
	}
	if loaded.UserID != "user123" {
		t.Errorf("Expected UserID 'user123', got '%s'", loaded.UserID)
	}

	err := cm.Close(ctx)
	if err != nil {
		t.Errorf("Error closing map: %v", err)
	}
}
