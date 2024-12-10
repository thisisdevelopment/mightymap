package storage

import (
	"context"
	"sync"

	"github.com/dolthub/swiss"
)

type mightyMapSwissStorage[K comparable, V any] struct {
	data  *swiss.Map[K, V]
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
func NewMightyMapSwissStorage[K comparable, V any](optfuncs ...OptionFuncSwiss) IMightyMapStorage[K, V] {
	opts := getDefaultSwissOptions()

	for _, optfunc := range optfuncs {
		optfunc(opts)
	}

	return &mightyMapSwissStorage[K, V]{
		data:  swiss.NewMap[K, V](opts.defaultCapacity),
		mutex: &sync.RWMutex{},
	}
}

// WithDefaultCapacity returns an OptionFuncSwiss that sets the initial capacity of the swiss.Map.
// The capacity should be set based on the expected number of items to optimize memory usage.
func WithDefaultCapacity(capacity uint32) OptionFuncSwiss {
	return func(o *swissOpts) {
		o.defaultCapacity = capacity
	}
}

func (c *mightyMapSwissStorage[K, V]) Load(_ context.Context, key K) (value V, ok bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	value, ok = c.data.Get(key)
	return
}

func (c *mightyMapSwissStorage[K, V]) Store(_ context.Context, key K, value V) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.data.Put(key, value)
}

func (c *mightyMapSwissStorage[K, V]) Delete(_ context.Context, keys ...K) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for _, key := range keys {
		c.data.Delete(key)
	}
}

func (c *mightyMapSwissStorage[K, V]) Range(_ context.Context, f func(key K, value V) bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	c.data.Iter(func(k K, v V) bool {
		return !f(k, v)
	})
}

func (c *mightyMapSwissStorage[K, V]) Len(_ context.Context) int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.data.Count()
}

func (c *mightyMapSwissStorage[K, V]) Clear(_ context.Context) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.data.Clear()
}

func (c *mightyMapSwissStorage[K, V]) Next(ctx context.Context) (key K, value V, ok bool) {
	c.Range(ctx, func(k K, v V) bool {
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

func (c *mightyMapSwissStorage[K, V]) Close(_ context.Context) error {
	// nothing to do
	return nil
}

func getDefaultSwissOptions() *swissOpts {
	return &swissOpts{
		defaultCapacity: defaultSwissCapacity,
	}
}
