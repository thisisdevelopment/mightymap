# MightyMap

[![Go Reference](https://pkg.go.dev/badge/github.com/thisisdevelopment/mightymap.svg)](https://pkg.go.dev/github.com/thisisdevelopment/mightymap)
[![go report card](https://goreportcard.com/badge/github.com/thisisdevelopment/mightymap "go report card")](https://goreportcard.com/report/github.com/thisisdevelopment/mightymap)
[![codecov](https://codecov.io/gh/thisisdevelopment/mightymap/graph/badge.svg?token=DMJCBUY273)](https://codecov.io/gh/thisisdevelopment/mightymap)
[![CircleCI](https://dl.circleci.com/status-badge/img/gh/thisisdevelopment/mightymap/tree/master.svg?style=svg)](https://dl.circleci.com/status-badge/redirect/gh/thisisdevelopment/mightymap/tree/master)
[![Release](https://img.shields.io/github/v/tag/thisisdevelopment/mightymap?label=release)](https://github.com/thisisdevelopment/mightymap/tags)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**One typed, concurrent map for Go. You choose where the data lives: a Go map, files on disk, Badger, SQLite or Redis. Changing that is one line.**

```go
products := mightymap.New[string, Product](true)                 // in memory
products := mightymap.New[string, Product](true, segmentStorage) // keys in memory, values on disk
products := mightymap.New[string, Product](true, redisStorage)   // shared between services
```

The code that calls `Store`, `Load`, `Range` and the rest stays the same.

## Why

A cache usually starts as a `map` with a mutex. Then the data outgrows memory, or has to survive a restart, or has to be shared with another service, and every caller gets rewritten against a new client library. MightyMap puts one small, generic API in front of all of those, so that decision can change later without touching the callers.

- **Typed.** `Map[K, V]` with generics: no `interface{}`, no casts at the call site.
- **Safe for concurrent use**, on every storage.
- **Six storages, one interface.** Bring your own by implementing `storage.IMightyMapStorage`.
- **Data larger than memory.** The segment storage keeps only the keys in memory and reads values from disk, with nothing but the standard library.
- **Tested the hard way.** 91% statement coverage, the race detector, and every storage fuzzed against a plain Go map (see [Testing](#testing)).

## Install

```bash
go get github.com/thisisdevelopment/mightymap
```

Requires Go 1.23 or newer.

## Quick start

```go
package main

import (
	"context"
	"fmt"

	"github.com/thisisdevelopment/mightymap"
	"github.com/thisisdevelopment/mightymap/storage"
)

type Product struct {
	ID    string
	Title string
}

func main() {
	ctx := context.Background()

	// In memory. Swap this one line to move the values to disk, SQLite or Redis:
	//   store := storage.NewMightyMapSegmentStorage[string, Product](storage.WithSegmentDir("/var/cache/products"))
	store := storage.NewMightyMapDefaultStorage[string, Product]()

	products := mightymap.New[string, Product](true, store)
	defer products.Close(ctx)

	products.Store(ctx, "5083739", Product{ID: "5083739", Title: "Gloss 500ml"})

	if product, ok := products.Load(ctx, "5083739"); ok {
		fmt.Println(product.Title) // Gloss 500ml
	}
	fmt.Println(products.Len(ctx), products.Has(ctx, "unknown")) // 1 false
}
```

The first argument of `New` decides what `Store` does with a key that already exists: `true` replaces the value, `false` keeps the first one.

## Choosing a storage

| Storage | Values live | Survive a restart | Shared between processes | Memory grows with | Extra dependency | Use it when |
|---|---|---|---|---|---|---|
| **Default** | Go map | no | no | keys and values | none | the data fits in memory |
| **Segment** | files on disk | no | no | keys only | none | the data is larger than memory, written once, read often |
| **Badger** | Badger, on disk or in memory | yes, on disk | no | Badger's caches | [badger](https://github.com/dgraph-io/badger) | the data has to survive a restart |
| **SQLite** | one SQLite file | yes | no | SQLite's cache | [go-sqlite3](https://github.com/mattn/go-sqlite3) (cgo) | you want to open the data with other tools |
| **Redis** | a Redis server | yes | yes | nothing local | [go-redis](https://github.com/redis/go-redis) | several services need the same data |
| **Swiss** | Swiss table in memory | no | no | keys and values | [swiss](https://github.com/dolthub/swiss) | you are on Go 1.23; from Go 1.24 the default storage is as fast |

Badger, SQLite, Redis and Segment encode values with MessagePack. A value stored as a struct, a pointer or a slice comes back as that type, also in a `Map[K, any]`.

### Default storage

```go
cm := mightymap.New[int, string](true)
// the same as
cm := mightymap.New[int, string](true, storage.NewMightyMapDefaultStorage[int, string]())
```

### Segment storage

Keeps only the keys in memory and the values in files on disk. Made for data that is larger than the memory you want to spend on it, is written once, read often and removed in bulk: a cache that loads a new version of its data and then drops the previous one.

```go
store := storage.NewMightyMapSegmentStorage[string, any](
    storage.WithSegmentDir("/path/to/segments"),
    storage.WithSegmentValueCacheSize(512 << 20), // keep 512 MB of hot values decoded in memory
)
cm := mightymap.New[string, any](false, store)
```

How it works:

- `Store` appends the encoded value to the current segment file. An index in memory remembers for every key in which file its value is, where it starts and how long it is.
- `Load` is one read at a known position. Reads do not wait for each other.
- `Delete` takes the key out of the index. A segment file is removed as soon as none of its values is live. Nothing is ever rewritten: no compaction, no background work.
- `Keys` and `Len` are answered from the index without touching the disk.

Options:

```
    WithSegmentDir(dir string)                      // Directory for the segment files; created when missing. Default: a new directory in os.TempDir().
    WithSegmentSize(segmentSize int64)              // Size in bytes at which a segment file is full. Default: 256 << 20 (256 MB).
    WithSegmentValueCacheSize(valueCacheSize int64) // Keep up to this many bytes of recently loaded values in memory, decoded. Default: 0 (off).
```

Know before you pick it:

- **Memory** grows with the number of keys, not the size of the values: roughly 100 to 150 bytes per key for string keys of about 40 characters.
- **Not persistent.** The index lives in memory only. Segment files an earlier run left behind are removed when the storage opens, and `Close` removes the files of the current run. Use Badger when the data has to survive a restart.
- **One storage per directory.** Only files named `mightymap-*.seg` are ever removed.
- **Disk space comes back per file.** Values stored together and deleted together free their files at once. A value deleted on its own keeps its bytes on disk until every other value in the same file is gone too.
- **The value cache** hands the same decoded value to every caller: treat loaded values as read-only when it is on.

Measured against the Badger storage with values of 7.6 KB on average, in a container limited to 2.5 GiB of memory and 6 CPUs that holds 7.7 GB of values, so most reads come from disk:

| | Segment Storage | Badger Storage |
|---|---|---|
| Store 3.8 GB (510,000 values) | 2.7 - 9.4 s | 27 - 40 s |
| Go memory allocated while storing them | 0.08 GB | 36 - 40 GB |
| Random reads per second, 6 CPUs | 297,000 | 2,300 |
| Read latency p50 / p99 | 6 µs / 0.3 ms | 1.8 ms / 61 ms |
| Delete 510,000 values | 0.2 s | 11.9 s |
| Disk used after that delete, 3.9 GB live | 3.9 GB | 9.9 GB |

Raw bytes; decoding a value into a struct comes on top and is the same for both.

### Badger storage

[BadgerDB](https://github.com/dgraph-io/badger) is an embeddable, persistent key-value database in pure Go. Use it when the data has to be there after a restart.

```go
store := storage.NewMightyMapBadgerStorage[int, string](
    storage.WithMemoryStorage(false),
    storage.WithTempDir("/path/to/db"),
)
cm := mightymap.New[int, string](true, store)
```

`storage.WithValueCacheSize(bytes)` keeps recently loaded values decoded in memory, like the value cache of the segment storage.

For the BadgerDB backend, you can customize the options to optimize performance and storage behavior.

```
    WithNumCompactors(numCompactors int).         // Set the number of compaction workers.
    WithMetricsEnabled(metricsEnabled bool).      // Enable or disable metrics collection.
    WithDetectConflicts(detectConflicts bool).    // Enable or disable conflict detection.
    WithLoggingLevel(loggingLevel badger.LoggerLevel). // Set the logging level.
    WithBlockSize(blockSize int).                 // Set the size of each block in bytes.
    WithNumVersionsToKeep(numVersionsToKeep int). // Set the number of versions to keep per key.
    WithIndexCacheSize(indexCacheSize int64).     // Set the size of the index cache in bytes.
    WithMemoryStorage(memoryStorage bool).        // Use in-memory storage if true.
    WithBlockCacheSize(blockCacheSize int64)      // Set the size of the block cache in bytes.
    WithCompression(compression bool)             // Enable or disable data compression using ZSTD.
    WithValueThreshold(valueThreshold int64)      // Set the threshold for value storage in bytes.
    WithEncryptionKey(encryptionKey string)       // Set the encryption key for the Badger database.
    WithEncryptionKeyRotationDuration(encryptionKeyRotation time.Duration) // Set the rotation duration for the encryption key in Badger.
    WithSyncWrites(syncWrites bool)               // Enable or disable synchronous writes in Badger.
```

sensible defaults are used if you don't specify options.

```	
	return &badgerOpts{
		dir:                   os.TempDir() + fmt.Sprintf("/badger-%d", time.Now().UnixNano()),
		compression:           false,
		memoryStorage:         true,
		numCompactors:         4,
		numVersionsToKeep:     1,
		indexCacheSize:        int64(128 << 20),
		blockCacheSize:        512 << 20,
		blockSize:             16 * 1024,
		loggingLevel:          int(badger.ERROR),
		metricsEnabled:        true,
		detectConflicts:       true,
		gcInterval:            10 * time.Second,
		gcPercentage:          0.5,
		memTableSize:          64 << 20,
		valueThreshold:        1 << 20,
		encryptionKey:         "",
		encryptionKeyRotation: 10 * 24 * time.Hour, // 10 days default
		syncWrites:            false,
	}
```

### SQLite storage

One file you can also open with the `sqlite3` command line tool. Needs cgo.

```go
store := storage.NewMightyMapSQLiteStorage[string, any](
    storage.WithSQLiteDBPath("/path/to/data.db"),
    storage.WithSQLiteJournalMode("WAL"),
)
cm := mightymap.New[string, any](true, store)
```

Options: `WithSQLiteDBPath`, `WithSQLiteInMemory`, `WithSQLiteTableName`, `WithSQLiteJournalMode`, `WithSQLiteSyncMode`, `WithSQLiteCountCacheDuration`, `WithSQLiteMaxOpenConns`, `WithSQLiteMaxIdleConns`, `WithSQLitePragma`.

### Redis storage

The map lives in a Redis server, so several processes can share it.

```go
store := storage.NewMightyMapRedisStorage[string, any](
    storage.WithRedisAddr("localhost:6379"),
    storage.WithRedisUsername("myuser"),
    storage.WithRedisPassword("mypassword"),
    storage.WithRedisPrefix("products:"),
)
cm := mightymap.New[string, any](true, store)
```

Options: `WithRedisAddr`, `WithRedisUsername`, `WithRedisPassword`, `WithRedisDB`, `WithRedisPrefix`, `WithRedisExpire`, `WithRedisTimeout`, `WithRedisPoolSize`, `WithRedisMaxRetries`, `WithRedisTLS`, `WithRedisTLSConfig`.

### Swiss storage

Uses [dolthub/swiss](https://github.com/dolthub/swiss). Since Go 1.24 the built-in map is a Swiss table too, so on Go 1.24 and newer the default storage does the same job.

```go
store := storage.NewMightyMapSwissStorage[int, string](storage.WithDefaultCapacity(100_000))
cm := mightymap.New[int, string](true, store)
```

### Your own storage

Implement `storage.IMightyMapStorage[K, V]` (`Load`, `Store`, `Delete`, `Range`, `Keys`, `Next`, `Len`, `Clear`, `Close`) and pass it to `mightymap.New`.

## Values of your own types in a `Map[K, any]`

The storages that encode values write the name of the type next to the value, so a `Product` stored in a `Map[string, any]` comes back as a `Product`, and a `*Product` as a `*Product`. Types are learned when they are stored. After a restart, tell MightyMap about the types that are already in a persistent storage before you load them:

```go
mightymap.RegisterType(Product{})
```

A typed map such as `Map[string, Product]` never needs this.

## API

Every method takes a `context.Context` first, which is passed on to the storage.

- `Load(ctx, key K) (value V, ok bool)`: Retrieves the value for a key.
- `Has(ctx, key K) (ok bool)`: Checks if a key exists.
- `Store(ctx, key K, value V)`: Stores a value for a key.
- `Delete(ctx, keys ...K)`: Deletes one or more keys.
- `Range(ctx, f func(key K, value V) bool)`: Iterates over all key-value pairs.
- `Keys(ctx) []K`: Returns all keys in the map in an unspecified order.
- `Pop(ctx, key K) (value V, ok bool)`: Retrieves and deletes a value for a key.
- `Next(ctx) (value V, key K, ok bool)`: Retrieves the next key-value pair.
- `Len(ctx) int`: Returns the number of items in the map.
- `Clear(ctx)`: Removes all items from the map.
- `Close(ctx) error`: Closes the map.

### Constructor

- `New[K comparable, V any](allowOverwrite bool, storages ...storage.IMightyMapStorage[K, V]) *Map[K, V]`

    - `allowOverwrite`: If `true`, existing keys can be overwritten when using `Store()`. If `false`, `Store()` will only insert new keys.
    - `storages`: Optional storage implementation.

Full reference with examples: [pkg.go.dev](https://pkg.go.dev/github.com/thisisdevelopment/mightymap).

## Benchmarks

```bash
>> go test -benchmem -bench=. ./storage
goos: darwin
goarch: arm64
pkg: github.com/thisisdevelopment/mightymap/storage
cpu: Apple M2 Max
BenchmarkBadgerMemStorageStore-12         260845              4056 ns/op            1258 B/op         31 allocs/op
BenchmarkBadgerMemStorageLoad-12         1000000              1001 ns/op             632 B/op         17 allocs/op
BenchmarkBadgerMemStorageDelete-12        665391              1684 ns/op             722 B/op         18 allocs/op
BenchmarkBadgerStorageStore-12            231421              6041 ns/op            1518 B/op         41 allocs/op
BenchmarkBadgerStorageLoad-12            1000000              1098 ns/op             632 B/op         18 allocs/op
BenchmarkBadgerStorageDelete-12           547647              1970 ns/op             810 B/op         21 allocs/op
BenchmarkSegmentStorageStore-12           432920              2502 ns/op             225 B/op          3 allocs/op
BenchmarkSegmentStorageLoad-12           1873381               632.0 ns/op           152 B/op          9 allocs/op
BenchmarkSegmentStorageDelete-12        32737390                31.51 ns/op            8 B/op          1 allocs/op
BenchmarkSwissStorageStore-12           14271470               174.7 ns/op           102 B/op          0 allocs/op
BenchmarkSwissStorageLoad-12            21759957                52.01 ns/op            0 B/op          0 allocs/op
BenchmarkSwissStorageDelete-12          30795424                35.53 ns/op            8 B/op          1 allocs/op
BenchmarkDefaultStorageStore-12         11982510               135.4 ns/op            84 B/op          0 allocs/op
BenchmarkDefaultStorageLoad-12          24373977                45.56 ns/op            0 B/op          0 allocs/op
BenchmarkDefaultStorageDelete-12        33667726                31.96 ns/op            8 B/op          1 allocs/op
PASS
ok      github.com/thisisdevelopment/mightymap/storage  35.461s
```

The Badger and Segment numbers include encoding and decoding.

## Testing

```bash
./run_tests.sh -v                    # unit tests, race detector, every storage, coverage
./fuzz.sh                            # fuzz every target, 30 seconds each
FUZZ_TIME=5m ./fuzz.sh Operations    # only the targets with Operations in their name, longer
```

- **Coverage:** 91% of statements, 100% of the `mightymap` package.
- **Race detector** on the map and on the storages.
- **Fuzzing against a plain Go map.** Any sequence of `Store`, `Load`, `Has`, `Delete`, `Pop`, `Next`, `Len`, `Keys`, `Range` and `Clear`, with any keys and values (empty, NUL bytes, invalid UTF-8, text that means something to SQL or to a Redis key pattern), has to give the same answers on every storage as the map does.
- **Fuzzing the decoder.** Arbitrary bytes must be refused without a panic and without allocating for lengths the data does not hold; a value must come back from encoding as the type and value it went in as.
- A failing fuzz input is saved under `testdata/fuzz/` and committed with its fix, so every test run replays it.

## Limitations

- **No errors.** The methods do not return errors. A value that cannot be written (for example a full disk) is a panic; a value that cannot be read or decoded is reported as not found.
- **`Range` on the default and Swiss storage holds a lock.** Calling `Delete` from inside the function you pass to `Range` deadlocks there. Collect the keys and delete them afterwards:

```go
// Collect keys to delete during iteration
keysToDelete := []K{}

cm.Range(ctx, func(key K, value V) bool {
    // Replace this condition with your own logic
    if shouldDelete(key, value) {
        keysToDelete = append(keysToDelete, key)
    }
    return true // Continue iteration
})

// Delete all collected keys after iteration
cm.Delete(ctx, keysToDelete...)
```

- **The segment storage is not persistent**, see [Segment storage](#segment-storage).
- **SQLite needs cgo.**

## Versions and upgrading

Changes per version, including what breaks, are in [CHANGELOG.md](CHANGELOG.md). Data written by v0.4.5 or older with Badger or Redis has to be migrated once; the tools for that are in [`cmd/`](cmd/README.md).

## About Us Th[is]

[This](https://this.nl) is a digital agency based in Utrecht, the Netherlands, specializing in crafting high-performance, resilient, and scalable digital solutions, api's, microservices, and more. Our multidisciplinary team of designers, front and backend developers and strategists collaborates closely to deliver robust and efficient products that meet the demands of today's digital landscape. We are passionate about turning ideas into reality and providing exceptional value to our clients through innovative technology and exceptional user experiences.

## Contributing

Contributions are welcome! We especially encourage contributions of new storage backends. Please open an issue to discuss your ideas or submit a pull request with your implementation.

## License

This project is licensed under the MIT License.
