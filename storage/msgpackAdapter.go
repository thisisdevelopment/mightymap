package storage

import (
	"bytes"
	"context"
	"errors"
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
	// storageKeepsNothing is set for a storage whose Store does not hold on to
	// the slice it is given: it then gets the encode buffer itself, not a copy.
	storageKeepsNothing bool
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

// encodeBuffers holds the scratch buffers values are encoded in. Without them
// every value grows a buffer of its own, which allocates several times its size.
var encodeBuffers = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// A buffer that grew larger than this for one big value is not kept for reuse.
const maxPooledEncodeBuffer = 1 << 20

func getEncodeBuffer() *bytes.Buffer {
	buf := encodeBuffers.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

func putEncodeBuffer(buf *bytes.Buffer) {
	if buf.Cap() <= maxPooledEncodeBuffer {
		encodeBuffers.Put(buf)
	}
}

// msgpackEncodeValue encodes a value to a byte slice using MessagePack encoding.
// The slice is the caller's and exactly as large as the encoded value.
func msgpackEncodeValue[V any](value V) ([]byte, error) {
	buf := getEncodeBuffer()
	defer putEncodeBuffer(buf)

	if err := msgpackEncodeInto(buf, value); err != nil {
		return nil, err
	}
	return cloneExact(buf.Bytes()), nil
}

// cloneExact copies data into a slice without spare capacity.
func cloneExact(data []byte) []byte {
	clone := make([]byte, len(data))
	copy(clone, data)
	return clone
}

// msgpackEncodeInto appends the MessagePack encoding of value to buf.
func msgpackEncodeInto[V any](buf *bytes.Buffer, value V) error {
	// Store the type information along with the value, but only for non-nil,
	// non-interface types
	valueType := reflect.TypeOf(value)
	typed := valueType != nil && valueType.Kind() != reflect.Interface

	enc := msgpack.GetEncoder()
	defer msgpack.PutEncoder(enc)
	enc.Reset(buf)

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
		return fmt.Errorf("failed to msgpack encode value: %w", err)
	}
	return nil
}

// msgpackDecodeValue decodes a byte slice to a value using MessagePack encoding.
// The data is decoded once, straight into its type.
func msgpackDecodeValue[V any](data []byte) (V, error) {
	var value V
	if len(data) == 0 {
		return value, nil
	}

	if err := msgpackCheckLengths(data); err != nil {
		return value, fmt.Errorf("failed to msgpack decode value: %w", err)
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

var errMsgpackLength = errors.New("msgpack: data is cut off or claims a length it does not hold")

// msgpackCheckLengths walks over the first MessagePack value in data and
// reports whether every length in it fits in the bytes that follow. The
// decoder allocates for the length of a slice before it reads the elements,
// so a few damaged bytes that claim four billion elements would cost
// gigabytes. After this check a claimed length is never more than the data
// has bytes.
//
//nolint:mnd,gocyclo // the numbers are the MessagePack format codes
func msgpackCheckLengths(data []byte) error {
	pending := 1 // values still to come
	for pos := 0; pending > 0; pending-- {
		if pos >= len(data) {
			return errMsgpackLength
		}
		code := data[pos]
		pos++

		var skip, sizeBytes, children int
		switch {
		case code <= 0x7f, code >= 0xe0, code == 0xc0, code == 0xc2, code == 0xc3: // small ints, nil, bools
		case code <= 0x8f: // map of up to 15
			children = 2 * int(code&0x0f)
		case code <= 0x9f: // array of up to 15
			children = int(code & 0x0f)
		case code <= 0xbf: // string of up to 31
			skip = int(code & 0x1f)
		case code == 0xc4, code == 0xd9: // bin, string
			sizeBytes = 1
		case code == 0xc5, code == 0xda:
			sizeBytes = 2
		case code == 0xc6, code == 0xdb:
			sizeBytes = 4
		case code == 0xc7: // ext: one more byte for its type
			sizeBytes, skip = 1, 1
		case code == 0xc8:
			sizeBytes, skip = 2, 1
		case code == 0xc9:
			sizeBytes, skip = 4, 1
		case code == 0xca, code == 0xce, code == 0xd2: // float32, uint32, int32
			skip = 4
		case code == 0xcb, code == 0xcf, code == 0xd3: // float64, uint64, int64
			skip = 8
		case code == 0xcc, code == 0xd0:
			skip = 1
		case code == 0xcd, code == 0xd1:
			skip = 2
		case code >= 0xd4 && code <= 0xd8: // ext of 1, 2, 4, 8 or 16 bytes
			skip = 1 + 1<<(code-0xd4)
		case code == 0xdc, code == 0xde: // array, map
			sizeBytes = 2
		case code == 0xdd, code == 0xdf:
			sizeBytes = 4
		default: // 0xc1 is not used by MessagePack
			return errMsgpackLength
		}

		if sizeBytes > 0 {
			if pos+sizeBytes > len(data) {
				return errMsgpackLength
			}
			size := 0
			for _, b := range data[pos : pos+sizeBytes] {
				size = size<<8 | int(b)
			}
			pos += sizeBytes
			switch code {
			case 0xdc, 0xdd:
				children = size
			case 0xde, 0xdf:
				children = 2 * size
			default:
				skip += size
			}
		}
		// Every child takes at least one byte.
		if skip > len(data)-pos || children > len(data)-pos-skip {
			return errMsgpackLength
		}
		pos += skip
		pending += children
	}
	return nil
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
	buf := getEncodeBuffer()
	defer putEncodeBuffer(buf)

	if err := msgpackEncodeInto(buf, value); err != nil {
		// If we can't encode, we don't store anything
		return
	}
	encoded := buf.Bytes()
	if !m.storageKeepsNothing {
		encoded = cloneExact(encoded) // the buffer is reused, the storage may keep its slice
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
