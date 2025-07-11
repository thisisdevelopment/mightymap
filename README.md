# MightyMap

[![go report card](https://goreportcard.com/badge/github.com/thisisdevelopment/mightymap "go report card")](https://goreportcard.com/report/github.com/thisisdevelopment/mightymap)
[![codecov](https://codecov.io/gh/thisisdevelopment/mightymap/graph/badge.svg?token=DMJCBUY273)](https://codecov.io/gh/thisisdevelopment/mightymap)
[![CircleCI](https://dl.circleci.com/status-badge/img/gh/thisisdevelopment/mightymap/tree/master.svg?style=svg)](https://dl.circleci.com/status-badge/redirect/gh/thisisdevelopment/mightymap/tree/master)
[![GoDoc](https://godoc.org/github.com/thisisdevelopment/mightymap?status.svg)](https://godoc.org/github.com/thisisdevelopment/mightymap)


MightyMap is a powerful, thread-safe, and concurrent generic map implementation for Go with configurable storage backends. 🔒 It provides type-safe operations through Go generics and flexibility in choosing different storage engines depending on your performance and feature requirements. 🚀 The generic implementation allows you to create strongly-typed maps with any comparable key type and any value type. 💪

## Features

- **Thread-safe**: Designed for concurrent use without the need for additional synchronization.
- **Configurable Storage Backends**: Swap out the storage engine as needed. Currently supports:
  - Default in-memory map with mutex locking.
  - SwissStorage for optimized performance.
  - BadgerStorage for persistent key-value storage.
- **Generic Support**: Works with any comparable key type and any value type.
- **Flexible Overwrite Behavior**: Choose whether keys can be overwritten or not.

## Installation

To install MightyMap, use `go get`:

```bash
go get github.com/thisisdevelopment/mightymap
```

## Note v0.3.1 to v0.4.0
The `ctx` parameter is now required for all methods. This is a breaking change. Since we allow multiple storage backends, we need to be able pass the context to the storage backend.

## Note on swissMap Storage
Since golang 1.24 the default internal map implementation has been switched to a swiss map implementation. If you're using golang version >= 1.24 the SwissMapStorage implementation is obsolete, and you can just use the default storage (for in memory mightyMaps)

## Storage Changes in v0.5.0

In version 0.5.0, we've made significant changes to the underlying storage implementation for persistent backends (BadgerDB and Redis):

### MessagePack Encoding
- **New Encoding**: All persistent storage backends now use MessagePack encoding for improved reliability, type safety, and cross-language compatibility
- **Performance**: MessagePack provides efficient binary serialization with minimal overhead (~2-5% of total operation time)
- **Type Safety**: Better handling of complex data types including structs, maps, slices, and interfaces
- **Schema Evolution**: Future-proof format that supports gradual schema changes

### Backward Compatibility
⚠️ **Breaking Change**: v0.5.0 is **not backward compatible** with previous versions when using BadgerDB or Redis storage backends. Data stored with v0.4.5 and earlier versions uses direct Go encoding, while v0.5.0 uses MessagePack format.

### Redis Username Support
v0.5.1 also adds support for Redis username authentication (Redis 6.0+ ACL), providing enhanced security for Redis deployments:

```go
store := storage.NewMightyMapRedisStorage[string, any](
    storage.WithRedisAddr("localhost:6379"),
    storage.WithRedisUsername("myuser"),     // New in v0.5.0
    storage.WithRedisPassword("mypassword"),
    storage.WithRedisDB(0),
)
```

### Migration Tools
To help with the transition from v0.4.5 to v0.5.0, we provide comprehensive CLI migration tools:

#### Available Tools
- **`cmd/migrate-badger`**: Migrates BadgerDB data from v0.4.5 (direct storage) to v0.5.0 (MessagePack)
- **`cmd/migrate-redis`**: Migrates Redis data from v0.4.5 to v0.5.0 with username support

#### Features
- ✅ **Dry-run mode** to preview changes before migration
- ✅ **Configurable batch processing** with progress logging
- ✅ **Key pattern filtering** for selective migration
- ✅ **Error handling and reporting** with detailed statistics
- ✅ **Timeout controls** and safe operation practices
- ✅ **YAML configuration** with example files provided

#### Quick Start
```bash
# BadgerDB migration
cd cmd/migrate-badger
go run main.go                    # Creates config file
go run main.go --dry-run --verbose   # Preview migration
go run main.go --verbose         # Run actual migration

# Redis migration  
cd cmd/migrate-redis
go run main.go                    # Creates config file
go run main.go --dry-run --verbose   # Preview migration
go run main.go --verbose         # Run actual migration
```

See `cmd/README.md` for detailed setup instructions and configuration options.

### Why MessagePack?
The migration to MessagePack provides several benefits:
- **Reliability**: Consistent encoding/decoding across different Go versions
- **Performance**: Efficient binary format optimized for storage
- **Compatibility**: Cross-language support for polyglot environments  
- **Future-proofing**: Schema evolution capabilities for long-term data integrity

## Usage
Here is a simple example of how to use MightyMap:

```go
package main

import (
    "fmt"
    "github.com/thisisdevelopment/mightymap"
)

func main() {
    // Create a new concurrent map that allows overwriting existing keys
    ctx := context.Background()
    cm := mightymap.New[int, string](true)

    // Store a value
    cm.Store(ctx, 1, "one")

    // Load a value
    value, ok := cm.Load(ctx, 1)
    if ok {
        fmt.Println("Loaded value:", value)
    }

    // Check if a key exists
    exists := cm.Has(ctx, 2)
    fmt.Println("Key 2 exists:", exists)

    // Store more values
    cm.Store(ctx, 2, "two")
    cm.Store(ctx, 3, "three")

    // Get all keys
    keys := cm.Keys(ctx)
    fmt.Println("All keys:", keys)

    // Delete a key
    cm.Delete(ctx, 1)

    // Get map length
    fmt.Println("Map length:", cm.Len(ctx))

    // Clear the map
    cm.Clear()
}
```

## Storage Backends

MightyMap allows you to choose different storage backends. Here's how to use them:

### Default Storage

Uses the standard Go map with mutex locking.

```go
store := storage.NewMightyMapDefaultStorage[int, string]()
cm := mightymap.New[int, string](true, store)
-- or just --
cm := mightymap.New[int, string](true)
```

### Swiss Storage

Swiss Storage leverages the [dolthub/swiss](https://github.com/dolthub/swiss) package, which implements Swiss-table hash maps in Go for high-performance in-memory storage. Swiss-table hash maps offer faster lookups and lower memory usage compared to Go's native maps, especially beneficial in concurrent environments and with large datasets.

By utilizing Swiss Storage, MightyMap achieves optimized performance for workloads that require fast access and modification of in-memory data.

To use Swiss Storage:

```go
store := storage.NewMightyMapSwissStorage[int, string]()
cm := mightymap.New[int, string](true, store)
```

You can also customize the initial capacity to optimize memory usage for your specific use case:

```go
store := storage.NewMightyMapSwissStorage[int, string](storage.WithDefaultCapacity(100_000))
cm := mightymap.New[int, string](true, store)
```

### Badger Storage

Uses BadgerDB for persistent storage.

BadgerDB is an embeddable, persistent, and fast key-value database written in pure Go. It provides efficient storage for key-value data, supporting ACID transactions and achieving high performance on SSDs. Using BadgerDB as a storage backend allows MightyMap to handle large datasets that don't fit entirely in memory, with persistence across application restarts. BadgerDB also supports encryption and compression, making it a versatile choice for various use cases.

For more information about BadgerDB, visit the [BadgerDB GitHub repository](https://github.com/dgraph-io/badger).



```go
store := storage.NewMightyMapBadgerStorage[int, string](
    storage.WithMemoryStorage(false),
    storage.WithTempDir("/path/to/db"),
)
cm := mightymap.New[int, string](true, store)
```

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

## API Reference

### Methods

- `Load(key K) (value V, ok bool)`: Retrieves the value for a key.
- `Has(key K) (ok bool)`: Checks if a key exists.
- `Store(key K, value V)`: Stores a value for a key.
- `Delete(keys ...K)`: Deletes one or more keys.
- `Range(f func(key K, value V) bool)`: Iterates over all key-value pairs.
- `Keys() []K`: Returns all keys in the map in an unspecified order.
- `Pop(key K) (value V, ok bool)`: Retrieves and deletes a value for a key.
- `Next() (value V, key K, ok bool)`: Retrieves the next key-value pair.
- `Len() int`: Returns the number of items in the map.
- `Clear()`: Removes all items from the map.
- `Close() error`: Closes the map.

### Constructor

- `New[K comparable, V any](allowOverwrite bool, storages ...storage.IMightyMapStorage[K, V]) *Map[K, V]`

    - `allowOverwrite`: If `true`, existing keys can be overwritten when using `Store()`. If `false`, `Store()` will only insert new keys.
    - `storages`: Optional storage implementation.

## Benchmarks

Benchmarks are available in the `storage` package to compare the performance of different storage backends. You can run the benchmarks using:

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
BenchmarkSwissStorageStore-12           14271470               174.7 ns/op           102 B/op          0 allocs/op
BenchmarkSwissStorageLoad-12            21759957                52.01 ns/op            0 B/op          0 allocs/op
BenchmarkSwissStorageDelete-12          30795424                35.53 ns/op            8 B/op          1 allocs/op
BenchmarkDefaultStorageStore-12         11982510               135.4 ns/op            84 B/op          0 allocs/op
BenchmarkDefaultStorageLoad-12          24373977                45.56 ns/op            0 B/op          0 allocs/op
BenchmarkDefaultStorageDelete-12        33667726                31.96 ns/op            8 B/op          1 allocs/op
PASS
ok      github.com/thisisdevelopment/mightymap/storage  35.461s
```

## Limitations
- The **default storage** backend and **Swiss backend** are not thread-safe by default. Using Delete() while iterating with Range() will cause a deadlock. To safely delete items while iterating, collect the keys to delete during Range() and call Delete() with all keys after Range() completes. For example:

```go
// Collect keys to delete during iteration
keysToDelete := []K{}

cm.Range(func(key K, value V) bool {
    // Replace this condition with your own logic
    if shouldDelete(key, value) {
        keysToDelete = append(keysToDelete, key)
    }
    return true // Continue iteration
})

// Delete all collected keys after iteration
cm.Delete(keysToDelete...)
```

## StorageAPI Reference

### Methods
```
	Load(ctx context.Context, key K) (value V, ok bool)
	Store(ctx context.Context, key K, value V)
	Delete(ctx context.Context, keys ...K)
	Range(ctx context.Context, f func(key K, value V) bool)
	Keys(ctx context.Context) []K
	Next(ctx context.Context) (key K, value V, ok bool)
	Len(ctx context.Context) int
	Clear(ctx context.Context)
```

## About Us Th[is]

[This](https://this.nl) is a digital agency based in Utrecht, the Netherlands, specializing in crafting high-performance, resilient, and scalable digital solutions, api's, microservices, and more. Our multidisciplinary team of designers, front and backend developers and strategists collaborates closely to deliver robust and efficient products that meet the demands of today's digital landscape. We are passionate about turning ideas into reality and providing exceptional value to our clients through innovative technology and exceptional user experiences.

## Contributing

Contributions are welcome! We especially encourage contributions of new storage backends. Please open an issue to discuss your ideas or submit a pull request with your implementation.

## License

This project is licensed under the MIT License.

