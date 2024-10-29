package storage_test

import (
	"testing"

	"github.com/thisisdevelopment/mightymap/storage"
)

func BenchmarkSwissStorageStore(b *testing.B) {
	store := storage.NewMightyMapSwissStorage[int, string]()
	for i := 0; i < b.N; i++ {
		store.Store(i, "value")
	}
}

func BenchmarkSwissStorageLoad(b *testing.B) {
	store := storage.NewMightyMapSwissStorage[int, string]()
	// Pre-populate the store
	for i := 0; i < 1000000; i++ {
		store.Store(i, "value")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Load(i % 1000000)
	}
}

func BenchmarkSwissStorageDelete(b *testing.B) {
	store := storage.NewMightyMapSwissStorage[int, string]()
	// Pre-populate the store
	for i := 0; i < 1000000; i++ {
		store.Store(i, "value")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Delete(i % 1000000)
	}
}
