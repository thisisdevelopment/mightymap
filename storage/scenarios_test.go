package storage

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	msgpack "github.com/vmihailenco/msgpack/v5"
)

// Scenarios that had no test: values that cannot be encoded or decoded, value
// types through every serializing backend, the SQLite count cache, stopping a
// Range early, draining with Next and a few options.

// msgpackFields encodes a MessagePack map from alternating field names and
// values; an odd number of parts leaves the last value out (truncated data).
func msgpackFields(t *testing.T, fields int, parts ...any) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	if err := enc.EncodeMapLen(fields); err != nil {
		t.Fatal(err)
	}
	for _, part := range parts {
		if err := enc.Encode(part); err != nil {
			t.Fatal(err)
		}
	}
	return buf.Bytes()
}

func TestMsgpackAdapter_ValueThatCannotBeEncodedIsNotStored(t *testing.T) {
	ctx := context.Background()
	adapter := newMsgpackAdapter[string, any](newMockByteStorage[string]())

	adapter.Store(ctx, "func", func() {})
	adapter.Store(ctx, "chan", make(chan int))
	adapter.Store(ctx, "ok", 1)

	if n := adapter.Len(ctx); n != 1 {
		t.Errorf("Len() = %d, want 1: only the encodable value is stored", n)
	}
	if _, ok := adapter.Load(ctx, "func"); ok {
		t.Error("Load() found a value that could not be encoded")
	}
}

func TestMsgpackAdapter_LoadOfBrokenDataIsNotFound(t *testing.T) {
	ctx := context.Background()
	raw := newMockByteStorage[string]()
	raw.Store(ctx, "broken", []byte{0xff})
	adapter := newMsgpackAdapter[string, any](raw)

	if value, ok := adapter.Load(ctx, "broken"); ok {
		t.Errorf("Load() = %v, true; want not found for data that cannot be decoded", value)
	}
}

func TestMsgpackDecodeValue_BrokenData(t *testing.T) {
	RegisterMsgpackType(0)
	unwrapped, err := msgpack.Marshal(map[string]string{"a": "b"})
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string][]byte{
		"map header only":              {0x82},
		"field name is not a string":   {0x81, 0x01, 0x01},
		"type without a value":         msgpackFields(t, 1, "type"),
		"unknown field without value":  msgpackFields(t, 1, "other"),
		"data without a value":         msgpackFields(t, 2, "type", "int", "data"),
		"data does not fit its type":   msgpackFields(t, 2, "type", "int", "data", "text"),
		"data does not fit the map":    msgpackFields(t, 1, "data", "text"),
		"unwrapped, does not fit":      unwrapped,
		"earlier order, does not fit":  msgpackFields(t, 2, "data", "text", "type", "int"),
		"earlier order, data cut off":  msgpackFields(t, 2, "data"),
		"not messagepack":              {0xc1},
		"wrapper inside a longer map":  msgpackFields(t, 3, "type", "int"),
		"text where a map is expected": []byte("plain text"),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if value, err := msgpackDecodeValue[int](data); err == nil {
				t.Errorf("decode returned %v without an error", value)
			}
			if name == "data does not fit the map" || name == "unwrapped, does not fit" {
				return // these are valid for a map of any
			}
			if value, err := msgpackDecodeValue[any](data); err == nil {
				t.Errorf("decode into any returned %v without an error", value)
			}
		})
	}
}

type stringerItem struct{ ID string }

func (s stringerItem) String() string { return s.ID }

