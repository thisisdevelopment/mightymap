package mightymap

import (
	"sync"

	"github.com/thisisdevelopment/mightymap/storage"
)

type IConcurrentMap[K comparable, V any] interface {
	Load(key K) (value V, ok bool)
	Has(key K) (ok bool)
	Store(key K, value V)
	Delete(keys ...K)
	Range(f func(key K, value V) bool)
	Pop(key K) (value V, ok bool)
	Next() (value V, key K, ok bool)
	Len() int
	Clear()
}

// ConcurrentMap provides a thread-safe map implementation with configurable storage backend.
// Type parameters:
//   - K: the key type, must be comparable
//   - V: the value type, can be any type
type ConcurrentMap[K comparable, V any] struct {
	storage        storage.IConcurrentMapStorage[K, V]
	mutex          sync.Mutex
	allowOverwrite bool
}

// NewConcurrentMap creates a new concurrent map instance.
// Parameters:
//   - allowOverwrite: if true, existing keys can be overwritten with Store(); if false, Store() will only insert new keys
//   - storages: optional storage implementation; if none provided, uses default storage (map[K]V with mutex locking)
//
// Returns a new ConcurrentMap instance with the specified storage engine (or default) and overwrite behavior
func NewConcurrentMap[K comparable, V any](allowOverwrite bool, storages ...storage.IConcurrentMapStorage[K, V]) *ConcurrentMap[K, V] {
	var store storage.IConcurrentMapStorage[K, V]
	if len(storages) == 0 {
		store = storage.NewConcurrentMapDefaultStorage[K, V]()
	} else {
		store = storages[0]
	}

	return &ConcurrentMap[K, V]{
		storage:        store,
		mutex:          sync.Mutex{},
		allowOverwrite: allowOverwrite,
	}
}

// Load retrieves a value from the map for the given key.
// Returns the value and true if found, zero value and false if not present.
func (cm *ConcurrentMap[K, V]) Load(key K) (value V, ok bool) {
	return cm.storage.Load(key)
}

// Has checks if a key exists in the map.
// Returns true if the key exists, false otherwise.
func (cm *ConcurrentMap[K, V]) Has(key K) (ok bool) {
	_, ok = cm.storage.Load(key)
	return
}

// Store inserts or updates a value in the map for the given key.
// If allowOverwrite is false, it will only insert if the key doesn't exist.
func (cm *ConcurrentMap[K, V]) Store(key K, value V) {
	if _, ok := cm.storage.Load(key); !ok || cm.allowOverwrite {
		cm.storage.Store(key, value)
	}
}

// Delete removes one or more keys and their associated values from the map.
func (cm *ConcurrentMap[K, V]) Delete(keys ...K) {
	cm.storage.Delete(keys...)
}

// Range iterates over the map's key-value pairs in an unspecified order,
// calling the provided function for each pair.
// If the function returns false, iteration stops.
func (cm *ConcurrentMap[K, V]) Range(f func(key K, value V) bool) {
	cm.storage.Range(f)
}

// Pop retrieves and removes a value from the map.
// Returns the value and true if found, zero value and false if not present.
func (cm *ConcurrentMap[K, V]) Pop(key K) (value V, ok bool) {
	value, ok = cm.storage.Load(key)
	if !ok {
		return value, ok
	}
	cm.storage.Delete(key)
	return value, true
}

// Next returns the next key-value pair from the map.
// The iteration order is not specified.
// Returns zero values and false when there are no more items.
func (cm *ConcurrentMap[K, V]) Next() (value V, key K, ok bool) {
	key, value, ok = cm.storage.Next()
	return
}

// Len returns the number of key-value pairs in the map.
func (cm *ConcurrentMap[K, V]) Len() int {
	return cm.storage.Len()
}

// Clear removes all key-value pairs from the map.
func (cm *ConcurrentMap[K, V]) Clear() {
	cm.storage.Clear()
}
