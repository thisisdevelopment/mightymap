package mightymap_test

import (
	"testing"

	"github.com/thisisdevelopment/mightymap/storage"
)

// FuzzOperationsBadger holds the Badger storage to the invariant in
// fuzz_operations_test.go: after any sequence of operations it answers like a
// plain Go map.
// It runs in memory, with a value cache so small that values are dropped from
// it all the time: the cache must never answer with a value that is gone.
func FuzzOperationsBadger(f *testing.F) {
	fuzzOperations(f, storage.NewMightyMapBadgerStorage[string, string](storage.WithValueCacheSize(512)))
}
