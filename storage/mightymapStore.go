package storage

import (
	"context"
	"sync"
)

type IMightyMapStorage[K comparable, V any] interface {
	Load(ctx context.Context, key K) (value V, ok bool)
	Store(ctx context.Context, key K, value V)
	Delete(ctx context.Context, keys ...K)
	Range(ctx context.Context, f func(key K, value V) bool)
	Next(ctx context.Context) (key K, value V, ok bool)
	Len(ctx context.Context) int
	Clear(ctx context.Context)
	Close(ctx context.Context) error
}

type mightyMapDefaultStorage[K comparable, V any] struct {
	data  map[K]V
	mutex *sync.RWMutex
}

func NewMightyMapDefaultStorage[K comparable, V any]() IMightyMapStorage[K, V] {
	return &mightyMapDefaultStorage[K, V]{
		data:  make(map[K]V),
		mutex: &sync.RWMutex{},
	}
}

func (c *mightyMapDefaultStorage[K, V]) Load(_ context.Context, key K) (value V, ok bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	value, ok = c.data[key]
	return
}

func (c *mightyMapDefaultStorage[K, V]) Store(_ context.Context, key K, value V) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.data[key] = value
}

func (c *mightyMapDefaultStorage[K, V]) Delete(_ context.Context, keys ...K) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for _, key := range keys {
		delete(c.data, key)
	}
}

func (c *mightyMapDefaultStorage[K, V]) Range(_ context.Context, f func(key K, value V) bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	for k, v := range c.data {
		if !f(k, v) {
			break
		}
	}
}

func (c *mightyMapDefaultStorage[K, V]) Len(_ context.Context) int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return len(c.data)
}

func (c *mightyMapDefaultStorage[K, V]) Clear(_ context.Context) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.data = make(map[K]V)
}

func (c *mightyMapDefaultStorage[K, V]) Next(ctx context.Context) (key K, value V, ok bool) {
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

func (c *mightyMapDefaultStorage[K, V]) Close(_ context.Context) error {
	// nothing to do
	return nil
}
