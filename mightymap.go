// Package mightymap provides a thread-safe, generic map implementation with configurable storage backends.
// It offers type-safe operations through Go generics and flexibility in choosing different storage engines
// depending on performance and feature requirements.
//
// The package supports multiple storage backends:
//   - Default in-memory map with mutex locking
//   - Swiss storage for optimized performance using swiss tables
//   - BadgerDB storage for persistent key-value storage
//
// Key features:
//   - Thread-safe operations
//   - Generic support for any comparable key type and any value type
//   - Configurable overwrite behavior
//   - Pluggable storage backends
//   - Comprehensive API including Store, Load, Delete, Range, Pop, and more
//
// Example usage:
//
//	cm := mightymap.New[int, string](true)
//	c.Store(1, "one")
//	value, ok := c.Load(1)
//
// For custom storage backends:
//
//	store := storage.NewMightyMapDefaultStorage[int, string]()
//	cm := mightymap.New[int, string](true, store)
package mightymap

import (
	"github.com/thisisdevelopment/mightymap/storage"
)

// Map provides a thread-safe map implementation with configurable storage backend.
// Type parameters:
//   - K: the key type, must be comparable
//   - V: the value type, can be any type
type Map[K comparable, V any] struct {
	storage        storage.IMightyMapStorage[K, V]
	allowOverwrite bool
}

// New creates a new concurrent map instance.
// Parameters:
//   - allowOverwrite: if true, existing keys can be overwritten with Store(); if false, Store() will only insert new keys
//   - storages: optional storage implementation; if none provided, uses default storage (map[K]V with mutex locking)
//
// Returns a new Map instance with the specified storage engine (or default) and overwrite behavior
func New[K comparable, V any](allowOverwrite bool, storages ...storage.IMightyMapStorage[K, V]) *Map[K, V] {
	var store storage.IMightyMapStorage[K, V]
	if len(storages) == 0 {
		store = storage.NewMightyMapDefaultStorage[K, V]()
	} else {
		store = storages[0]
	}

	return &Map[K, V]{
		storage:        store,
		allowOverwrite: allowOverwrite,
	}
}

// Load retrieves a value from the map for the given key.
// Returns the value and true if found, zero value and false if not present.
func (m *Map[K, V]) Load(key K) (value V, ok bool) {
	return m.storage.Load(key)
}

// Has checks if a key exists in the map.
// Returns true if the key exists, false otherwise.
func (m *Map[K, V]) Has(key K) (ok bool) {
	_, ok = m.storage.Load(key)
	return
}

// Store inserts or updates a value in the map for the given key.
// If allowOverwrite is false, it will only insert if the key doesn't exist.
func (m *Map[K, V]) Store(key K, value V) {
	if _, ok := m.storage.Load(key); !ok || m.allowOverwrite {
		m.storage.Store(key, value)
	}
}

// Delete removes one or more keys and their associated values from the map.
func (m *Map[K, V]) Delete(keys ...K) {
	m.storage.Delete(keys...)
}

// Range iterates over the map's key-value pairs in an unspecified order,
// calling the provided function for each pair.
// If the function returns false, iteration stops.
func (m *Map[K, V]) Range(f func(key K, value V) bool) {
	m.storage.Range(f)
}

// Pop retrieves and removes a value from the map.
// Returns the value and true if found, zero value and false if not present.
func (m *Map[K, V]) Pop(key K) (value V, ok bool) {
	value, ok = m.storage.Load(key)
	if !ok {
		return value, ok
	}
	m.storage.Delete(key)
	return value, true
}

// Next returns the next key-value pair from the map.
// The iteration order is not specified.
// Returns zero values and false when there are no more items.
func (m *Map[K, V]) Next() (value V, key K, ok bool) {
	key, value, ok = m.storage.Next()
	return
}

// Len returns the number of key-value pairs in the map.
func (m *Map[K, V]) Len() int {
	return m.storage.Len()
}

// Clear removes all key-value pairs from the map.
func (m *Map[K, V]) Clear() {
	m.storage.Clear()
}

// Close closes the map
func (m *Map[K, V]) Close() error {
	return m.storage.Close()
}
