package mightymap_test

import (
	"testing"

	"github.com/thisisdevelopment/mightymap/storage"
)

// FuzzOperationsSQLite holds the SQLite storage to the invariant in
// fuzz_operations_test.go: after any sequence of operations it answers like a
// plain Go map. It runs in memory.
func FuzzOperationsSQLite(f *testing.F) {
	fuzzOperations(f, storage.NewMightyMapSQLiteStorage[string, string](storage.WithSQLiteInMemory()))
}
