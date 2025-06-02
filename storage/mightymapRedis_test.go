package storage

import (
	"context"
	"crypto/tls"
	"testing"
	"time"
)

func TestMightyMapRedisStorage(t *testing.T) {
	// Test with mock Redis
	store := NewMightyMapRedisStorage[string, int](
		WithRedisMock(t),
		WithRedisPrefix("test:"),
	)
	defer store.Close(context.Background())

	ctx := context.Background()

	// Test Store and Load
	t.Run("Store and Load", func(t *testing.T) {
		store.Store(ctx, "key1", 1)
		value, ok := store.Load(ctx, "key1")
		if !ok || value != 1 {
			t.Errorf("Load() = %v, %v; want 1, true", value, ok)
		}
	})

	// Test Delete
	t.Run("Delete", func(t *testing.T) {
		store.Store(ctx, "key2", 2)
		store.Delete(ctx, "key2")
		_, ok := store.Load(ctx, "key2")
		if ok {
			t.Error("Delete() did not remove the key")
		}
	})

	// Test Range
	t.Run("Range", func(t *testing.T) {
		store.Clear(ctx)
		store.Store(ctx, "key3", 3)
		store.Store(ctx, "key4", 4)
		count := 0
		store.Range(ctx, func(key string, value int) bool {
			count++
			return true
		})
		if count != 2 {
			t.Errorf("Range() visited %d items; want 2", count)
		}
	})

	// Test Len
	t.Run("Len", func(t *testing.T) {
		store.Clear(ctx)
		store.Store(ctx, "key5", 5)
		store.Store(ctx, "key6", 6)
		if store.Len(ctx) != 2 {
			t.Errorf("Len() = %d; want 2", store.Len(ctx))
		}
	})

	// Test Clear
	t.Run("Clear", func(t *testing.T) {
		store.Clear(ctx)
		if store.Len(ctx) != 0 {
			t.Error("Clear() did not remove all items")
		}
	})

	// Test Next
	t.Run("Next", func(t *testing.T) {
		store.Store(ctx, "key7", 7)
		key, value, ok := store.Next(ctx)
		if !ok || key != "key7" || value != 7 {
			t.Errorf("Next() = %v, %v, %v; want key7, 7, true", key, value, ok)
		}
	})
}

func TestMightyMapRedisStorageOptions(t *testing.T) {
	// Test with various Redis options
	t.Run("Redis options", func(t *testing.T) {
		store := NewMightyMapRedisStorage[string, int](
			WithRedisMock(t),
			WithRedisAddr("localhost:6379"),
			WithRedisUsername("testuser"),
			WithRedisPassword("password"),
			WithRedisDB(1),
			WithRedisPoolSize(10),
			WithRedisMaxRetries(3),
			WithRedisTimeout(5*time.Second),
			WithRedisExpire(10*time.Second),
			WithRedisTLS(true),
			WithRedisPrefix("test:"),
		)
		defer store.Close(context.Background())

		ctx := context.Background()
		store.Store(ctx, "key1", 1)
		value, ok := store.Load(ctx, "key1")
		if !ok || value != 1 {
			t.Errorf("Load() = %v, %v; want 1, true", value, ok)
		}
	})

	// Test with username only
	t.Run("Redis username option", func(t *testing.T) {
		store := NewMightyMapRedisStorage[string, int](
			WithRedisMock(t),
			WithRedisUsername("myuser"),
		)
		defer store.Close(context.Background())

		ctx := context.Background()
		store.Store(ctx, "userkey", 42)
		value, ok := store.Load(ctx, "userkey")
		if !ok || value != 42 {
			t.Errorf("Load() = %v, %v; want 42, true", value, ok)
		}
	})

	// Test with TLS configuration
	t.Run("Redis TLS config", func(t *testing.T) {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: "localhost",
		}
		store := NewMightyMapRedisStorage[string, int](
			WithRedisMock(t),
			WithRedisTLSConfig(tlsConfig),
		)
		defer store.Close(context.Background())

		ctx := context.Background()
		store.Store(ctx, "key1", 1)
		value, ok := store.Load(ctx, "key1")
		if !ok || value != 1 {
			t.Errorf("Load() = %v, %v; want 1, true", value, ok)
		}
	})
}

func TestMightyMapRedisStorageConcurrent(t *testing.T) {
	store := NewMightyMapRedisStorage[string, int](
		WithRedisMock(t),
	)
	defer store.Close(context.Background())

	ctx := context.Background()
	done := make(chan bool)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func(i int) {
			store.Store(ctx, "key"+string(rune(i)), i)
			done <- true
		}(i)
	}

	// Wait for all writes to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all values were stored
	for i := 0; i < 10; i++ {
		value, ok := store.Load(ctx, "key"+string(rune(i)))
		if !ok || value != i {
			t.Errorf("Load() = %v, %v; want %d, true", value, ok, i)
		}
	}
}

func TestMightyMapRedisStorageEdgeCases(t *testing.T) {
	store := NewMightyMapRedisStorage[string, int](
		WithRedisMock(t),
	)
	defer store.Close(context.Background())

	ctx := context.Background()

	// Test empty key
	t.Run("Empty key", func(t *testing.T) {
		store.Store(ctx, "", 0)
		value, ok := store.Load(ctx, "")
		if !ok || value != 0 {
			t.Errorf("Load() = %v, %v; want 0, true", value, ok)
		}
	})

	// Test non-existent key
	t.Run("Non-existent key", func(t *testing.T) {
		_, ok := store.Load(ctx, "nonexistent")
		if ok {
			t.Error("Load() returned true for non-existent key")
		}
	})

	// Test delete non-existent key
	t.Run("Delete non-existent key", func(t *testing.T) {
		store.Delete(ctx, "nonexistent")
		// Should not panic
	})

	// Test range with empty store
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

	// Test next with empty store
	t.Run("Next empty store", func(t *testing.T) {
		store.Clear(ctx)
		_, _, ok := store.Next(ctx)
		if ok {
			t.Error("Next() returned true for empty store")
		}
	})
}
