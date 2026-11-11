package storage

import (
	"container/list"
	"sync"
	"sync/atomic"
)

// valueCache keeps recently loaded values in memory in their decoded form, so
// that reading a hot value again does not go to the storage and decode it once
// more. It holds at most maxBytes, counted by the encoded size of the values,
// and drops the least recently used ones first.
//
// Values are only added when they are loaded. Everything that changes the
// storage drops the affected values, so the cache never answers with a value
// that is no longer stored.
//
// A nil *valueCache is a valid, disabled cache.
type valueCache[K comparable, V any] struct {
	mutex    sync.Mutex
	maxBytes int64
	bytes    int64
	order    *list.List // most recently used in front
	entries  map[K]*list.Element

	// writes counts the changes to the storage. A Load remembers it before it
	// reads; when it moved in the meantime the loaded value may be outdated
	// already and is not cached.
	writes atomic.Uint64
}

type valueCacheEntry[K comparable, V any] struct {
	key   K
	value V
	size  int64
}

func newValueCache[K comparable, V any](maxBytes int64) *valueCache[K, V] {
	if maxBytes <= 0 {
		return nil
	}
	return &valueCache[K, V]{
		maxBytes: maxBytes,
		order:    list.New(),
		entries:  make(map[K]*list.Element),
	}
}

// get returns the cached value for key and marks it as recently used.
func (c *valueCache[K, V]) get(key K) (value V, ok bool) {
	if c == nil {
		return value, false
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()

	element, ok := c.entries[key]
	if !ok {
		return value, false
	}
	c.order.MoveToFront(element)
	return element.Value.(*valueCacheEntry[K, V]).value, true
}

// writeCount is what a Load passes to add after it has read from the storage.
func (c *valueCache[K, V]) writeCount() uint64 {
	if c == nil {
		return 0
	}
	return c.writes.Load()
}

// add caches a value that was just loaded, unless the storage changed since
// writeCount was read or the value is larger than the whole cache.
func (c *valueCache[K, V]) add(key K, value V, size int64, writeCount uint64) {
	if c == nil || size > c.maxBytes {
		return
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.writes.Load() != writeCount {
		return
	}
	if element, ok := c.entries[key]; ok {
		c.remove(element)
	}
	c.entries[key] = c.order.PushFront(&valueCacheEntry[K, V]{key: key, value: value, size: size})
	c.bytes += size
	for c.bytes > c.maxBytes {
		c.remove(c.order.Back())
	}
}

// drop forgets the given keys. It has to be called after the storage changed.
func (c *valueCache[K, V]) drop(keys ...K) {
	if c == nil {
		return
	}
	c.writes.Add(1)
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for _, key := range keys {
		if element, ok := c.entries[key]; ok {
			c.remove(element)
		}
	}
}

// clear forgets everything. It has to be called after the storage was cleared.
func (c *valueCache[K, V]) clear() {
	if c == nil {
		return
	}
	c.writes.Add(1)
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.order.Init()
	clear(c.entries)
	c.bytes = 0
}

// remove takes an entry out; the caller holds the mutex.
func (c *valueCache[K, V]) remove(element *list.Element) {
	entry := c.order.Remove(element).(*valueCacheEntry[K, V])
	delete(c.entries, entry.key)
	c.bytes -= entry.size
}
