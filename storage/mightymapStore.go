package storage

import (
	"sync"
)

type IConcurrentMapStorage[K comparable, V any] interface {
	Load(key K) (value V, ok bool)
	Store(key K, value V)
	Delete(keys ...K)
	Range(f func(key K, value V) bool)
	Next() (key K, value V, ok bool)
	Len() int
	Clear()
}

type concurrentMapDefaultStorage[K comparable, V any] struct {
	data  map[K]V
	mutex *sync.RWMutex
}

func NewConcurrentMapDefaultStorage[K comparable, V any]() IConcurrentMapStorage[K, V] {
	return &concurrentMapDefaultStorage[K, V]{
		data:  make(map[K]V),
		mutex: &sync.RWMutex{},
	}
}

func (c *concurrentMapDefaultStorage[K, V]) Load(key K) (value V, ok bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	value, ok = c.data[key]
	return
}

func (c *concurrentMapDefaultStorage[K, V]) Store(key K, value V) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.data[key] = value
}

func (c *concurrentMapDefaultStorage[K, V]) Delete(keys ...K) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for _, key := range keys {
		delete(c.data, key)
	}
}

func (c *concurrentMapDefaultStorage[K, V]) Range(f func(key K, value V) bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	for k, v := range c.data {
		if !f(k, v) {
			break
		}
	}
}

func (c *concurrentMapDefaultStorage[K, V]) Len() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return len(c.data)
}

func (c *concurrentMapDefaultStorage[K, V]) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.data = make(map[K]V)
}

func (c *concurrentMapDefaultStorage[K, V]) Next() (key K, value V, ok bool) {
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