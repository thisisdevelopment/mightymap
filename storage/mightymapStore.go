package storage

import (
	"context"
	"sync"
)

// IMightyMapStorage defines the interface for all storage implementations used by MightyMap.
// This interface provides thread-safe operations for storing, retrieving, and managing key-value pairs.
// All implementations must support concurrent access and provide context-aware operations.
//
// Type parameters:
//   - K: the key type, must be comparable
//   - V: the value type, can be any type
type IMightyMapStorage[K comparable, V any] interface {
	// Load retrieves a value from storage for the given key.
	// Returns the value and true if found, zero value and false if not present.
	Load(ctx context.Context, key K) (value V, ok bool)

	// Store adds or updates a key-value pair in storage.
	// The operation is atomic and thread-safe.
	Store(ctx context.Context, key K, value V)

	// Delete removes one or more keys and their associated values from storage.
	// Non-existent keys are silently ignored.
	Delete(ctx context.Context, keys ...K)

	// Range iterates over all key-value pairs in storage in an unspecified order,
	// calling the provided function for each pair.
	// If the function returns false, iteration stops early.
	Range(ctx context.Context, f func(key K, value V) bool)

	// Next returns and removes the next key-value pair from storage.
	// The iteration order is not specified and may vary between implementations.
	// Returns zero values and false when storage is empty.
	// This operation is atomic - the key-value pair is removed as part of retrieval.
	Next(ctx context.Context) (key K, value V, ok bool)

	// Len returns the current number of key-value pairs in storage.
	Len(ctx context.Context) int

	// Clear removes all key-value pairs from storage.
	// This operation is atomic and thread-safe.
	Clear(ctx context.Context)

	// Close releases any resources held by the storage implementation.
	// After calling Close, the storage should not be used for further operations.
	Close(ctx context.Context) error
}

// byteStorage is an internal interface that works with byte slices for storage implementations
// that require serialization. This interface is used by adapters that handle encoding/decoding
// of values to/from byte arrays for persistent or optimized storage backends.
//
// Type parameters:
//   - K: the key type, must be comparable
type byteStorage[K comparable] interface {
	// Load retrieves a byte slice value from storage for the given key.
	// Returns the raw bytes and true if found, nil and false if not present.
	Load(ctx context.Context, key K) (value []byte, ok bool)

	// Store adds or updates a key with a byte slice value in storage.
	Store(ctx context.Context, key K, value []byte)

	// Delete removes one or more keys and their associated byte values from storage.
	Delete(ctx context.Context, keys ...K)

	// Range iterates over all key-byte value pairs in storage.
	Range(ctx context.Context, f func(key K, value []byte) bool)

	// Next returns and removes the next key-byte value pair from storage.
	Next(ctx context.Context) (key K, value []byte, ok bool)

	// Len returns the current number of key-value pairs in storage.
	Len(ctx context.Context) int

	// Clear removes all key-value pairs from storage.
	Clear(ctx context.Context)

	// Close releases any resources held by the storage implementation.
	Close(ctx context.Context) error
}

// mightyMapDirectStorage provides a direct in-memory storage implementation without any encoding.
// This is the fastest storage option as it stores values directly in a Go map with mutex protection.
// Values are stored as-is without any serialization overhead.
//
// Type parameters:
//   - K: the key type, must be comparable
//   - V: the value type, can be any type
type mightyMapDirectStorage[K comparable, V any] struct {
	data  map[K]V
	mutex *sync.RWMutex
}

// mightyMapDefaultStorage provides byte-based storage for implementations that require serialization.
// This storage type is used by adapters that handle MessagePack or other encoding schemes.
// It stores all values as byte slices and relies on higher-level adapters for type conversion.
//
// Type parameters:
//   - K: the key type, must be comparable
type mightyMapDefaultStorage[K comparable] struct {
	data  map[K][]byte
	mutex *sync.RWMutex
}

