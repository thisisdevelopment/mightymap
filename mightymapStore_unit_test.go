package mightymap_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thisisdevelopment/mightymap"
)

func TestMightyMapStore_Unit(t *testing.T) {
	ctx := context.Background()

	t.Run("Basic Operations", func(t *testing.T) {
		store := mightymap.New[string, string](true)
		defer store.Close(ctx)

		// Test Store and Load
		store.Store(ctx, "key1", "value1")

		val, ok := store.Load(ctx, "key1")
		require.True(t, ok)
		assert.Equal(t, "value1", val)

		// Test Delete
		store.Delete(ctx, "key1")

		_, ok = store.Load(ctx, "key1")
		assert.False(t, ok)
	})

	t.Run("Has Operation", func(t *testing.T) {
		store := mightymap.New[string, string](true)
		defer store.Close(ctx)

		// Test Has with existing key
		store.Store(ctx, "key1", "value1")
		assert.True(t, store.Has(ctx, "key1"))

		// Test Has with non-existing key
		assert.False(t, store.Has(ctx, "non_existent"))
	})

	t.Run("Concurrent Operations", func(t *testing.T) {
		store := mightymap.New[string, string](true)
		defer store.Close(ctx)

		// Test concurrent Store operations
		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func(idx int) {
				key := fmt.Sprintf("concurrent_key_%d", idx)
				store.Store(ctx, key, fmt.Sprintf("value_%d", idx))
				done <- true
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < 10; i++ {
			<-done
		}

		// Verify all values were stored correctly
		for i := 0; i < 10; i++ {
			key := fmt.Sprintf("concurrent_key_%d", i)
			val, ok := store.Load(ctx, key)
			require.True(t, ok)
			assert.Equal(t, fmt.Sprintf("value_%d", i), val)
		}
	})

	t.Run("Pop Operation", func(t *testing.T) {
		store := mightymap.New[string, string](true)
		defer store.Close(ctx)

		// Test Pop existing key
		store.Store(ctx, "pop_key", "pop_value")
		val, ok := store.Pop(ctx, "pop_key")
		require.True(t, ok)
		assert.Equal(t, "pop_value", val)

		// Verify key is deleted after Pop
		assert.False(t, store.Has(ctx, "pop_key"))

		// Test Pop non-existent key
		_, ok = store.Pop(ctx, "non_existent")
		assert.False(t, ok)
	})

	t.Run("Range Operation", func(t *testing.T) {
		store := mightymap.New[string, string](true)
		defer store.Close(ctx)

		// Store test data
		testData := map[string]string{
			"key1": "value1",
			"key2": "value2",
			"key3": "value3",
		}

		for k, v := range testData {
			store.Store(ctx, k, v)
		}

		// Test Range
		found := make(map[string]string)
		store.Range(ctx, func(key string, value string) bool {
			found[key] = value
			return true
		})

		assert.Equal(t, testData, found)
	})

	t.Run("Len and Clear Operations", func(t *testing.T) {
		store := mightymap.New[string, string](true)
		defer store.Close(ctx)

		// Test initial length
		assert.Equal(t, 0, store.Len(ctx))

		// Add some data
		store.Store(ctx, "key1", "value1")
		store.Store(ctx, "key2", "value2")
		assert.Equal(t, 2, store.Len(ctx))

		// Test Clear
		store.Clear(ctx)
		assert.Equal(t, 0, store.Len(ctx))
	})

	t.Run("Overwrite Behavior", func(t *testing.T) {
		// Test with overwrite allowed
		storeWithOverwrite := mightymap.New[string, string](true)
		defer storeWithOverwrite.Close(ctx)

		storeWithOverwrite.Store(ctx, "key1", "value1")
		storeWithOverwrite.Store(ctx, "key1", "value2")
		val, ok := storeWithOverwrite.Load(ctx, "key1")
		require.True(t, ok)
		assert.Equal(t, "value2", val)

		// Test with overwrite disabled
		storeNoOverwrite := mightymap.New[string, string](false)
		defer storeNoOverwrite.Close(ctx)

		storeNoOverwrite.Store(ctx, "key1", "value1")
		storeNoOverwrite.Store(ctx, "key1", "value2")
		val, ok = storeNoOverwrite.Load(ctx, "key1")
		require.True(t, ok)
		assert.Equal(t, "value1", val) // Should still be original value
	})
}
