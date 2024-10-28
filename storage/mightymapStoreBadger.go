package storage

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/badger/v4/options"
	msgpack "github.com/vmihailenco/msgpack/v5"
)

type badgerOpts struct {
	dir               string
	memoryStorage     bool
	compression       bool
	numCompactors     int
	numVersionsToKeep int
	indexCacheSize    int64
	blockCacheSize    int64
	blockSize         int
	loggingLevel      int
	metricsEnabled    bool
	detectConflicts   bool
}

type concurrentMapBadgerStorage[K comparable, V any] struct {
	db  *badger.DB
	len atomic.Int64
	mtx sync.Mutex
}

type OptionFuncBadger func(*badgerOpts)

func NewConcurrentMapBadgerStorage[K comparable, V any](optfuncs ...OptionFuncBadger) IConcurrentMapStorage[K, V] {
	// default options
	opts := getDefaultBadgerOptions()

	for _, optfunc := range optfuncs {
		optfunc(opts)
	}

	badgerOpts := badger.DefaultOptions("")
	if !opts.memoryStorage {
		badgerOpts = badger.DefaultOptions(opts.dir)
	}
	if opts.compression {
		badgerOpts = badgerOpts.WithCompression(options.ZSTD)
	} else {
		badgerOpts = badgerOpts.WithCompression(options.None)
	}

	// const (
	// 	DEBUG loggingLevel = iota
	// 	INFO
	// 	WARNING
	// 	ERROR
	// )
	// we have to proxy this, badger hides its internal type
	loggingLevel := badger.ERROR
	switch opts.loggingLevel {
	case 0:
		loggingLevel = badger.DEBUG
	case 1:
		loggingLevel = badger.INFO
	case 2:
		loggingLevel = badger.WARNING
	case 3:
		loggingLevel = badger.ERROR
	}

	badgerOpts = badgerOpts.
		WithNumCompactors(opts.numCompactors).
		WithMetricsEnabled(opts.metricsEnabled).
		WithDetectConflicts(false).
		WithLoggingLevel(loggingLevel).
		WithBlockSize(opts.blockSize).
		WithNumVersionsToKeep(opts.numVersionsToKeep).
		WithIndexCacheSize(opts.indexCacheSize).
		WithInMemory(opts.memoryStorage).
		WithBlockCacheSize(opts.blockCacheSize)

	db, err := badger.Open(badgerOpts)
	if err != nil {
		panic(err)
	}

	// start a goroutine to run value log GC, sensible defaults according to the docs
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			_ = db.RunValueLogGC(0.5)
		}
	}()

	return &concurrentMapBadgerStorage[K, V]{
		db:  db,
		len: atomic.Int64{},
		mtx: sync.Mutex{},
	}
}

func getDefaultBadgerOptions() *badgerOpts {
	return &badgerOpts{
		dir:               os.TempDir() + fmt.Sprintf("/badger-%d", time.Now().UnixNano()),
		compression:       false,
		memoryStorage:     true,
		numCompactors:     4,
		numVersionsToKeep: 1,
		indexCacheSize:    int64(128 << 20),
		blockCacheSize:    512 << 20,
		blockSize:         16 * 1024,
		loggingLevel:      int(badger.ERROR),
		metricsEnabled:    true,
		detectConflicts:   true,
	}
}

// WithTempDir sets the directory for storing the Badger database files.
// **Default value**: `os.TempDir() + "/badger-{timestamp}"`
func WithTempDir(dir string) OptionFuncBadger {
	return func(o *badgerOpts) {
		o.dir = dir
	}
}

// WithMemoryStorage enables or disables in-memory storage.
// If set to `true`, the database will be stored in memory.
// **Default value**: `true`
func WithMemoryStorage(memoryStorage bool) OptionFuncBadger {
	return func(o *badgerOpts) {
		o.memoryStorage = memoryStorage
	}
}

// WithCompression enables or disables data compression using ZSTD in Badger.
// **Default value**: `false`
func WithCompression(compression bool) OptionFuncBadger {
	return func(o *badgerOpts) {
		o.compression = compression
	}
}

// WithNumCompactors sets the number of compaction workers in Badger.
// **Default value**: `8`
func WithNumCompactors(numCompactors int) OptionFuncBadger {
	return func(o *badgerOpts) {
		o.numCompactors = numCompactors
	}
}

// WithNumVersionsToKeep specifies the number of versions to keep per key.
// **Default value**: `2`
func WithNumVersionsToKeep(numVersionsToKeep int) OptionFuncBadger {
	return func(o *badgerOpts) {
		o.numVersionsToKeep = numVersionsToKeep
	}
}

// WithIndexCacheSize sets the size of the LSM tree cache in bytes.
// **Default value**: `128 << 20` (128 MB)
func WithIndexCacheSize(indexCacheSize int64) OptionFuncBadger {
	return func(o *badgerOpts) {
		o.indexCacheSize = indexCacheSize
	}
}