// NewMightyMapDefaultStorage creates a new default storage implementation with the specified key and value types.
// This function returns a direct in-memory storage without encoding for optimal performance.
// The storage uses a standard Go map protected by a read-write mutex for thread safety.
//
// This is the recommended storage for most use cases where persistence is not required
// and maximum performance is desired.
//
// Type parameters:
//   - K: the key type, must be comparable
//   - V: the value type, can be any type
//
// Returns a new IMightyMapStorage instance ready for use.
func NewMightyMapDefaultStorage[K comparable, V any]() IMightyMapStorage[K, V] {
	return &mightyMapDirectStorage[K, V]{
		data:  make(map[K]V),
		mutex: &sync.RWMutex{},
	}
}

// Load retrieves a value from the direct storage for the given key.
// This operation uses a read lock to ensure thread safety while allowing concurrent reads.
//
// Parameters:
//   - ctx: context for the operation (currently unused but maintained for interface compatibility)
//   - key: the key to look up
//
// Returns:
//   - value: the stored value if found, zero value if not found
//   - ok: true if the key exists, false otherwise
func (c *mightyMapDirectStorage[K, V]) Load(_ context.Context, key K) (value V, ok bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	value, ok = c.data[key]
	return
}

// Store adds or updates a key-value pair in the direct storage.
// This operation uses a write lock to ensure thread safety and data consistency.
//
// Parameters:
//   - ctx: context for the operation (currently unused but maintained for interface compatibility)
//   - key: the key to store
//   - value: the value to associate with the key
func (c *mightyMapDirectStorage[K, V]) Store(_ context.Context, key K, value V) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.data[key] = value
}

// Delete removes one or more keys and their associated values from the direct storage.
// Non-existent keys are silently ignored. This operation uses a write lock for thread safety.
//
// Parameters:
//   - ctx: context for the operation (currently unused but maintained for interface compatibility)
//   - keys: one or more keys to remove from storage
func (c *mightyMapDirectStorage[K, V]) Delete(_ context.Context, keys ...K) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for _, key := range keys {
		delete(c.data, key)
	}
}

// Range iterates over all key-value pairs in the direct storage in an unspecified order.
// The iteration uses a read lock to ensure data consistency during traversal.
// If the provided function returns false, iteration stops early.
//
// Parameters:
//   - ctx: context for the operation (currently unused but maintained for interface compatibility)
//   - f: function called for each key-value pair; return false to stop iteration
func (c *mightyMapDirectStorage[K, V]) Range(_ context.Context, f func(key K, value V) bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	for k, v := range c.data {
		if !f(k, v) {
			break
		}
	}
}

// Len returns the current number of key-value pairs in the direct storage.
// This operation uses a read lock to ensure an accurate count.
//
// Parameters:
//   - ctx: context for the operation (currently unused but maintained for interface compatibility)
//
// Returns the number of stored key-value pairs.
func (c *mightyMapDirectStorage[K, V]) Len(_ context.Context) int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return len(c.data)
}

// Clear removes all key-value pairs from the direct storage.
// This operation creates a new empty map and uses a write lock for thread safety.
//
// Parameters:
//   - ctx: context for the operation (currently unused but maintained for interface compatibility)
func (c *mightyMapDirectStorage[K, V]) Clear(_ context.Context) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.data = make(map[K]V)
}

