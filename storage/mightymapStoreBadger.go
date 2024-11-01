package storage

import (
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/badger/v4/options"
	msgpack "github.com/vmihailenco/msgpack/v5"
)

type mightyMapBadgerStorage[K comparable, V any] struct {
	db  *badger.DB
	len atomic.Int64
}

// OptionFuncBadger is a function type that modifies badgerOpts configuration.
// It allows customizing the behavior of the BadgerDB storage implementation
// through functional options pattern. WithXXX...
type OptionFuncBadger func(*badgerOpts)

// NewMightyMapBadgerStorage creates a new thread-safe storage implementation using BadgerDB.
// It accepts optional configuration through OptionFuncBadger functions to customize the BadgerDB instance.
//
// Parameters:
//   - optfuncs: Optional configuration functions that modify badgerOpts settings
//
// The function:
//  1. Starts with default options and applies any provided option functions
//  2. Configures BadgerDB options including compression, logging level, and performance settings
//  3. Opens a BadgerDB instance with the configured options
//  4. Starts a background goroutine for value log garbage collection
//
// Returns:
//   - IMightyMapStorage[K, V]: A new BadgerDB-backed storage implementation
//
// Panics if BadgerDB fails to open with the provided configuration.
func NewMightyMapBadgerStorage[K comparable, V any](optfuncs ...OptionFuncBadger) IMightyMapStorage[K, V] {
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
		WithMemTableSize(opts.memTableSize).
		WithBlockCacheSize(opts.blockCacheSize).
		WithValueThreshold(opts.valueThreshold).
		WithSyncWrites(opts.syncWrites)

	if opts.encryptionKey != "" {
		badgerOpts = badgerOpts.
			WithEncryptionKey([]byte(opts.encryptionKey)).
			WithEncryptionKeyRotationDuration(opts.encryptionKeyRotation)
	}

	db, err := badger.Open(badgerOpts)
	if err != nil {
		panic(err)
	}

	// start a goroutine to run value log GC, sensible defaults according to the docs
	go func() {
		ticker := time.NewTicker(opts.gcInterval)
		defer ticker.Stop()
		for range ticker.C {
			_ = db.RunValueLogGC(opts.gcPercentage)
		}
	}()

	return &mightyMapBadgerStorage[K, V]{
		db:  db,
		len: atomic.Int64{},
	}
}

// Store adds a key-value pair to the Badger storage.
func (c *mightyMapBadgerStorage[K, V]) Store(key K, value V) {
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

func (c *mightyMapBadgerStorage[K, V]) Load(key K) (value V, ok bool) {
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

func (c *mightyMapBadgerStorage[K, V]) Delete(keys ...K) {
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

func (c *mightyMapBadgerStorage[K, V]) Range(f func(key K, value V) bool) {
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

func (c *mightyMapBadgerStorage[K, V]) Len() int {
	return int(c.len.Load())
}

func (c *mightyMapBadgerStorage[K, V]) Clear() {
	err := c.db.DropAll()
	if err != nil {
		panic(err)
	}
	c.len.Store(0)
}

func (c *mightyMapBadgerStorage[K, V]) Next() (key K, value V, ok bool) {
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
