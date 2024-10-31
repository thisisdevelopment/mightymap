package storage

import (
	"fmt"
	"os"
	"time"

	"github.com/dgraph-io/badger/v4"
)

type badgerOpts struct {
	dir                   string
	memoryStorage         bool
	compression           bool
	numCompactors         int
	numVersionsToKeep     int
	indexCacheSize        int64
	blockCacheSize        int64
	blockSize             int
	loggingLevel          int
	metricsEnabled        bool
	detectConflicts       bool
	gcInterval            time.Duration
	gcPercentage          float64
	memTableSize          int64
	valueThreshold        int64
	encryptionKey         string
	encryptionKeyRotation time.Duration
}

func getDefaultBadgerOptions() *badgerOpts {
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

// WithGcInterval sets the interval for garbage collection in Badger.
// **Default value**: `10 * time.Second`
func WithGcInterval(gcInterval time.Duration) OptionFuncBadger {
	return func(o *badgerOpts) {
		o.gcInterval = gcInterval
	}
}

// WithGcPercentage sets the percentage of value log space to be collected during garbage collection.
// **Default value**: `0.5`
func WithGcPercentage(gcPercentage float64) OptionFuncBadger {
	return func(o *badgerOpts) {
		o.gcPercentage = gcPercentage
	}
}

// WithMemTableSize sets the size of the memtable in bytes.
// **Default value**: `128 << 20` (128 MB)
func WithMemTableSize(memTableSize int64) OptionFuncBadger {
	return func(o *badgerOpts) {
		o.memTableSize = memTableSize
	}
}

// WithValueThreshold sets the threshold for value storage in Badger.
// **Default value**: `4 << 20` (4 MB)
func WithValueThreshold(valueThreshold int64) OptionFuncBadger {
	return func(o *badgerOpts) {
		o.valueThreshold = valueThreshold
	}
}

// WithEncryptionKey sets the encryption key for the Badger database.
func WithEncryptionKey(encryptionKey string) OptionFuncBadger {
	// During OpenKeyRegistry error: Encryption key's length should beeither 16, 24, or 32 bytes
	if len(encryptionKey) != 16 && len(encryptionKey) != 24 && len(encryptionKey) != 32 {
		panic(fmt.Sprintf("Encryption key's length should be either 16, 24, or 32 bytes current length: %d", len(encryptionKey)))
	}

	return func(o *badgerOpts) {
		o.encryptionKey = encryptionKey
	}
}

// WithEncryptionKeyRotation sets the rotation duration for the encryption key in Badger.
func WithEncryptionKeyRotationDuration(encryptionKeyRotation time.Duration) OptionFuncBadger {
	return func(o *badgerOpts) {
		o.encryptionKeyRotation = encryptionKeyRotation
	}
}
