package storage

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/vmihailenco/msgpack/v5"
)

// typeRegistry maps type names to their concrete types
var (
	typeRegistry     = make(map[string]reflect.Type)
	typeRegistryLock sync.RWMutex
)

// The msgpackAdapter adapts any byteStorage implementation to implement IMightyMapStorage interface
type msgpackAdapter[K comparable, V any] struct {
	storage byteStorage[K]
}

// newMsgpackAdapter creates a new adapter that uses MessagePack encoding to convert between V and []byte
func newMsgpackAdapter[K comparable, V any](storage byteStorage[K]) *msgpackAdapter[K, V] {
	return &msgpackAdapter[K, V]{
		storage: storage,
	}
}

// msgpackEncodeValue encodes a value to a byte slice using MessagePack encoding
func msgpackEncodeValue[V any](value V) ([]byte, error) {
	// Store the type information along with the value
	valueType := reflect.TypeOf(value)

	// Create a wrapper with type information
	wrapper := map[string]interface{}{
		"data": value,
	}

	// Add type information only for non-nil, non-interface types
	if valueType != nil && valueType.Kind() != reflect.Interface {
		wrapper["type"] = valueType.String()

		// Register the type if not already registered
		typeRegistryLock.RLock()
		_, exists := typeRegistry[valueType.String()]
		typeRegistryLock.RUnlock()

		if !exists {
			RegisterMsgpackType(value)
		}
	}

	encoded, err := msgpack.Marshal(wrapper)
	if err != nil {
		return nil, fmt.Errorf("failed to msgpack encode value: %w", err)
	}
	return encoded, nil
}

// msgpackDecodeValue decodes a byte slice to a value using MessagePack encoding
func msgpackDecodeValue[V any](data []byte) (V, error) {
	var value V
	if len(data) == 0 {
		return value, nil
	}

	// Try to decode with type information first
	var wrapper map[string]interface{}
	err := msgpack.Unmarshal(data, &wrapper)
	if err != nil {
		// Return the error immediately for invalid msgpack data
		return value, fmt.Errorf("failed to msgpack decode value: %w", err)
	}

	// Extract the data part from the wrapper
	rawData, ok := wrapper["data"]
	if !ok {
		// No data field, try direct decoding
		err = msgpack.Unmarshal(data, &value)
		if err != nil {
			return value, fmt.Errorf("failed to msgpack decode value: %w", err)
		}
		return value, nil
	}

	// Check if we have type information
	typeName, hasType := wrapper["type"].(string)
	if !hasType {
		// No type information, try to decode data directly
		encoded, err := msgpack.Marshal(rawData)
		if err != nil {
			return value, fmt.Errorf("failed to re-encode inner data: %w", err)
		}

		err = msgpack.Unmarshal(encoded, &value)
		if err != nil {
			return value, fmt.Errorf("failed to decode inner data: %w", err)
		}
		return value, nil
	}

	// Look up the registered type
	typeRegistryLock.RLock()
	valueType, exists := typeRegistry[typeName]
	typeRegistryLock.RUnlock()

	if !exists {
		// Type not found, try decoding data directly
		encoded, err := msgpack.Marshal(rawData)
		if err != nil {
			return value, fmt.Errorf("failed to re-encode inner data: %w", err)
		}

		err = msgpack.Unmarshal(encoded, &value)
		if err != nil {
			return value, fmt.Errorf("failed to decode inner data: %w", err)
		}
		return value, nil
	}

	// Create a new instance of the concrete type
	newValue := reflect.New(valueType).Interface()

	// Re-encode the data part
	reencoded, err := msgpack.Marshal(rawData)
	if err != nil {
		return value, fmt.Errorf("failed to re-encode inner data: %w", err)
	}

	// Decode into the concrete type
	err = msgpack.Unmarshal(reencoded, newValue)
	if err != nil {
		return value, fmt.Errorf("failed to decode to concrete type: %w", err)
	}

	// Check if we can assign to the expected type
	valueOfV := reflect.ValueOf(&value).Elem()

	// For interface{} types, we can just set the concrete value
	if valueOfV.Kind() == reflect.Interface && valueOfV.CanSet() {
		valueOfV.Set(reflect.ValueOf(reflect.ValueOf(newValue).Elem().Interface()))
		return value, nil
	}

	// For non-interface types, try direct decoding
	encoded, err := msgpack.Marshal(rawData)
	if err != nil {
		return value, fmt.Errorf("failed to re-encode data: %w", err)
	}

	err = msgpack.Unmarshal(encoded, &value)
	if err != nil {
		return value, fmt.Errorf("failed to decode to expected type: %w", err)
	}

	return value, nil
}

// RegisterMsgpackType registers a type with the MessagePack encoder.
// Unlike Gob, MessagePack doesn't require explicit registration, but
// we use this to maintain a type registry for proper type conversion.
func RegisterMsgpackType(value interface{}) {
	t := reflect.TypeOf(value)

	// If it's a pointer, get the underlying element type
	if t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	// Only register if it's a valid type
	if t != nil {
		typeRegistryLock.Lock()
		typeRegistry[t.String()] = t
		typeRegistryLock.Unlock()
	}
}

// Load retrieves a value from the storage
func (m *msgpackAdapter[K, V]) Load(ctx context.Context, key K) (value V, ok bool) {
	var zeroV V
	data, ok := m.storage.Load(ctx, key)
	if !ok {
		return zeroV, false
	}

	decoded, err := msgpackDecodeValue[V](data)
	if err != nil {
		// If we can't decode, it's as if the key isn't there
		return zeroV, false
	}

	return decoded, true
}

// Store serializes and stores a value in the storage
func (m *msgpackAdapter[K, V]) Store(ctx context.Context, key K, value V) {
	encoded, err := msgpackEncodeValue(value)
	if err != nil {
		// If we can't encode, we don't store anything
		return
	}

	m.storage.Store(ctx, key, encoded)
}

// Delete removes one or more keys from the storage
func (m *msgpackAdapter[K, V]) Delete(ctx context.Context, keys ...K) {
	m.storage.Delete(ctx, keys...)
}

// Range iterates over all key-value pairs in the storage
func (m *msgpackAdapter[K, V]) Range(ctx context.Context, f func(key K, value V) bool) {
	m.storage.Range(ctx, func(key K, data []byte) bool {
		decoded, err := msgpackDecodeValue[V](data)
		if err != nil {
			// Skip entries that can't be decoded
			return true
		}

		return f(key, decoded)
	})
}

// Keys returns all keys in the storage in an unspecified order.
func (m *msgpackAdapter[K, V]) Keys(ctx context.Context) []K {
	return m.storage.Keys(ctx)
}

// Next returns the next key-value pair from the storage
func (m *msgpackAdapter[K, V]) Next(ctx context.Context) (key K, value V, ok bool) {
	var zeroV V
	k, data, ok := m.storage.Next(ctx)
	if !ok {
		return k, zeroV, false
	}

	decoded, err := msgpackDecodeValue[V](data)
	if err != nil {
		// If we can't decode, it's as if there are no more items
		return k, zeroV, false
	}

	return k, decoded, true
}

// Len returns the number of items in the storage
func (m *msgpackAdapter[K, V]) Len(ctx context.Context) int {
	return m.storage.Len(ctx)
}

// Clear removes all items from the storage
func (m *msgpackAdapter[K, V]) Clear(ctx context.Context) {
	m.storage.Clear(ctx)
}

// Close closes the storage
func (m *msgpackAdapter[K, V]) Close(ctx context.Context) error {
	return m.storage.Close(ctx)
}
