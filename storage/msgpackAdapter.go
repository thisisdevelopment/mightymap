package storage

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
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
	// cache holds decoded values that were loaded recently; nil when it is off.
	// With a cache every Load of a key returns the same value, so callers must
	// treat loaded values as read-only.
	cache *valueCache[K, V]
}

// newMsgpackAdapter creates a new adapter that uses MessagePack encoding to convert between V and []byte
func newMsgpackAdapter[K comparable, V any](storage byteStorage[K]) *msgpackAdapter[K, V] {
	return &msgpackAdapter[K, V]{
		storage: storage,
	}
}

// Field names of the wrapper a value is stored in: a MessagePack map that holds
// the name of the value's type next to the value itself.
const (
	wrapperTypeField = "type"
	wrapperDataField = "data"
)

// msgpackEncodeValue encodes a value to a byte slice using MessagePack encoding
func msgpackEncodeValue[V any](value V) ([]byte, error) {
	// Store the type information along with the value, but only for non-nil,
	// non-interface types
	valueType := reflect.TypeOf(value)
	typed := valueType != nil && valueType.Kind() != reflect.Interface

	var buf bytes.Buffer
	enc := msgpack.GetEncoder()
	defer msgpack.PutEncoder(enc)
	enc.Reset(&buf)

	// The type goes first, so a reader knows it before it reaches the data and
	// can decode the data in one go.
	fields := 1
	if typed {
		fields = 2
	}
	err := enc.EncodeMapLen(fields)
	if err == nil && typed {
		registerMsgpackType(valueType)
		if err = enc.EncodeString(wrapperTypeField); err == nil {
			err = enc.EncodeString(valueType.String())
		}
	}
	if err == nil {
		if err = enc.EncodeString(wrapperDataField); err == nil {
			err = enc.Encode(value)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to msgpack encode value: %w", err)
	}
	return buf.Bytes(), nil
}

// msgpackDecodeValue decodes a byte slice to a value using MessagePack encoding.
// The data is decoded once, straight into its type.
func msgpackDecodeValue[V any](data []byte) (V, error) {
	var value V
	if len(data) == 0 {
		return value, nil
	}

	dec := msgpack.GetDecoder()
	defer msgpack.PutDecoder(dec)
	dec.Reset(bytes.NewReader(data))

	fields, err := dec.DecodeMapLen()
	if err != nil {
		// Return the error immediately for invalid msgpack data
		return value, fmt.Errorf("failed to msgpack decode value: %w", err)
	}

	var (
		typeName string
		raw      msgpack.RawMessage // the data, when it comes before its type
		hasData  bool
	)
	for i := 0; i < fields; i++ {
		field, err := dec.DecodeString()
		if err != nil {
			return value, fmt.Errorf("failed to msgpack decode value: %w", err)
		}
		switch field {
		case wrapperTypeField:
			var name any
			name, err = dec.DecodeInterface()
			typeName, _ = name.(string)
		case wrapperDataField:
			if typeName != "" {
				return msgpackDecodeAs[V](typeName, dec.Decode)
			}
			// Written by an earlier version with the data first: keep the
			// bytes until the type is known.
			raw, err = dec.DecodeRaw()
			hasData = true
		default:
			err = dec.Skip()
		}
		if err != nil {
			return value, fmt.Errorf("failed to msgpack decode value: %w", err)
		}
	}

	if !hasData {
		// No data field, the bytes are the value itself
		if err = msgpack.Unmarshal(data, &value); err != nil {
			return value, fmt.Errorf("failed to msgpack decode value: %w", err)
		}
		return value, nil
	}
	return msgpackDecodeAs[V](typeName, func(target any) error {
		return msgpack.Unmarshal(raw, target)
	})
}

// msgpackDecodeAs decodes into a V. When V is an interface the result gets the
// registered type with the given name, so a value comes back as what it was
// stored as: a struct as that struct, a pointer as a pointer.
func msgpackDecodeAs[V any](typeName string, decode func(target any) error) (V, error) {
	var value V
	target := reflect.ValueOf(&value).Elem()

	if target.Kind() == reflect.Interface {
		if concrete, ok := lookupMsgpackType(typeName); ok && concrete.AssignableTo(target.Type()) {
			decoded := reflect.New(concrete)
			if err := decode(decoded.Interface()); err != nil {
				return value, fmt.Errorf("failed to decode to concrete type: %w", err)
			}
			target.Set(decoded.Elem())
			return value, nil
		}
	}

	// A concrete V, or a type that is not registered: decode directly
	if err := decode(&value); err != nil {
		return value, fmt.Errorf("failed to decode to expected type: %w", err)
	}
	return value, nil
}

// registerMsgpackType registers the type under the name RegisterMsgpackType
// uses for it, unless it is there already.
func registerMsgpackType(t reflect.Type) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	typeRegistryLock.RLock()
	_, exists := typeRegistry[t.String()]
	typeRegistryLock.RUnlock()
	if !exists {
		typeRegistryLock.Lock()
		typeRegistry[t.String()] = t
		typeRegistryLock.Unlock()
	}
}

// lookupMsgpackType finds the type for a name as written by msgpackEncodeValue.
// Pointer types are registered by their element type, so "*pkg.T" resolves
// through "pkg.T".
func lookupMsgpackType(name string) (reflect.Type, bool) {
	pointers := 0
	typeRegistryLock.RLock()
	t, ok := typeRegistry[name]
	for !ok && strings.HasPrefix(name, "*") {
		name = name[1:]
		pointers++
		t, ok = typeRegistry[name]
	}
	typeRegistryLock.RUnlock()
	if !ok {
		return nil, false
	}
	for ; pointers > 0; pointers-- {
		t = reflect.PointerTo(t)
	}
	return t, true
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
	if cached, ok := m.cache.get(key); ok {
		return cached, true
	}

	writeCount := m.cache.writeCount()
	data, ok := m.storage.Load(ctx, key)
	if !ok {
		return zeroV, false
	}

	decoded, err := msgpackDecodeValue[V](data)
	if err != nil {
		// If we can't decode, it's as if the key isn't there
		return zeroV, false
	}

	m.cache.add(key, decoded, int64(len(data)), writeCount)
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
	m.cache.drop(key)
}

// Delete removes one or more keys from the storage
func (m *msgpackAdapter[K, V]) Delete(ctx context.Context, keys ...K) {
	m.storage.Delete(ctx, keys...)
	m.cache.drop(keys...)
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
	m.cache.drop(k)

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
	m.cache.clear()
}

// Close closes the storage
func (m *msgpackAdapter[K, V]) Close(ctx context.Context) error {
	return m.storage.Close(ctx)
}
