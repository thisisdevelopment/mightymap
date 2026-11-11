# Changelog

## v0.7.1

Security update of two dependencies that come in through Badger.

- `github.com/klauspost/compress` v1.18.0 to v1.20.4: out-of-bounds write and integer overflow in its compression code (GHSA-259r-337f-4rfw, GO-2026-6865). Badger reaches that code, so this one mattered.
- `golang.org/x/net` v0.39.0 to v0.58.0: denial of service in its HTML parser (GHSA-5cv4-jp36-h3mw). MightyMap does not call that code.
- **The minimum Go version is now 1.25** (was 1.23): the fixed versions of both need it.

No change in the API, the behaviour or the stored data.

## v0.7.0

### New: Segment Storage

`storage.NewMightyMapSegmentStorage` is a new storage backend for data that does not fit in memory. It keeps only the keys in memory and the values in files on disk, and uses nothing but the Go standard library.

- New constructor: `NewMightyMapSegmentStorage[K, V](opts ...OptionFuncSegment)`.
- New options: `WithSegmentDir`, `WithSegmentSize`, `WithSegmentValueCacheSize`.
- Values are encoded with MessagePack, like in the Badger, SQLite and Redis storages, so custom types come back as the type they were stored as.
- No compaction: disk space comes back per segment file, as soon as none of the values in it is live.
- The values do not survive a restart. Segment files of an earlier run are removed when the storage opens, and `Close` removes the files of the current run.
- No new dependencies and no changes to the existing storages or to the `IMightyMapStorage` interface.

See the README for usage, options and when to pick it.

### Fixes found by fuzzing

- Badger: `Len` counted a key again every time its value was replaced.
- Default and Swiss storage: `Next` never returned the entry with the zero key (`0`, `""`) and reported an empty storage instead.
- Decoding: a few damaged bytes that claimed an array of four billion elements made the decoder allocate for all of them (a bug in msgpack v5.4.1). Stored bytes are now checked before they are decoded. This costs about 8% per decoded value. Data that is cut off is now always refused.

### Less memory allocated when storing

Storing a value in a storage that encodes with MessagePack (Badger, SQLite, Redis, Segment) used to grow a new buffer for every value, which allocated about 3.6 times the size of the encoded value. Values are now encoded in a reused buffer:

- Badger, SQLite and Redis get a copy of exactly the encoded size: about a third of the allocation per `Store`.
- The Segment Storage writes the buffer to its file without a copy: a `Store` of a 4.5 KB value went from 16.4 KB and 10 allocations to 16 bytes and 1 allocation.

No change in behaviour or in the stored format.

### Values keep their type

- A pointer stored in a `Map[K, any]` came back as an untyped map. It now comes back as the pointer it was, like structs already did. The same goes for pointers to pointers and slices of pointers.
- Every read decoded the value twice. It is decoded once now: reading a struct of 4.5 KB went from 71 to 21 microseconds.
- Data written by earlier versions stays readable.

### Value cache for Badger

- `WithValueCacheSize(bytes)` keeps recently loaded values in memory in decoded form, least recently used out first. Off by default. With it on, treat loaded values as read-only.

### Other fixes

- `WithRedisTLSConfig` ignored the configuration it was given.
- The background job that cleans up Badger's value log kept running after `Close`.
- `run_tests.sh` would have skipped every test whose name starts with `TestF`, `Testu` or `Testz`.

### Fuzzing

- Every storage is fuzzed against a plain Go map, and the decoder against arbitrary bytes. `./fuzz.sh` runs the targets; `./run_tests.sh` replays their saved inputs.

## v0.5.1

- Redis username authentication (Redis 6.0+ ACL) with `WithRedisUsername`.

## v0.5.0

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

## v0.4.0

The `ctx` parameter is now required for all methods. This is a breaking change. Since there are several storage backends, the context has to be passed on to them.

## Note on the Swiss storage

Since Go 1.24 the built-in map is a Swiss table. On Go 1.24 and newer the Swiss storage is obsolete and the default storage does the same job.