// A map whose value type is an interface gets its values back as the type
// that was stored, as long as that type implements the interface.
func TestMsgpackDecodeValue_IntoInterfaceType(t *testing.T) {
	encoded, err := msgpackEncodeValue[fmt.Stringer](stringerItem{ID: "a"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := msgpackDecodeValue[fmt.Stringer](encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if item, ok := got.(stringerItem); !ok || item.ID != "a" {
		t.Errorf("got %T %v, want stringerItem{a}", got, got)
	}
}

// Every serializing backend must return what was stored in a map of any:
// a struct as that struct, a pointer as a pointer, a slice as that slice.
func TestBackendsKeepValueTypes(t *testing.T) {
	item := newRoundTripItem(3)
	stored := map[string]any{
		"struct":  item,
		"pointer": &item,
		"slice":   []roundTripItem{item, item},
		"string":  "plain",
		"number":  42,
	}
	backends := map[string]func(t *testing.T) IMightyMapStorage[string, any]{
		"badger in memory": func(_ *testing.T) IMightyMapStorage[string, any] {
			return NewMightyMapBadgerStorage[string, any]()
		},
		"badger on disk": func(t *testing.T) IMightyMapStorage[string, any] {
			return NewMightyMapBadgerStorage[string, any](WithMemoryStorage(false), WithTempDir(t.TempDir()))
		},
		"sqlite": func(t *testing.T) IMightyMapStorage[string, any] {
			return NewMightyMapSQLiteStorage[string, any](WithSQLiteDBPath(filepath.Join(t.TempDir(), "kv.db")))
		},
		"redis": func(t *testing.T) IMightyMapStorage[string, any] {
			return NewMightyMapRedisStorage[string, any](WithRedisMock(t))
		},
		"segment files": func(t *testing.T) IMightyMapStorage[string, any] {
			return NewMightyMapSegmentStorage[string, any](WithSegmentDir(t.TempDir()))
		},
	}
	for name, open := range backends {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			store := open(t)
			defer store.Close(ctx)
			for key, value := range stored {
				store.Store(ctx, key, value)
			}

			for key, want := range stored {
				got, ok := store.Load(ctx, key)
				if !ok {
					t.Errorf("Load(%q) did not find the value", key)
				} else if !reflect.DeepEqual(got, want) {
					t.Errorf("Load(%q) = %T %+v, want %T %+v", key, got, got, want, want)
				}
			}

			seen := 0
			store.Range(ctx, func(key string, got any) bool {
				seen++
				if want := stored[key]; !reflect.DeepEqual(got, want) {
					t.Errorf("Range gave %q = %T %+v, want %T %+v", key, got, got, want, want)
				}
				return true
			})
			if seen != len(stored) {
				t.Errorf("Range visited %d values, want %d", seen, len(stored))
			}
		})
	}
}

func newSQLiteTestStore(t *testing.T, opts ...OptionFuncSQLite) IMightyMapStorage[string, int] {
	t.Helper()
	opts = append([]OptionFuncSQLite{WithSQLiteDBPath(filepath.Join(t.TempDir(), "kv.db"))}, opts...)
	store := NewMightyMapSQLiteStorage[string, int](opts...)
	t.Cleanup(func() { _ = store.Close(context.Background()) })
	return store
}

// Len is answered from a cached count, which every write has to reset.
func TestSQLiteLenIsCachedBetweenWrites(t *testing.T) {
	ctx := context.Background()
	store := newSQLiteTestStore(t, WithSQLiteCountCacheDuration(time.Hour))
	wantLen := func(step string, want int) {
		t.Helper()
		for i := range 2 { // the second call is served from the cache
			if got := store.Len(ctx); got != want {
				t.Errorf("%s: Len() call %d = %d, want %d", step, i+1, got, want)
			}
		}
	}

	wantLen("empty", 0)
	store.Store(ctx, "a", 1)
	store.Store(ctx, "b", 2)
	wantLen("after two stores", 2)
	store.Store(ctx, "a", 10)
	wantLen("after overwriting a key", 2)
	store.Delete(ctx, "a")
	wantLen("after a delete", 1)
	if _, _, ok := store.Next(ctx); !ok {
		t.Fatal("Next() found nothing")
	}
	wantLen("after Next", 0)
	store.Store(ctx, "c", 3)
	store.Clear(ctx)
	wantLen("after Clear", 0)
}

func TestSQLiteRangeStopsEarlyAndDeleteWithoutKeys(t *testing.T) {
	ctx := context.Background()
	store := newSQLiteTestStore(t)
	for i := range 3 {
		store.Store(ctx, fmt.Sprintf("key%d", i), i)
	}

	calls := 0
	store.Range(ctx, func(string, int) bool {
		calls++
		return false
	})
	if calls != 1 {
		t.Errorf("Range called the function %d times after it returned false, want 1", calls)
	}

	store.Delete(ctx)
	store.Delete(ctx, "missing")
	if got := store.Len(ctx); got != 3 {
		t.Errorf("Len() = %d after deleting nothing, want 3", got)
	}
}

func TestSQLiteCloseTwice(t *testing.T) {
	ctx := context.Background()
	store := NewMightyMapSQLiteStorage[string, int](WithSQLiteDBPath(filepath.Join(t.TempDir(), "kv.db")))
	if err := store.Close(ctx); err != nil {
		t.Fatalf("first Close(): %v", err)
	}
	if err := store.Close(ctx); err != nil {
		t.Errorf("second Close(): %v", err)
	}
}

func TestRedisRangeStopsEarly(t *testing.T) {
	ctx := context.Background()
	store := NewMightyMapRedisStorage[string, int](WithRedisMock(t))
	defer store.Close(ctx)
	for i := range 3 {
		store.Store(ctx, fmt.Sprintf("key%d", i), i)
	}

	calls := 0
	store.Range(ctx, func(string, int) bool {
		calls++
		return false
	})
	if calls != 1 {
		t.Errorf("Range called the function %d times after it returned false, want 1", calls)
	}
}

// Next hands out every value once and removes it; an empty store has no next.
func TestRedisNextDrainsTheStore(t *testing.T) {
	ctx := context.Background()
	store := NewMightyMapRedisStorage[string, int](WithRedisMock(t), WithRedisPrefix("drain:"))
	defer store.Close(ctx)

	if key, value, ok := store.Next(ctx); ok {
		t.Fatalf("Next() on an empty store = %q, %d, true", key, value)
	}

	want := map[string]int{"a": 1, "b": 2, "c": 3}
	for key, value := range want {
		store.Store(ctx, key, value)
	}
	for range len(want) {
		key, value, ok := store.Next(ctx)
		if !ok {
			t.Fatal("Next() stopped before the store was empty")
		}
		if wantValue, found := want[key]; !found || wantValue != value {
			t.Errorf("Next() = %q, %d; not a stored pair or handed out twice", key, value)
		}
		delete(want, key)
	}
	if _, _, ok := store.Next(ctx); ok {
		t.Error("Next() found a value after the store was drained")
	}
	if got := store.Len(ctx); got != 0 {
		t.Errorf("Len() = %d after draining, want 0", got)
	}
}

func TestWithRedisTLSConfigUsesTheGivenConfig(t *testing.T) {
	given := &tls.Config{ServerName: "redis.example.com", MinVersion: tls.VersionTLS13}
	opts := getDefaultRedisOptions()
	WithRedisTLSConfig(given)(opts)
	if opts.tlsConfig != given {
		t.Errorf("tlsConfig = %+v, want the config that was passed in", opts.tlsConfig)
	}

	// A later option replaces an earlier one.
	other := &tls.Config{ServerName: "other.example.com", MinVersion: tls.VersionTLS12}
	WithRedisTLSConfig(other)(opts)
	if opts.tlsConfig != other {
		t.Errorf("tlsConfig = %+v, want the config of the last option", opts.tlsConfig)
	}
}

func TestBadgerLoggingLevels(t *testing.T) {
	ctx := context.Background()
	for level, name := range []string{"debug", "info", "warning", "error"} {
		t.Run(name, func(t *testing.T) {
			store := NewMightyMapBadgerStorage[string, int](WithLoggingLevel(level))
			defer store.Close(ctx)
			store.Store(ctx, "key", level)
			if got, ok := store.Load(ctx, "key"); !ok || got != level {
				t.Errorf("Load() = %d, %v; want %d, true", got, ok, level)
			}
		})
	}
}

func TestBadgerEncryptionKeyLength(t *testing.T) {
	for _, key := range []string{"0123456789abcdef", "0123456789abcdef01234567", "0123456789abcdef0123456789abcdef"} {
		if WithEncryptionKey(key) == nil {
			t.Errorf("no option for a key of %d bytes", len(key))
		}
	}
	for _, key := range []string{"", "short", "0123456789abcdef0"} {
		t.Run(fmt.Sprintf("%d bytes", len(key)), func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("a key of %d bytes was accepted", len(key))
				}
			}()
			WithEncryptionKey(key)
		})
	}
}

