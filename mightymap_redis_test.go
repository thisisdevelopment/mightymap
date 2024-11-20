package mightymap_test

import (
	"testing"
	"time"

	"github.com/thisisdevelopment/mightymap"
	"github.com/thisisdevelopment/mightymap/storage"
)

func TestMightyMap_RedisStorage(t *testing.T) {
	store := storage.NewMightyMapRedisStorage[int, string](
		storage.WithRedisExpire(1*time.Hour),
		storage.WithRedisTimeout(5*time.Second),
		storage.WithRedisMock(t),
	)
	cm := mightymap.New[int, string](true, store)

	// Clean up any existing data
	cm.Clear()

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
			t.Errorf("Expected pop to return false for nonexistent key, got value '%v'", value)
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
		// Clear first to have a known state
		cm.Clear()
		cm.Store(1, "one")
		if cm.Len() != 1 {
			t.Errorf("Expected map to have 1 item, got %d", cm.Len())
		}
	})

	t.Run("Clear", func(t *testing.T) {
		cm.Store(5, "five")
		cm.Clear()
		if cm.Len() != 0 {
			t.Errorf("Expected map to be empty after Clear, got length %d", cm.Len())
		}
	})

	t.Run("Close", func(t *testing.T) {
		err := cm.Close()
		if err != nil {
			t.Errorf("Error closing map: %v", err)
		}
	})
}

func TestMightyMap_RedisStorage_Configuration(t *testing.T) {
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

		cm.Store(1, "one")
		value, ok := cm.Load(1)
		if !ok || value != "one" {
			t.Errorf("Expected to load 'one', got '%v'", value)
		}

		err := cm.Close()
		if err != nil {
			t.Errorf("Error closing map: %v", err)
		}
	})
}

func TestMightyMap_RedisStorage_Concurrency(t *testing.T) {
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
				cm.Store(val, "value")
				done <- true
			}(i)
		}

		// Wait for all stores to complete
		for i := 0; i < numOperations; i++ {
			<-done
		}

		// Verify length
		if cm.Len() != numOperations {
			t.Errorf("Expected length %d, got %d", numOperations, cm.Len())
		}

		// Concurrent loads
		for i := 0; i < numOperations; i++ {
			go func(val int) {
				_, ok := cm.Load(val)
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

		cm.Clear()
		err := cm.Close()
		if err != nil {
			t.Errorf("Error closing map: %v", err)
		}
	})
}

type Session struct {
	UserID string
}

func TestMightyMap_RedisStorage_ComplexObjects(t *testing.T) {
	store := storage.NewMightyMapRedisStorage[string, *Session](
		storage.WithRedisExpire(1*time.Hour),
		storage.WithRedisTimeout(5*time.Second),
		storage.WithRedisPrefix("session_"),

		// storage.WithRedisMock(t),
	)
	cm := mightymap.New[string, *Session](true, store)

	// Clean up any existing data
	cm.Clear()

	t.Run("Store and Load", func(t *testing.T) {
		cm.Store("123", &Session{UserID: "123"})
		value, ok := cm.Load("123")
		if !ok || value.UserID != "123" {
			t.Errorf("Expected to load 'session_123', got '%v'", value)
		}
	})
}
