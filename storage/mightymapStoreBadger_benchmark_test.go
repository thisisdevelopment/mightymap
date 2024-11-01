package storage_test

import (
	"os"
	"testing"

	"github.com/thisisdevelopment/mightymap/storage"
)

func BenchmarkBadgerStorageStore(b *testing.B) {
	dir := os.TempDir() + "/badger_benchmark_store"
	defer os.RemoveAll(dir)

	store := storage.NewMightyMapBadgerStorage[int, string](
		storage.WithMemoryStorage(false),
		storage.WithTempDir(dir),
	)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Store(i, "value")
	}
}

func BenchmarkBadgerStorageLoad(b *testing.B) {
	dir := os.TempDir() + "/badger_benchmark_load"
	defer os.RemoveAll(dir)

	store := storage.NewMightyMapBadgerStorage[int, string](
		storage.WithMemoryStorage(false),
		storage.WithTempDir(dir),
	)
	// Pre-populate the store
	for i := 0; i < 100000; i++ {
		store.Store(i, "value")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Load(i % 100000)
	}
}

func BenchmarkBadgerStorageLoadWithSync(b *testing.B) {
	dir := os.TempDir() + "/badger_benchmark_load_sync"
	defer os.RemoveAll(dir)

	store := storage.NewMightyMapBadgerStorage[int, string](
		storage.WithMemoryStorage(false),
		storage.WithTempDir(dir),
		storage.WithSyncWrites(true),
	)

	b.ResetTimer()
	// Pre-populate the store
	for i := 0; i < b.N; i++ {
		store.Store(i, "value")
	}
}

func BenchmarkBadgerStorageLoadNoSync(b *testing.B) {
	dir := os.TempDir() + "/badger_benchmark_load_nosync"
	defer os.RemoveAll(dir)

	store := storage.NewMightyMapBadgerStorage[int, string](
		storage.WithMemoryStorage(false),
		storage.WithTempDir(dir),
	)

	b.ResetTimer()
	// Pre-populate the store
	for i := 0; i < b.N; i++ {
		store.Store(i, "value")
	}
}

func BenchmarkBadgerStorageDelete(b *testing.B) {
	dir := os.TempDir() + "/badger_benchmark_delete"
	defer os.RemoveAll(dir)

	store := storage.NewMightyMapBadgerStorage[int, string](
		storage.WithMemoryStorage(false),
		storage.WithTempDir(dir),
	)
	// Pre-populate the store
	for i := 0; i < 100000; i++ {
		store.Store(i, "value")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Delete(i % 100000)
	}
}