// badgerGCGoroutines counts the background goroutines that Badger storages
// started themselves (their value log GC).
func badgerGCGoroutines() int {
	var dump bytes.Buffer
	_ = pprof.Lookup("goroutine").WriteTo(&dump, 2)
	count := 0
	for _, line := range strings.Split(dump.String(), "\n") {
		if strings.HasPrefix(line, "created by ") && strings.Contains(line, "storage.NewMightyMapBadgerStorage") {
			count++
		}
	}
	return count
}

// Closing a Badger storage must stop its value log GC. It used to keep
// running, and to hold on to the closed database, for as long as the process
// lived.
func TestBadgerCloseStopsValueLogGC(t *testing.T) {
	ctx := context.Background()
	modes := map[string][]OptionFuncBadger{
		"in memory": {WithGcInterval(time.Millisecond)},
		"on disk":   {WithGcInterval(time.Millisecond), WithMemoryStorage(false), WithTempDir(t.TempDir())},
	}
	for name, opts := range modes {
		t.Run(name, func(t *testing.T) {
			before := badgerGCGoroutines()
			store := NewMightyMapBadgerStorage[string, int](opts...)
			store.Store(ctx, "key", 1)
			time.Sleep(20 * time.Millisecond) // let the GC come round a few times
			if got := badgerGCGoroutines(); got != before+1 {
				t.Fatalf("%d GC goroutines while the storage is open, want %d", got, before+1)
			}

			if err := store.Close(ctx); err != nil {
				t.Fatalf("Close(): %v", err)
			}
			deadline := time.Now().Add(2 * time.Second)
			for badgerGCGoroutines() != before && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}
			if got := badgerGCGoroutines(); got != before {
				t.Errorf("%d GC goroutines after Close, want %d", got, before)
			}

			if err := store.Close(ctx); err != nil {
				t.Errorf("second Close(): %v", err)
			}
		})
	}
}

