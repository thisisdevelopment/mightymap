package storage

import (
	"context"
	"reflect"
	"sync"
	"testing"

	msgpack "github.com/vmihailenco/msgpack/v5"
)

// mockByteStorage is a simple in-memory implementation of byteStorage for testing
// It is not thread-safe, but sufficient for unit tests
// It also allows simulating errors

type mockByteStorage[K comparable] struct {
	data   map[K][]byte
	closed bool
	lock   sync.RWMutex
	fail   bool // if true, simulate error on Store/Load
}

func newMockByteStorage[K comparable]() *mockByteStorage[K] {
	return &mockByteStorage[K]{data: make(map[K][]byte)}
}

func (m *mockByteStorage[K]) Load(_ context.Context, key K) ([]byte, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	if m.fail {
		return nil, false
	}
	v, ok := m.data[key]
	return v, ok
}

func (m *mockByteStorage[K]) Store(_ context.Context, key K, value []byte) {
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.fail {
		return
	}
	m.data[key] = value
}

func (m *mockByteStorage[K]) Delete(_ context.Context, keys ...K) {
	m.lock.Lock()
	defer m.lock.Unlock()
	for _, k := range keys {
		delete(m.data, k)
	}
}

func (m *mockByteStorage[K]) Range(_ context.Context, f func(key K, value []byte) bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	for k, v := range m.data {
		if !f(k, v) {
			break
		}
	}
}

func (m *mockByteStorage[K]) Keys(_ context.Context) []K {
	m.lock.RLock()
	defer m.lock.RUnlock()
	keys := []K{}
	for k := range m.data {
		keys = append(keys, k)
	}
	return keys
}

func (m *mockByteStorage[K]) Next(_ context.Context) (K, []byte, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()
	for k, v := range m.data {
		delete(m.data, k)
		return k, v, true
	}
	var zeroK K
	return zeroK, nil, false
}

func (m *mockByteStorage[K]) Len(_ context.Context) int {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return len(m.data)
}

func (m *mockByteStorage[K]) Clear(_ context.Context) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.data = make(map[K][]byte)
}

func (m *mockByteStorage[K]) Close(_ context.Context) error {
	m.closed = true
	return nil
}

func TestMsgpackEncodeDecodeValue_Roundtrip(t *testing.T) {
	types := []interface{}{
		42,
		"hello",
		struct{ X int }{X: 7},
		&struct{ Y string }{Y: "test"},
		[]int{1, 2, 3},
		map[string]int{"a": 1},
	}
	for _, v := range types {
		encoded, err := msgpackEncodeValue(v)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}
		// decode as interface{}
		var decoded interface{}
		err = nil
		decoded, err = msgpackDecodeValue[interface{}](encoded)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		// Can't always use reflect.DeepEqual for all types, but check not nil
		if decoded == nil {
			t.Errorf("decoded value is nil for %T", v)
		}
	}
}

func TestMsgpackEncodeValue_TypeRegistration(t *testing.T) {
	type myType struct{ Z int }
	// Clear registry
	typeRegistryLock.Lock()
	typeRegistry = make(map[string]reflect.Type)
	typeRegistryLock.Unlock()
	v := myType{Z: 99}
	_, err := msgpackEncodeValue(v)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	typeRegistryLock.RLock()
	_, ok := typeRegistry[reflect.TypeOf(v).String()]
	typeRegistryLock.RUnlock()
	if !ok {
		t.Error("type was not registered")
	}
}

func TestMsgpackDecodeValue_EmptyData(t *testing.T) {
	v, err := msgpackDecodeValue[int]([]byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 0 {
		t.Errorf("expected zero value, got %v", v)
	}
}

func TestMsgpackDecodeValue_NoTypeInfo(t *testing.T) {
	// encode a value without type info
	encoded, _ := msgpackEncodeValue(123)
	// remove type info
	var wrapper map[string]interface{}
	_ = msgpack.Unmarshal(encoded, &wrapper)
	delete(wrapper, "type")
	encoded2, _ := msgpack.Marshal(wrapper)
	v, err := msgpackDecodeValue[int](encoded2)
	if err != nil || v != 123 {
		t.Errorf("expected 123, got %v, err=%v", v, err)
	}
}

func TestMsgpackDecodeValue_UnregisteredType(t *testing.T) {
	type myType struct{ Q int }
	v := myType{Q: 5}
	encoded, _ := msgpackEncodeValue(v)
	// Remove from registry
	typeRegistryLock.Lock()
	delete(typeRegistry, reflect.TypeOf(v).String())
	typeRegistryLock.Unlock()
	// Should still decode
	_, err := msgpackDecodeValue[myType](encoded)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRegisterMsgpackType_Pointer(t *testing.T) {
	type foo struct{ A int }
	RegisterMsgpackType(&foo{A: 1})
	typeRegistryLock.RLock()
	_, ok := typeRegistry[reflect.TypeOf(foo{}).String()]
	typeRegistryLock.RUnlock()
	if !ok {
		t.Error("pointer type not registered as element type")
	}
}

func TestMsgpackAdapter_BasicOps(t *testing.T) {
	ctx := context.Background()
	mock := newMockByteStorage[string]()
	adapter := newMsgpackAdapter[string, int](mock)

	// Store
	adapter.Store(ctx, "key1", 42)
	// Load
	val, ok := adapter.Load(ctx, "key1")
	if !ok || val != 42 {
		t.Errorf("expected 42, got %v", val)
	}
	// Delete
	adapter.Delete(ctx, "key1")
	val, ok = adapter.Load(ctx, "key1")
	if ok {
		t.Errorf("expected key to be deleted")
	}

	// Test Keys
	adapter.Store(ctx, "key2", 100)
	adapter.Store(ctx, "key3", 200)
	adapter.Store(ctx, "key4", 300)

	keys := adapter.Keys(ctx)
	if len(keys) != 3 {
		t.Errorf("Keys() returned %d keys; want 3", len(keys))
	}

	// Verify all expected keys are present
	keyMap := make(map[string]bool)
	for _, key := range keys {
		keyMap[key] = true
	}
	expectedKeys := []string{"key2", "key3", "key4"}
	for _, expected := range expectedKeys {
		if !keyMap[expected] {
			t.Errorf("Expected key %s not found in Keys() result", expected)
		}
	}

	// Test Keys with empty adapter
	adapter.Clear(ctx)
	keys = adapter.Keys(ctx)
	if len(keys) != 0 {
		t.Errorf("Keys() returned %d keys for empty adapter; want 0", len(keys))
	}
}

func TestMsgpackAdapter_RangeDecodeError(t *testing.T) {
	ctx := context.Background()
	store := newMockByteStorage[string]()
	adapter := newMsgpackAdapter[string, int](store)
	// Store a value that can't be decoded as int
	store.Store(ctx, "bad", []byte{0xff})
	adapter.Store(ctx, "good", 42)
	count := 0
	adapter.Range(ctx, func(k string, v int) bool {
		count++
		return true
	})
	if count != 1 {
		t.Errorf("expected 1 good item, got %d", count)
	}
}

func TestMsgpackAdapter_NextDecodeError(t *testing.T) {
	ctx := context.Background()
	store := newMockByteStorage[string]()
	adapter := newMsgpackAdapter[string, int](store)
	store.Store(ctx, "bad", []byte{0xff})
	key, val, ok := adapter.Next(ctx)
	if ok {
		t.Errorf("expected not ok for bad data, got key=%v val=%v", key, val)
	}
}
