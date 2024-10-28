package storage_test

import (
	"testing"

	"github.com/thisisdevelopment/mightymap/storage"
)

func BenchmarkDefaultStorageStore(b *testing.B) {
	store := storage.NewConcurrentMapDefaultStorage[int, string]()
	for i := 0; i < b.N; i++ {
		store.Store(i, "value")
	}
}

func BenchmarkDefaultStorageLoad(b *testing.B) {
	store := storage.NewConcurrentMapDefaultStorage[int, string]()
	// Pre-populate the store
	for i := 0; i < 1000000; i++ {
		store.Store(i, "value")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Load(i % 1000000)
	}
}

func BenchmarkDefaultStorageDelete(b *testing.B) {
	store := storage.NewConcurrentMapDefaultStorage[int, string]()
	// Pre-populate the store
	for i := 0; i < 1000000; i++ {
		store.Store(i, "value")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Delete(i % 1000000)
	}
}