// Found by FuzzOperationsBadger: a key that is stored again is still one key.
func TestBadgerLenCountsAKeyThatIsStoredAgainOnce(t *testing.T) {
	ctx := context.Background()
	for name, opts := range map[string][]OptionFuncBadger{
		"in memory": nil,
		"on disk":   {WithMemoryStorage(false), WithTempDir(t.TempDir())},
	} {
		t.Run(name, func(t *testing.T) {
			store := NewMightyMapBadgerStorage[string, string](opts...)
			defer func() { _ = store.Close(ctx) }()
			store.Store(ctx, "a", "first")
			if got := store.Len(ctx); got != 1 {
				t.Fatalf("Len() = %d after one Store, want 1", got)
			}
			store.Store(ctx, "a", "second")
			store.Store(ctx, "b", "other")
			store.Store(ctx, "a", "third")
			if got := store.Len(ctx); got != 2 {
				t.Errorf("Len() = %d with two keys of which one was stored three times, want 2", got)
			}
			store.Delete(ctx, "a", "a", "missing")
			if got := store.Len(ctx); got != 1 {
				t.Errorf("Len() = %d after deleting one of two keys, want 1", got)
			}
		})
	}
}

// Found by FuzzMsgpackDecodeValue: a few bytes that claim to hold an array of
// four billion elements must be refused, not allocated for.
func TestMsgpackDecodeValueDoesNotTrustAClaimedLength(t *testing.T) {
	cases := map[string][]byte{
		"array as the value":    []byte("\x82\xa4type\xa3int\xa4data\xdd\xff\xff\xff\xff"),
		"array without wrapper": []byte("\xdd\xff\xff\xff\xff"),
		"array in a field":      []byte("\x82\xa4type\xa1x\xa4data\x81\xa5Sizes\xdd\xff\xff\xff\xff"),
		"data before its type":  []byte("\x82\xa4data\xdd\xff\xff\xff\xff\xa4type\xa3int"),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			start := time.Now()

			_, errSlice := msgpackDecodeValue[[]int](data)
			_, errItem := msgpackDecodeValue[roundTripItem](data)
			_, errAny := msgpackDecodeValue[any](data)

			runtime.ReadMemStats(&after)
			if errSlice == nil || errItem == nil || errAny == nil {
				t.Errorf("decoded without an error: []int %v, struct %v, any %v", errSlice, errItem, errAny)
			}
			if allocated := (after.TotalAlloc - before.TotalAlloc) >> 20; allocated > 16 {
				t.Errorf("allocated %d MB to decode %d bytes", allocated, len(data))
			}
			if took := time.Since(start); took > 2*time.Second {
				t.Errorf("took %s to decode %d bytes", took, len(data))
			}
		})
	}
}