// Next returns and removes the next key-value pair from the direct storage.
// The iteration order is not specified and depends on Go's map iteration behavior.
// This operation is atomic - the key-value pair is removed as part of retrieval.
//
// Note: This method has a known limitation with zero-value keys. If a key equals
// the zero value of type K, the method may not handle it correctly due to the
// zero-value detection mechanism used.
//
// Parameters:
//   - ctx: context for the operation, passed to Range and Delete methods
//
// Returns:
//   - key: the key of the retrieved pair, zero value if storage is empty
//   - value: the value of the retrieved pair, zero value if storage is empty
//   - ok: true if a pair was found and removed, false if storage is empty
func (c *mightyMapDirectStorage[K, V]) Next(ctx context.Context) (key K, value V, ok bool) {
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

// Close releases any resources held by the direct storage.
// For the direct storage implementation, no cleanup is required.
//
// Parameters:
//   - ctx: context for the operation (currently unused but maintained for interface compatibility)
//
// Returns nil as no errors can occur during cleanup.
func (c *mightyMapDirectStorage[K, V]) Close(_ context.Context) error {
	// No resources to clean up for direct storage
	return nil
}

// The following methods implement the byteStorage interface for the byte-based storage implementation.
// These methods are used by encoding adapters that need to store serialized data.

// Load retrieves a byte slice value from the byte storage for the given key.
// This operation uses a read lock to ensure thread safety while allowing concurrent reads.
//
// Parameters:
//   - ctx: context for the operation (currently unused but maintained for interface compatibility)
//   - key: the key to look up
//
// Returns:
//   - value: the stored byte slice if found, nil if not found
//   - ok: true if the key exists, false otherwise
func (c *mightyMapDefaultStorage[K]) Load(_ context.Context, key K) (value []byte, ok bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	value, ok = c.data[key]
	return
}

// Store adds or updates a key with a byte slice value in the byte storage.
// This operation uses a write lock to ensure thread safety and data consistency.
//
// Parameters:
//   - ctx: context for the operation (currently unused but maintained for interface compatibility)
//   - key: the key to store
//   - value: the byte slice to associate with the key
func (c *mightyMapDefaultStorage[K]) Store(_ context.Context, key K, value []byte) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.data[key] = value
}

// Delete removes one or more keys and their associated byte values from the byte storage.
// Non-existent keys are silently ignored. This operation uses a write lock for thread safety.
//
// Parameters:
//   - ctx: context for the operation (currently unused but maintained for interface compatibility)
//   - keys: one or more keys to remove from storage
func (c *mightyMapDefaultStorage[K]) Delete(_ context.Context, keys ...K) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for _, key := range keys {
		delete(c.data, key)
	}
}

// Range iterates over all key-byte value pairs in the byte storage in an unspecified order.
// The iteration uses a read lock to ensure data consistency during traversal.
// If the provided function returns false, iteration stops early.
//
// Parameters:
//   - ctx: context for the operation (currently unused but maintained for interface compatibility)
//   - f: function called for each key-value pair; return false to stop iteration
func (c *mightyMapDefaultStorage[K]) Range(_ context.Context, f func(key K, value []byte) bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	for k, v := range c.data {
		if !f(k, v) {
			break
		}
	}
}

// Len returns the current number of key-value pairs in the byte storage.
// This operation uses a read lock to ensure an accurate count.
//
// Parameters:
//   - ctx: context for the operation (currently unused but maintained for interface compatibility)
//
// Returns the number of stored key-value pairs.
func (c *mightyMapDefaultStorage[K]) Len(_ context.Context) int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return len(c.data)
}

// Clear removes all key-value pairs from the byte storage.
// This operation creates a new empty map and uses a write lock for thread safety.
//
// Parameters:
//   - ctx: context for the operation (currently unused but maintained for interface compatibility)
func (c *mightyMapDefaultStorage[K]) Clear(_ context.Context) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.data = make(map[K][]byte)
}

// Next returns and removes the next key-byte value pair from the byte storage.
// The iteration order is not specified and depends on Go's map iteration behavior.
// This operation is atomic - the key-value pair is removed as part of retrieval.
//
// Note: This method has a known limitation with zero-value keys. If a key equals
// the zero value of type K, the method may not handle it correctly due to the
// zero-value detection mechanism used.
//
// Parameters:
//   - ctx: context for the operation, passed to Range and Delete methods
//
// Returns:
//   - key: the key of the retrieved pair, zero value if storage is empty
//   - value: the byte slice of the retrieved pair, nil if storage is empty
//   - ok: true if a pair was found and removed, false if storage is empty
func (c *mightyMapDefaultStorage[K]) Next(ctx context.Context) (key K, value []byte, ok bool) {
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

// Close releases any resources held by the byte storage.
// For the byte storage implementation, no cleanup is required.
//
// Parameters:
//   - ctx: context for the operation (currently unused but maintained for interface compatibility)
//
// Returns nil as no errors can occur during cleanup.
func (c *mightyMapDefaultStorage[K]) Close(_ context.Context) error {
	// No resources to clean up for byte storage
	return nil
}
