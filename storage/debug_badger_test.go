package storage_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/thisisdevelopment/mightymap"
	"github.com/thisisdevelopment/mightymap/storage"
)

// Modified version of the BadgerDB storage that adds debugging
func newDebugBadgerStorage[K comparable, V any](dbPath string) storage.IMightyMapStorage[K, V] {
	store := storage.NewMightyMapBadgerStorage[K, V](
		storage.WithMemoryStorage(false),
		storage.WithTempDir(dbPath),
		storage.WithLoggingLevel(3), // ERROR level
	)
	return store
}

// This function prints the exact byte representation of the key
func debugKey(key string) {
	fmt.Printf("Key length: %d bytes\n", len(key))
	fmt.Printf("Key as string: %q\n", key)
	fmt.Print("Key bytes: [")
	for i, b := range []byte(key) {
		if i > 0 {
			fmt.Print(" ")
		}
		fmt.Printf("%02x", b)
	}
	fmt.Println("]")
}

func TestDebugBadgerStorageWithByteSlices(t *testing.T) {
	// Create a temporary directory for the persistent BadgerDB
	tempDir, err := os.MkdirTemp("", "badger-debug-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Using temp dir: %s", tempDir)
	defer os.RemoveAll(tempDir)

	ctx := context.Background()
	dbPath := filepath.Join(tempDir, "badger-db")

	// The specific key from the reported error
	testKey := "localtest:credis.eukdlx.notset.en.skudetail.d29c3a4b-979f-4fce-b873-6d692e6e9c49"
	testValue := []byte("this is a test byte slice value")

	// Debug the key format
	fmt.Println("\n=== Debug Key Format ===")
	debugKey(testKey)

	log.Println("\n=== Step 1: Store value ===")
	// First create a map and store a byte slice
	{
		store := newDebugBadgerStorage[string, []byte](dbPath)
		mm := mightymap.New[string, []byte](true, store)

		// Store the byte slice value
		log.Println("Storing value...")
		mm.Store(ctx, testKey, testValue)
		log.Printf("Stored %d bytes for key: %s", len(testValue), testKey)

		// Immediately verify it can be retrieved
		log.Println("Immediately loading value...")
		value, ok := mm.Load(ctx, testKey)
		if !ok {
			t.Fatalf("Failed to immediately load value for key: %s", testKey)
		}

		if string(value) != string(testValue) {
			t.Errorf("Immediate load: value mismatch\nExpected: %q\nGot: %q",
				string(testValue), string(value))
		} else {
			log.Println("Immediate load successful ✓")
		}

		log.Println("Closing first BadgerDB instance...")
		mm.Close(ctx)
	}

	log.Println("\n=== Step 2: Reopen and verify persistence ===")
	// Close and reopen the DB to test persistence
	{
		store := newDebugBadgerStorage[string, []byte](dbPath)
		mm := mightymap.New[string, []byte](true, store)
		defer mm.Close(ctx)

		// Retrieve the value after reopening
		log.Println("Loading value after reopening...")
		value, ok := mm.Load(ctx, testKey)

		log.Printf("Load result - ok: %v, value length: %d", ok, len(value))
		if !ok {
			t.Fatalf("Failed to load persisted value for key: %s", testKey)
		}

		if string(value) != string(testValue) {
			t.Errorf("Persistence test: value mismatch\nExpected: %q\nGot: %q",
				string(testValue), string(value))
		} else {
			log.Println("Persistence test successful ✓")
		}
	}

	// If we reach here, the test passed
	log.Println("\n=== Test Completed Successfully ===")
}
