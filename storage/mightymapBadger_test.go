package storage

import (
	"context"
	"testing"
	"time"
)

func TestMightyMapBadgerStorageOptions(t *testing.T) {
	// Test with various Badger options
	t.Run("Badger options", func(t *testing.T) {
		store := NewMightyMapBadgerStorage[string, int](
			WithTempDir(""),
			WithCompression(true),
			WithNumCompactors(2),
			WithNumVersionsToKeep(3),
			WithIndexCacheSize(100*1024*1024),
			WithBlockCacheSize(100*1024*1024),
			WithBlockSize(4*1024),
			WithMetricsEnabled(true),
			WithDetectConflicts(true),
			WithGcInterval(5*time.Minute),
			WithGcPercentage(20),
			WithMemTableSize(64*1024*1024),
			WithValueThreshold(32),
			WithEncryptionKey("0123456789abcdef"),
			WithEncryptionKeyRotationDuration(24*time.Hour),
			WithSyncWrites(true),
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
