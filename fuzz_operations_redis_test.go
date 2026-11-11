package mightymap_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/thisisdevelopment/mightymap/storage"
)

// FuzzOperationsRedis holds the Redis storage to the invariant in
// fuzz_operations_test.go: after any sequence of operations it answers like a
// plain Go map.
// It talks to an in-process Redis server.
func FuzzOperationsRedis(f *testing.F) {
	server, err := miniredis.Run()
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(server.Close)
	fuzzOperations(f, storage.NewMightyMapRedisStorage[string, string](storage.WithRedisAddr(server.Addr())))
}
