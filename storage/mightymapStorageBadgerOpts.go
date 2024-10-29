package storage

import "time"

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
