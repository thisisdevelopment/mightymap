package mightymap_test

import (
	"testing"

	"github.com/thisisdevelopment/mightymap/storage"
)

// FuzzOperationsDefault holds the default storage to the invariant in
// fuzz_operations_test.go: after any sequence of operations it answers like a
// plain Go map.
func FuzzOperationsDefault(f *testing.F) {
	fuzzOperations(f, storage.NewMightyMapDefaultStorage[string, string]())
}
