package storage

import (
	"context"
	"log"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/dolthub/swiss"
)

type mightyMapSwissStorage[K comparable] struct {
	data  *swiss.Map[K, []byte]
	mutex *sync.RWMutex
}

type swissOpts struct {
	defaultCapacity uint32
}

const defaultSwissCapacity = 10_000

// OptionFuncSwiss is a function type that modifies swissOpts configuration.
// It allows customizing the behavior of the swiss.Map storage implementation
// through functional options pattern.
type OptionFuncSwiss func(*swissOpts)

// NewMightyMapSwissStorage creates a new thread-safe map storage implementation using swiss.Map
// with optional configuration through OptionFuncSwiss functions.
// Values are automatically encoded using MessagePack encoding.
//
// NOTE: If you're using Go 1.24 or later, consider using the default storage implementation
// instead, as Go 1.24+ already uses SwissMap internally for its map implementation.
func NewMightyMapSwissStorage[K comparable, V any](optfuncs ...OptionFuncSwiss) IMightyMapStorage[K, V] {
	// Check Go version and print warning if using Go 1.24+
	checkGoVersion()

	opts := getDefaultSwissOptions()

	for _, optfunc := range optfuncs {
		optfunc(opts)
	}

	storage := &mightyMapSwissStorage[K]{
		data:  swiss.NewMap[K, []byte](opts.defaultCapacity),
		mutex: &sync.RWMutex{},
	}
	return newMsgpackAdapter[K, V](storage)
}

// checkGoVersion checks if the runtime Go version is 1.24 or higher and logs a warning
// if it is, since Go 1.24+ already uses SwissMap internally for its map implementation.
func checkGoVersion() {
	goVersion := runtime.Version()
	// Parse Go version string (format like "go1.24.1")
	if strings.HasPrefix(goVersion, "go1.") {
		versionParts := strings.Split(strings.TrimPrefix(goVersion, "go1."), ".")
		if len(versionParts) > 0 {
			majorVersion, err := strconv.Atoi(versionParts[0])
			if err == nil && majorVersion >= 24 {
				log.Println("WARNING: You are using SwissMapStorage with Go 1.24+, which is redundant because Go 1.24+ " +
					"already uses SwissMap internally for map implementation. Consider using the default storage implementation instead.")
			}
		}
	}
}

// WithDefaultCapacity returns an OptionFuncSwiss that sets the initial capacity of the swiss.Map.
// The capacity should be set based on the expected number of items to optimize memory usage.
func WithDefaultCapacity(capacity uint32) OptionFuncSwiss {
	return func(o *swissOpts) {
		o.defaultCapacity = capacity
	}
}

func (c *mightyMapSwissStorage[K]) Load(_ context.Context, key K) (value []byte, ok bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	value, ok = c.data.Get(key)
	return
}

func (c *mightyMapSwissStorage[K]) Store(_ context.Context, key K, value []byte) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.data.Put(key, value)
}

func (c *mightyMapSwissStorage[K]) Delete(_ context.Context, keys ...K) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for _, key := range keys {
		c.data.Delete(key)
	}
}

func (c *mightyMapSwissStorage[K]) Range(_ context.Context, f func(key K, value []byte) bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	c.data.Iter(func(k K, v []byte) bool {
		return !f(k, v)
	})
}

func (c *mightyMapSwissStorage[K]) Keys(_ context.Context) []K {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	keys := []K{}
	c.data.Iter(func(k K, v []byte) bool {
		keys = append(keys, k)
		return false // Continue iteration (based on Range method pattern)
	})
	return keys
}

func (c *mightyMapSwissStorage[K]) Len(_ context.Context) int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.data.Count()
}

func (c *mightyMapSwissStorage[K]) Clear(_ context.Context) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.data.Clear()
}

func (c *mightyMapSwissStorage[K]) Next(ctx context.Context) (key K, value []byte, ok bool) {
	c.Range(ctx, func(k K, v []byte) bool {
		value = v
		key = k
		return false
	})

	if key != *new(K) {
		ok = true
		c.Delete(ctx, key)
	}

	return
}

func (c *mightyMapSwissStorage[K]) Close(_ context.Context) error {
	// nothing to do
	return nil
}

func getDefaultSwissOptions() *swissOpts {
	return &swissOpts{
		defaultCapacity: defaultSwissCapacity,
	}
}
