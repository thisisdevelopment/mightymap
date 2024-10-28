# MightyMap

MightyMap is a powerful, thread-safe, and concurrent map implementation for Go with configurable storage backends. It provides flexibility in choosing different storage engines depending on your performance and feature requirements.

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
    cm := mightymap.NewConcurrentMap[int, string](true)

    // Store a value
    cm.Store(1, "one")

    // Load a value
    value, ok := cm.Load(1)
    if ok {
        fmt.Println("Loaded value:", value)
    }

    // Check if a key exists
    exists := cm.Has(2)
    fmt.Println("Key 2 exists:", exists)

    // Delete a key
    cm.Delete(1)

    // Get map length
    fmt.Println("Map length:", cm.Len())

    // Clear the map
    cm.Clear()
}
```

## Storage Backends

MightyMap allows you to choose different storage backends. Here's how to use them:

### Default Storage

Uses the standard Go map with mutex locking.

```go
store := storage.NewConcurrentMapDefaultStorage[int, string]()
cm := mightymap.NewConcurrentMap[int, string](true, store)
-- or just --
cm := mightymap.NewConcurrentMap[int, string](true)
```

### Swiss Storage

Swiss Storage leverages the [dolthub/swiss](https://github.com/dolthub/swiss) package, which implements Swiss-table hash maps in Go for high-performance in-memory storage. Swiss-table hash maps offer faster lookups and lower memory usage compared to Go's native maps, especially beneficial in concurrent environments and with large datasets.

By utilizing Swiss Storage, MightyMap achieves optimized performance for workloads that require fast access and modification of in-memory data.

To use Swiss Storage:

```go
store := storage.NewConcurrentMapSwissStorage[int, string]()
cm := mightymap.NewConcurrentMap[int, string](true, store)
```

You can also customize the initial capacity to optimize memory usage for your specific use case:

```go
store := storage.NewConcurrentMapSwissStorage[int, string](storage.WithDefaultCapacity(100_000))
cm := mightymap.NewConcurrentMap[int, string](true, store)
```



An optimized in-memory storage for better performance.

```go
store := storage.NewConcurrentMapSwissStorage[int, string]()
cm := mightymap.NewConcurrentMap[int, string](true, store)
```

### Badger Storage

Uses BadgerDB for persistent storage.

BadgerDB is an embeddable, persistent, and fast key-value database written in pure Go. It provides efficient storage for key-value data, supporting ACID transactions and achieving high performance on SSDs. Using BadgerDB as a storage backend allows MightyMap to handle large datasets that don't fit entirely in memory, with persistence across application restarts.

For more information about BadgerDB, visit the [BadgerDB GitHub repository](https://github.com/dgraph-io/badger).



```go
store := storage.NewConcurrentMapBadgerStorage[int, string](
    storage.WithMemoryStorage(false),
    storage.WithTempDir("/path/to/db"),
)
cm := mightymap.NewConcurrentMap[int, string](true, store)
```

## API Reference

### Methods

- `Load(key K) (value V, ok bool)`: Retrieves the value for a key.
- `Has(key K) (ok bool)`: Checks if a key exists.
- `Store(key K, value V)`: Stores a value for a key.
- `Delete(keys ...K)`: Deletes one or more keys.
- `Range(f func(key K, value V) bool)`: Iterates over all key-value pairs.
- `Pop(key K) (value V, ok bool)`: Retrieves and deletes a value for a key.
- `Next() (value V, key K, ok bool)`: Retrieves the next key-value pair.
- `Len() int`: Returns the number of items in the map.
- `Clear()`: Removes all items from the map.

### Constructor

- `NewConcurrentMap[K comparable, V any](allowOverwrite bool, storages ...storage.IConcurrentMapStorage[K, V]) *ConcurrentMap[K, V]`

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

## StorageAPI Reference

### Methods
```
type IConcurrentMapStorage[K comparable, V any] interface {
	Load(key K) (value V, ok bool)
	Store(key K, value V)
	Delete(keys ...K)
	Range(f func(key K, value V) bool)
	Next() (key K, value V, ok bool)
	Len() int
	Clear()
}
```


## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## License

This project is licensed under the MIT License.
