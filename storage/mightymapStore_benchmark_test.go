package storage_test

import (
	"context"
	"testing"

	"github.com/thisisdevelopment/mightymap/storage"
)

var ctx = context.Background()

func BenchmarkDefaultStorageStore(b *testing.B) {
	store := storage.NewMightyMapDefaultStorage[int, string]()
	for i := 0; i < b.N; i++ {
		store.Store(ctx, i, "value")
	}
}

func BenchmarkDefaultStorageLoad(b *testing.B) {
	store := storage.NewMightyMapDefaultStorage[int, string]()
	// Pre-populate the store
	for i := 0; i < 1000000; i++ {
		store.Store(ctx, i, "value")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Load(ctx, i%1000000)
	}
}

func BenchmarkDefaultStorageDelete(b *testing.B) {
	store := storage.NewMightyMapDefaultStorage[int, string]()
	// Pre-populate the store
	for i := 0; i < 1000000; i++ {
		store.Store(ctx, i, "value")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Delete(ctx, i%1000000)
	}
}
