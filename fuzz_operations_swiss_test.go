package mightymap_test

import (
	"testing"

	"github.com/thisisdevelopment/mightymap/storage"
)

// FuzzOperationsSwiss holds the Swiss storage to the invariant in
// fuzz_operations_test.go: after any sequence of operations it answers like a
// plain Go map.
func FuzzOperationsSwiss(f *testing.F) {
	fuzzOperations(f, storage.NewMightyMapSwissStorage[string, string]())
}
