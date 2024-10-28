package storage

import (
	"sync"

	"github.com/dolthub/swiss"
)

type concurrentMapSwissStorage[K comparable, V any] struct {
	data  *swiss.Map[K, V]
	mutex *sync.RWMutex
}

type swissOpts struct {
	defaultCapacity uint32
}

const defaultSwissCapacity = 10_000

type OptionFuncSwiss func(*swissOpts)

func NewConcurrentMapSwissStorage[K comparable, V any](optfuncs ...OptionFuncSwiss) IConcurrentMapStorage[K, V] {
	opts := getDefaultSwissOptions()

	for _, optfunc := range optfuncs {
		optfunc(opts)
	}

	return &concurrentMapSwissStorage[K, V]{
		data:  swiss.NewMap[K, V](opts.defaultCapacity),
		mutex: &sync.RWMutex{},
	}
}

func WithDefaultCapacity(capacity uint32) OptionFuncSwiss {
	return func(o *swissOpts) {
		o.defaultCapacity = capacity
	}
}

func (c *concurrentMapSwissStorage[K, V]) Load(key K) (value V, ok bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	value, ok = c.data.Get(key)
	return
}

func (c *concurrentMapSwissStorage[K, V]) Store(key K, value V) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.data.Put(key, value)
}

func (c *concurrentMapSwissStorage[K, V]) Delete(keys ...K) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for _, key := range keys {
		c.data.Delete(key)
	}
}

func (c *concurrentMapSwissStorage[K, V]) Range(f func(key K, value V) bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	c.data.Iter(func(k K, v V) bool {
		return !f(k, v)
	})
}

func (c *concurrentMapSwissStorage[K, V]) Len() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.data.Count()
}

func (c *concurrentMapSwissStorage[K, V]) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.data.Clear()
}

func (c *concurrentMapSwissStorage[K, V]) Next() (key K, value V, ok bool) {
	c.Range(func(k K, v V) bool {
		value = v
		key = k
		return false
	})

	if key != *new(K) {
		ok = true
		c.Delete(key)
	}

	return
}

func getDefaultSwissOptions() *swissOpts {
	return &swissOpts{
		defaultCapacity: defaultSwissCapacity,
	}
}
