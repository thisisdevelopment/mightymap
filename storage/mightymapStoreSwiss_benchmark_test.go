package storage_test

import (
	"testing"

	"github.com/thisisdevelopment/mightymap/storage"
)

func BenchmarkSwissStorageStore(b *testing.B) {
	store := storage.NewMightyMapSwissStorage[int, string]()
	for i := 0; i < b.N; i++ {
		store.Store(ctx, i, "value")
	}
}

func BenchmarkSwissStorageLoad(b *testing.B) {
	store := storage.NewMightyMapSwissStorage[int, string]()
	// Pre-populate the store
	for i := 0; i < 1000000; i++ {
		store.Store(ctx, i, "value")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Load(ctx, i%1000000)
	}
}

func BenchmarkSwissStorageDelete(b *testing.B) {
	store := storage.NewMightyMapSwissStorage[int, string]()
	// Pre-populate the store
	for i := 0; i < 1000000; i++ {
		store.Store(ctx, i, "value")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Delete(ctx, i%1000000)
	}
}