// WithBlockCacheSize sets the size of the block cache in bytes.
// **Default value**: `512 << 20` (512 MB)
func WithBlockCacheSize(blockCacheSize int64) OptionFuncBadger {
	return func(o *badgerOpts) {
		o.blockCacheSize = blockCacheSize
	}
}

// WithBlockSize sets the size of each block in the LSM tree in bytes.
// **Default value**: `16 * 1024` (16 KB)
func WithBlockSize(blockSize int) OptionFuncBadger {
	return func(o *badgerOpts) {
		o.blockSize = blockSize
	}
}

// WithLoggingLevel sets the logging level for Badger.
// **Default value**: `int(badger.ERROR)`
// Logging levels:
// - `0`: `DEBUG`
// - `1`: `INFO`
// - `2`: `WARNING`
// - `3`: `ERROR`
func WithLoggingLevel(loggingLevel int) OptionFuncBadger {
	return func(o *badgerOpts) {
		o.loggingLevel = loggingLevel
	}
}

// WithMetricsEnabled enables or disables metrics collection in Badger.
// **Default value**: `false`
func WithMetricsEnabled(metricsEnabled bool) OptionFuncBadger {
	return func(o *badgerOpts) {
		o.metricsEnabled = metricsEnabled
	}
}

// WithDetectConflicts enables or disables conflict detection in Badger.
// **Default value**: `true`
func WithDetectConflicts(detectConflicts bool) OptionFuncBadger {
	return func(o *badgerOpts) {
		o.detectConflicts = detectConflicts
	}
}

// Store adds a key-value pair to the Badger storage.
func (c *concurrentMapBadgerStorage[K, V]) Store(key K, value V) {
	keyBytes, err := msgpack.Marshal(key)
	if err != nil {
		panic(err)
	}

	valueBytes, err := msgpack.Marshal(value)
	if err != nil {
		panic(err)
	}

	err = c.db.Update(func(txn *badger.Txn) error {
		return txn.Set(keyBytes, valueBytes)
	})
	if err != nil {
		panic(err)
	}
	c.len.Add(1)
}

func (c *concurrentMapBadgerStorage[K, V]) Load(key K) (value V, ok bool) {
	keyBytes, err := msgpack.Marshal(key)
	if err != nil {
		panic(err)
	}

	var valCopy []byte

	err = c.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(keyBytes)
		if err != nil {
			return err
		}

		valCopy, err = item.ValueCopy(nil)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return value, false
	}

	err = msgpack.Unmarshal(valCopy, &value)
	if err != nil {
		return value, false
	}

	return value, true
}

func (c *concurrentMapBadgerStorage[K, V]) Delete(keys ...K) {
	for _, key := range keys {
		if _, ok := c.Load(key); !ok {
			continue
		}

		keyBytes, err := msgpack.Marshal(key)
		if err != nil {
			panic(err)
		}

		err = c.db.Update(func(txn *badger.Txn) error {
			return txn.Delete(keyBytes)
		})
		if err != nil {
			panic(err)
		}

		c.len.Add(-1)
	}
}

func (c *concurrentMapBadgerStorage[K, V]) Range(f func(key K, value V) bool) {
	// var key K
	// var value V
	// st := time.Now()
	err := c.db.View(func(txn *badger.Txn) error {
		opts := badger.IteratorOptions{
			PrefetchValues: true,
			// PrefetchSize:   100,
			Reverse:     false,
			AllVersions: false,
		}

		// opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		// st := time.Now()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			kBytes := item.Key()
			var k K
			err := msgpack.Unmarshal(kBytes, &k)
			if err != nil {
				return err
			}

			vBytes, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			var v V
			err = msgpack.Unmarshal(vBytes, &v)
			if err != nil {
				return err
			}

			if !f(k, v) {
				return nil
			}
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
}

func (c *concurrentMapBadgerStorage[K, V]) Len() int {
	return int(c.len.Load())
}

func (c *concurrentMapBadgerStorage[K, V]) Clear() {
	err := c.db.DropAll()
	if err != nil {
		panic(err)
	}
	c.len.Store(0)
}

func (c *concurrentMapBadgerStorage[K, V]) Next() (key K, value V, ok bool) {
	err := c.db.View(func(txn *badger.Txn) error {
		opts := badger.IteratorOptions{
			PrefetchValues: true,
			PrefetchSize:   1,
			Reverse:        false,
			AllVersions:    false,
		}

		it := txn.NewIterator(opts)
		defer it.Close()

		it.Rewind()
		if !it.Valid() {
			return nil
		}

		item := it.Item()
		kBytes := item.Key()
		vBytes, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}

		err = msgpack.Unmarshal(kBytes, &key)
		if err != nil {
			return err
		}

		// var value V
		err = msgpack.Unmarshal(vBytes, &value)
		if err != nil {
			return err
		}

		ok = true
		c.Delete(key)
		return nil
	})
	if err != nil {
		panic(err)
	}
	return key, value, ok
}
