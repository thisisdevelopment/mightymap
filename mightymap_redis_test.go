package mightymap_test

import (
	"context"
	"fmt"
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
			storage.WithRedisUsername("testuser"),
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

	t.Run("Redis ACL Authentication", func(t *testing.T) {
		store := storage.NewMightyMapRedisStorage[int, string](
			storage.WithRedisMock(t),
			storage.WithRedisUsername("acl_user"),
			storage.WithRedisPassword("acl_password"),
		)
		cm := mightymap.New[int, string](true, store)

		cm.Store(ctx, 1, "acl_value")
		value, ok := cm.Load(ctx, 1)
		if !ok || value != "acl_value" {
			t.Errorf("Expected to load 'acl_value', got '%v'", value)
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

// Session is an example struct for testing complex object storage
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

// ExampleNewMightyMapRedisStorage demonstrates Redis username authentication configuration
func ExampleNewMightyMapRedisStorage() {
	// Configure Redis storage with username authentication (Redis 6.0+ ACL)
	store := storage.NewMightyMapRedisStorage[string, string](
		storage.WithRedisUsername("myuser"),
		storage.WithRedisPassword("mypassword"),
		storage.WithRedisAddr("localhost:6379"),
	)

	mm := mightymap.New[string, string](true, store)
	defer mm.Close(context.Background())

	fmt.Println("Redis storage configured with username authentication")

	// Output: Redis storage configured with username authentication
}

// Example showing different Redis authentication configurations
func Example_redisAuthenticationOptions() {
	ctx := context.Background()

	// Example 1: No authentication (development/local Redis)
	store1 := storage.NewMightyMapRedisStorage[string, int](
	// No authentication options needed for local development
	)
	mm1 := mightymap.New[string, int](true, store1)
	mm1.Store(ctx, "key1", 100)
	defer mm1.Close(ctx)

	// Example 2: Password-only authentication (legacy Redis < 6.0)
	store2 := storage.NewMightyMapRedisStorage[string, int](
		storage.WithRedisPassword("your-redis-password"),
	)
	mm2 := mightymap.New[string, int](true, store2)
	mm2.Store(ctx, "key2", 200)
	defer mm2.Close(ctx)

	// Example 3: Username + Password authentication (Redis 6.0+ ACL)
	store3 := storage.NewMightyMapRedisStorage[string, int](
		storage.WithRedisUsername("app-user"),        // ACL username
		storage.WithRedisPassword("secure-password"), // ACL password
		storage.WithRedisDB(1),                       // Use database 1
	)
	mm3 := mightymap.New[string, int](true, store3)
	mm3.Store(ctx, "key3", 300)
	defer mm3.Close(ctx)

	// Example 4: Full production configuration with username
	store4 := storage.NewMightyMapRedisStorage[string, string](
		storage.WithRedisAddr("redis.example.com:6379"),  // Production Redis address
		storage.WithRedisUsername("production-user"),     // Production ACL user
		storage.WithRedisPassword("production-password"), // Production ACL password
		storage.WithRedisDB(2),                           // Dedicated database
		storage.WithRedisTLS(true),                       // Enable TLS encryption
		storage.WithRedisPoolSize(20),                    // Connection pool size
		storage.WithRedisTimeout(10*time.Second),         // Operation timeout
		storage.WithRedisPrefix("myapp:"),                // Key prefix for namespacing
	)
	mm4 := mightymap.New[string, string](true, store4)
	mm4.Store(ctx, "session:user123", "session-data")
	defer mm4.Close(ctx)
}
