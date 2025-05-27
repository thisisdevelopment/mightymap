package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/thisisdevelopment/mightymap"
	"github.com/thisisdevelopment/mightymap/storage"
)

// TestBadgerEdgeCases tests various edge cases for BadgerDB storage
func TestBadgerEdgeCases(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "badger-edge-cases-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ctx := context.Background()
	dbPath := filepath.Join(tempDir, "badger-db")

	// Create storage
	store := storage.NewMightyMapBadgerStorage[string, []byte](
		storage.WithTempDir(dbPath),
		storage.WithSyncWrites(true),
	)
	mm := mightymap.New[string, []byte](true, store)
	defer mm.Close(ctx)

	t.Run("EmptyByteSlice", func(t *testing.T) {
		key := "empty-value"
		emptyValue := []byte{}

		// Store empty byte slice
		mm.Store(ctx, key, emptyValue)

		// Retrieve
		value, ok := mm.Load(ctx, key)
		if !ok {
			t.Fatal("Failed to load empty byte slice")
		}

		if len(value) != 0 {
			t.Fatalf("Expected empty byte slice, got %d bytes", len(value))
		}
	})

	t.Run("LargeValue", func(t *testing.T) {
		key := "large-value"
		// Create a 500KB byte slice (well under BadgerDB's 1MB default limit)
		largeValue := make([]byte, 500*1024)
		for i := range largeValue {
			largeValue[i] = byte(i % 256)
		}

		// Store large value
		mm.Store(ctx, key, largeValue)

		// Retrieve
		value, ok := mm.Load(ctx, key)
		if !ok {
			t.Fatal("Failed to load large value")
		}

		if len(value) != len(largeValue) {
			t.Fatalf("Value size mismatch. Expected %d bytes, got %d bytes", len(largeValue), len(value))
		}

		// Check a few sample bytes to verify content
		checkIndices := []int{0, 1000, 10000, 100000, len(largeValue) - 1}
		for _, idx := range checkIndices {
			if value[idx] != largeValue[idx] {
				t.Fatalf("Value mismatch at index %d. Expected %d, got %d", idx, largeValue[idx], value[idx])
			}
		}
	})

	t.Run("SpecialCharactersInKeys", func(t *testing.T) {
		specialKeys := []string{
			"key!@#$%^&*()_+",
			"key\nwith\nnewlines",
			"key\\with\\backslashes",
			"key\"with\"quotes",
			"key\twith\ttabs",
			"key\x00with\x00nullbytes",
		}

		for _, key := range specialKeys {
			value := []byte("value for " + key)

			// Store
			mm.Store(ctx, key, value)

			// Retrieve
			retrieved, ok := mm.Load(ctx, key)
			if !ok {
				t.Fatalf("Failed to load value for key with special characters: %q", key)
			}

			if string(retrieved) != string(value) {
				t.Fatalf("Value mismatch for key %q. Expected %q, got %q", key, string(value), string(retrieved))
			}
		}
	})

	t.Run("UnicodeKeys", func(t *testing.T) {
		unicodeKeys := []string{
			"키_한글",          // Korean
			"кључ_ћирилица", // Cyrillic
			"مفتاح_عربي",    // Arabic
			"键_中文",          // Chinese
			"🔑_🔒_🔓",         // Emoji
			"ключ_кириллица_日本語_한글", // Mixed scripts
		}

		for _, key := range unicodeKeys {
			value := []byte("value for " + key)

			// Store
			mm.Store(ctx, key, value)

			// Retrieve
			retrieved, ok := mm.Load(ctx, key)
			if !ok {
				t.Fatalf("Failed to load value for Unicode key: %q", key)
			}

			if string(retrieved) != string(value) {
				t.Fatalf("Value mismatch for Unicode key %q. Expected %q, got %q", key, string(value), string(retrieved))
			}
		}
	})
}

// TestConcurrentBadgerAccess tests concurrent access to the same key
func TestConcurrentBadgerAccess(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "badger-concurrent-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ctx := context.Background()
	dbPath := filepath.Join(tempDir, "badger-db")

	// Create storage
	store := storage.NewMightyMapBadgerStorage[string, []byte](
		storage.WithTempDir(dbPath),
		storage.WithSyncWrites(true),
	)
	mm := mightymap.New[string, []byte](true, store)
	defer mm.Close(ctx)

	const (
		numGoroutines = 10
		numOperations = 100
	)

	// Shared key that all goroutines will access
	sharedKey := "concurrent-key"

	// Store initial value
	initialValue := []byte("initial")
	mm.Store(ctx, sharedKey, initialValue)

	// Create wait group for synchronization
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Launch goroutines
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			// Perform operations
			for j := 0; j < numOperations; j++ {
				// Read
				value, ok := mm.Load(ctx, sharedKey)
				if !ok {
					t.Errorf("Goroutine %d: Failed to load value at operation %d", id, j)
					continue
				}

				// Write (slightly modified value)
				newValue := append([]byte{}, value...)
				mm.Store(ctx, sharedKey, newValue)

				// Small sleep to increase chance of race conditions
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify key still exists
	_, ok := mm.Load(ctx, sharedKey)
	if !ok {
		t.Fatal("Key no longer exists after concurrent operations")
	}
}