// The length check must let through everything the encoder can write, and
// refuse every value that is cut off.
func TestMsgpackCheckLengths(t *testing.T) {
	long := strings.Repeat("x", 70000)
	values := []any{
		nil, true, false, 0, 127, -1, -33, 200, 70000, int64(5e9), -200, -70000, int64(-5e9),
		uint8(200), uint16(60000), uint32(4e9), uint64(1 << 63), float32(1.5), 1.5,
		"", "short", strings.Repeat("s", 40), strings.Repeat("s", 300), long,
		[]byte{}, []byte("bin"), []byte(strings.Repeat("b", 300)), []byte(long),
		[]int{1, 2, 3}, make([]int, 20), make([]string, 70000),
		map[string]int{"a": 1}, map[string]any{"a": []any{1, "b", nil}, "c": map[string]any{"d": 2.5}},
		time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC), newRoundTripItem(40),
	}
	manyKeys := map[int]int{}
	for i := range 20 {
		manyKeys[i] = i
	}
	values = append(values, manyKeys)

	for _, value := range values {
		encoded, err := msgpack.Marshal(value)
		if err != nil {
			t.Fatalf("%T: %v", value, err)
		}
		if err := msgpackCheckLengths(encoded); err != nil {
			t.Errorf("%T of %d bytes is refused: %v", value, len(encoded), err)
		}
		for _, cut := range []int{0, 1, len(encoded) / 2, len(encoded) - 1} {
			if cut < len(encoded) && msgpackCheckLengths(encoded[:cut]) == nil {
				t.Errorf("%T cut off at %d of %d bytes is accepted", value, cut, len(encoded))
			}
		}
		if err := msgpackCheckLengths(append(encoded, 0xc1, 0xff)); err != nil {
			t.Errorf("%T followed by other bytes is refused: %v", value, err)
		}
	}

	for name, tc := range map[string]struct {
		data  string
		valid bool
	}{
		"ext of 1 byte":               {"\xd4\x01\xaa", true},
		"ext of 16 bytes":             {"\xd8\x01" + strings.Repeat("\xaa", 16), true},
		"ext of 16 bytes, cut off":    {"\xd8\x01" + strings.Repeat("\xaa", 15), false},
		"ext with 1 byte length":      {"\xc7\x03\x01\xaa\xbb\xcc", true},
		"ext with 2 byte length":      {"\xc8\x00\x02\x01\xaa\xbb", true},
		"ext with 4 byte length":      {"\xc9\x00\x00\x00\x01\x01\xaa", true},
		"ext that claims 4 GB":        {"\xc9\xff\xff\xff\xff\x01\xaa", false},
		"length that is cut off":      {"\xdd\x00\x00", false},
		"byte MessagePack never uses": {"\xc1", false},
		"map that claims too much":    {"\xde\xff\xff\xa1a\x01", false},
		"string that claims 4 GB":     {"\xdb\xff\xff\xff\xffabc", false},
	} {
		if err := msgpackCheckLengths([]byte(tc.data)); (err == nil) != tc.valid {
			t.Errorf("%s: err = %v, want valid = %v", name, err, tc.valid)
		}
	}
}

// Found by FuzzOperationsDefault: the zero value of the key type is a key
// like any other, Next must hand it out too.
func TestNextReturnsTheZeroKey(t *testing.T) {
	ctx := context.Background()
	backends := map[string]func(t *testing.T) IMightyMapStorage[int, string]{
		"default": func(*testing.T) IMightyMapStorage[int, string] { return NewMightyMapDefaultStorage[int, string]() },
		"swiss":   func(*testing.T) IMightyMapStorage[int, string] { return NewMightyMapSwissStorage[int, string]() },
		"badger":  func(*testing.T) IMightyMapStorage[int, string] { return NewMightyMapBadgerStorage[int, string]() },
		"sqlite": func(*testing.T) IMightyMapStorage[int, string] {
			return NewMightyMapSQLiteStorage[int, string](WithSQLiteInMemory())
		},
		"redis": func(t *testing.T) IMightyMapStorage[int, string] {
			return NewMightyMapRedisStorage[int, string](WithRedisMock(t))
		},
		"segment files": func(t *testing.T) IMightyMapStorage[int, string] {
			return NewMightyMapSegmentStorage[int, string](WithSegmentDir(t.TempDir()))
		},
	}
	for name, open := range backends {
		t.Run(name, func(t *testing.T) {
			store := open(t)
			defer func() { _ = store.Close(ctx) }()
			store.Store(ctx, 0, "zero")

			key, value, ok := store.Next(ctx)
			if !ok || key != 0 || value != "zero" {
				t.Errorf("Next() = %d, %q, %v; want 0, zero, true", key, value, ok)
			}
			if got := store.Len(ctx); got != 0 {
				t.Errorf("Len() = %d after Next took the only value, want 0", got)
			}
			if _, _, ok := store.Next(ctx); ok {
				t.Error("Next() found a value in an empty storage")
			}
		})
	}
}
