package mightymap_test

import (
	"testing"

	"github.com/thisisdevelopment/mightymap/storage"
)

// FuzzOperationsSegment holds the segment file storage to the invariant in
// fuzz_operations_test.go: after any sequence of operations it answers like a
// plain Go map.
// Segment files are 256 bytes, so files fill up, are removed and replaced
// all the time.
func FuzzOperationsSegment(f *testing.F) {
	fuzzOperations(f, storage.NewMightyMapSegmentStorage[string, string](
		storage.WithSegmentDir(f.TempDir()),
		storage.WithSegmentSize(256),
	))
}
