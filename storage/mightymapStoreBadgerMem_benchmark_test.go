package storage_test

import (
	"testing"

	"github.com/thisisdevelopment/mightymap/storage"
)

func BenchmarkBadgerMemStorageStore(b *testing.B) {
	store := storage.NewMightyMapBadgerStorage[int, string]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Store(i, "value")
	}
}

func BenchmarkBadgerMemStorageLoad(b *testing.B) {
	store := storage.NewMightyMapBadgerStorage[int, string]()
	// Pre-populate the store
	for i := 0; i < 100000; i++ {
		store.Store(i, "value")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Load(i % 100000)
	}
}

func BenchmarkBadgerMemStorageDelete(b *testing.B) {
	store := storage.NewMightyMapBadgerStorage[int, string]()
	// Pre-populate the store
	for i := 0; i < 100000; i++ {
		store.Store(i, "value")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Delete(i % 100000)
	}
}
