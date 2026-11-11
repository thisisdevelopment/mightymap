package storage

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/badger/v4/options"
	msgpack "github.com/vmihailenco/msgpack/v5"
)

type mightyMapBadgerStorage[K comparable] struct {
	db          *badger.DB
	len         atomic.Int64
	initLenCall atomic.Bool

	// The value log GC runs in the background; Close closes stopGC once and
	// waits for gcStopped.
	stopGC    chan struct{}
	gcStopped chan struct{}
	closeOnce sync.Once
}

// OptionFuncBadger is a function type that modifies badgerOpts configuration.
// It allows customizing the behavior of the BadgerDB storage implementation
// through functional options pattern. WithXXX...
type OptionFuncBadger func(*badgerOpts)

// NewMightyMapBadgerStorage creates a new thread-safe storage implementation using BadgerDB.
// It accepts optional configuration through OptionFuncBadger functions to customize the BadgerDB instance.
// Values are automatically encoded using MessagePack encoding.
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

	// Logging level constants for BadgerDB
	const (
		loggingLevelDebug   = 0
		loggingLevelInfo    = 1
		loggingLevelWarning = 2
		loggingLevelError   = 3
	)

	// we have to proxy this, badger hides its internal type
	loggingLevel := badger.ERROR
	switch opts.loggingLevel {
	case loggingLevelDebug:
		loggingLevel = badger.DEBUG
	case loggingLevelInfo:
		loggingLevel = badger.INFO
	case loggingLevelWarning:
		loggingLevel = badger.WARNING
	case loggingLevelError:
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

	storage := &mightyMapBadgerStorage[K]{
		db:          db,
		len:         atomic.Int64{},
		initLenCall: atomic.Bool{},
		stopGC:      make(chan struct{}),
		gcStopped:   make(chan struct{}),
	}

	// run value log GC in the background until the storage is closed, sensible
	// defaults according to the docs
	go storage.runValueLogGC(opts.gcInterval, opts.gcPercentage)

	adapter := newMsgpackAdapter[K, V](storage)
	adapter.cache = newValueCache[K, V](opts.valueCacheSize)
	return adapter
}

// Store adds a key-value pair to the Badger storage.
func (c *mightyMapBadgerStorage[K]) Store(_ context.Context, key K, value []byte) {
	// Serialize the key with MessagePack
	keyBytes, err := msgpack.Marshal(key)
	if err != nil {
		log.Printf("Error marshalling key: %v", err)
		panic(err)
	}

	// Store in BadgerDB with proper error handling. Only a key that was not
	// there yet adds to the number of keys.
	isNew := false
	err = c.db.Update(func(txn *badger.Txn) error {
		_, getErr := txn.Get(keyBytes)
		isNew = errors.Is(getErr, badger.ErrKeyNotFound)
		return txn.Set(keyBytes, value)
	})
	if err != nil {
		log.Printf("Error storing value: %v", err)
		panic(err)
	}
	if isNew {
		c.len.Add(1)
	}
}

func (c *mightyMapBadgerStorage[K]) Load(_ context.Context, key K) (value []byte, ok bool) {
	// Serialize the key with MessagePack consistently with Store method
	keyBytes, err := msgpack.Marshal(key)
	if err != nil {
		log.Printf("Error marshalling key: %v", err)
		panic(err)
	}
	var valCopy []byte

	// Read from BadgerDB
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
		// Not found or other error
		return nil, false
	}

	return valCopy, true
}

func (c *mightyMapBadgerStorage[K]) Delete(ctx context.Context, keys ...K) {
	for _, key := range keys {
		if _, ok := c.Load(ctx, key); !ok {
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

func (c *mightyMapBadgerStorage[K]) Range(_ context.Context, f func(key K, value []byte) bool) {
	err := c.db.View(func(txn *badger.Txn) error {
		opts := badger.IteratorOptions{
			PrefetchValues: true,
			Reverse:        false,
			AllVersions:    false,
		}

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			kBytes := item.Key()
			var k K
			err := msgpack.Unmarshal(kBytes, &k)
			if err != nil {
				log.Printf("error: unmarshalling key: '%v' err: %v", string(kBytes), err)
				continue
			}

			vBytes, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			if !f(k, vBytes) {
				return nil
			}
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
}

func (c *mightyMapBadgerStorage[K]) Keys(_ context.Context) []K {
	keys := []K{}
	err := c.db.View(func(txn *badger.Txn) error {
		opts := badger.IteratorOptions{
			PrefetchValues: false,
			Reverse:        false,
			AllVersions:    false,
		}

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			kBytes := item.Key()
			var k K
			err := msgpack.Unmarshal(kBytes, &k)
			if err != nil {
				log.Printf("error: unmarshalling key: '%v' err: %v", string(kBytes), err)
				continue
			}

			keys = append(keys, k)
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	return keys
}

func (c *mightyMapBadgerStorage[K]) Len(_ context.Context) int {
	if !c.initLenCall.Load() {
		c.initLenCall.Store(true)
		cnt := 0
		err := c.db.View(func(txn *badger.Txn) error {
			opts := badger.IteratorOptions{
				PrefetchValues: true,
				Reverse:        false,
				AllVersions:    false,
			}
			it := txn.NewIterator(opts)
			defer it.Close()
			for it.Rewind(); it.Valid(); it.Next() {
				cnt++
			}
			return nil
		})
		if err != nil {
			panic(err)
		}
		c.len.Store(int64(cnt))
	}
	return int(c.len.Load())
}

func (c *mightyMapBadgerStorage[K]) Clear(_ context.Context) {
	err := c.db.DropAll()
	if err != nil {
		panic(err)
	}
	c.len.Store(0)
}

func (c *mightyMapBadgerStorage[K]) Next(ctx context.Context) (key K, value []byte, ok bool) {
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

		value = vBytes
		ok = true
		c.Delete(ctx, key)
		return nil
	})
	if err != nil {
		panic(err)
	}
	return key, value, ok
}

func (c *mightyMapBadgerStorage[K]) Close(_ context.Context) error {
	// Stop the value log GC first and wait for it, so it does not run on a
	// closed database and does not keep that database alive.
	c.closeOnce.Do(func() { close(c.stopGC) })
	<-c.gcStopped

	return c.db.Close()
}

// runValueLogGC runs Badger's value log GC every interval until Close.
func (c *mightyMapBadgerStorage[K]) runValueLogGC(interval time.Duration, discardRatio float64) {
	defer close(c.gcStopped)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = c.db.RunValueLogGC(discardRatio)
		case <-c.stopGC:
			return
		}
	}
}
