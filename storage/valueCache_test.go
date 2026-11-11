package storage

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

// loadCountingStorage counts how often values are read from the byte storage,
// which is what the value cache is there to avoid.
type loadCountingStorage[K comparable] struct {
	*mockByteStorage[K]
	loads atomic.Int64
}

func (s *loadCountingStorage[K]) Load(ctx context.Context, key K) ([]byte, bool) {
	s.loads.Add(1)
	return s.mockByteStorage.Load(ctx, key)
}

func newCachedTestAdapter(maxBytes int64) (*msgpackAdapter[string, any], *loadCountingStorage[string]) {
	raw := &loadCountingStorage[string]{mockByteStorage: newMockByteStorage[string]()}
	adapter := newMsgpackAdapter[string, any](raw)
	adapter.cache = newValueCache[string, any](maxBytes)
	return adapter, raw
}

// encodedSize is what one stored value counts for in the cache.
func encodedSize(t *testing.T, value any) int64 {
	t.Helper()
	encoded, err := msgpackEncodeValue(value)
	if err != nil {
		t.Fatal(err)
	}
	return int64(len(encoded))
}

func TestValueCacheServesRepeatedLoadsFromMemory(t *testing.T) {
	ctx := context.Background()
	adapter, raw := newCachedTestAdapter(1 << 20)
	item := newRoundTripItem(3)
	adapter.Store(ctx, "item", &item)

	first, ok := adapter.Load(ctx, "item")
	if !ok || !reflect.DeepEqual(first, &item) {
		t.Fatalf("Load() = %+v, %v; want the stored item", first, ok)
	}
	for range 5 {
		again, ok := adapter.Load(ctx, "item")
		if !ok || again.(*roundTripItem) != first.(*roundTripItem) {
			t.Fatalf("repeated Load() did not return the value that is already in memory")
		}
	}
	if got := raw.loads.Load(); got != 1 {
		t.Errorf("storage was read %d times for 6 loads of one key, want 1", got)
	}

	if _, ok := adapter.Load(ctx, "missing"); ok {
		t.Error("Load() found a key that was never stored")
	}
}

// Without a cache every Load reads and decodes again.
func TestNoValueCacheByDefault(t *testing.T) {
	ctx := context.Background()
	raw := &loadCountingStorage[string]{mockByteStorage: newMockByteStorage[string]()}
	adapter := newMsgpackAdapter[string, any](raw)
	item := newRoundTripItem(3)
	adapter.Store(ctx, "item", &item)

	first, _ := adapter.Load(ctx, "item")
	second, _ := adapter.Load(ctx, "item")
	if first.(*roundTripItem) == second.(*roundTripItem) {
		t.Error("two loads returned the same object although no cache is configured")
	}
	if got := raw.loads.Load(); got != 2 {
		t.Errorf("storage was read %d times for 2 loads, want 2", got)
	}
}

func TestValueCacheIsBoundedAndDropsTheLeastRecentlyUsed(t *testing.T) {
	ctx := context.Background()
	value := newRoundTripItem(3)
	size := encodedSize(t, value)
	adapter, raw := newCachedTestAdapter(2*size + size/2) // room for two values
	for _, key := range []string{"a", "b", "c"} {
		adapter.Store(ctx, key, value)
	}
	loadsFor := func(key string) int64 {
		t.Helper()
		before := raw.loads.Load()
		if _, ok := adapter.Load(ctx, key); !ok {
			t.Fatalf("Load(%q) did not find the value", key)
		}
		return raw.loads.Load() - before
	}

	loadsFor("a")
	loadsFor("b")
	loadsFor("a") // a is now used more recently than b
	loadsFor("c") // does not fit next to a and b: b has to go

	if adapter.cache.bytes > adapter.cache.maxBytes {
		t.Errorf("cache holds %d bytes, more than its limit of %d", adapter.cache.bytes, adapter.cache.maxBytes)
	}
	if got := loadsFor("a"); got != 0 {
		t.Errorf("a was read from storage %d times, want 0: it was used recently", got)
	}
	if got := loadsFor("c"); got != 0 {
		t.Errorf("c was read from storage %d times, want 0: it was just loaded", got)
	}
	if got := loadsFor("b"); got != 1 {
		t.Errorf("b was read from storage %d times, want 1: it was the least recently used", got)
	}
}

func TestValueCacheSkipsValuesLargerThanItsLimit(t *testing.T) {
	ctx := context.Background()
	big := newRoundTripItem(200)
	adapter, raw := newCachedTestAdapter(encodedSize(t, big) / 2)
	adapter.Store(ctx, "big", big)

	for range 3 {
		if got, ok := adapter.Load(ctx, "big"); !ok || !reflect.DeepEqual(got, big) {
			t.Fatalf("Load() = %v, want the stored value", ok)
		}
	}
	if got := raw.loads.Load(); got != 3 {
		t.Errorf("storage was read %d times, want 3: the value does not fit in the cache", got)
	}
	if adapter.cache.bytes != 0 {
		t.Errorf("cache holds %d bytes, want 0", adapter.cache.bytes)
	}
}

// Every way to change the storage must drop what the cache holds for that key.
func TestValueCacheNeverServesAReplacedOrRemovedValue(t *testing.T) {
	ctx := context.Background()
	adapter, _ := newCachedTestAdapter(1 << 20)
	load := func(key string) (any, bool) {
		value, ok := adapter.Load(ctx, key)
		return value, ok
	}

	adapter.Store(ctx, "key", "old")
	load("key")
	adapter.Store(ctx, "key", "new")
	if got, ok := load("key"); !ok || got != "new" {
		t.Errorf("after Store: Load() = %v, %v; want new", got, ok)
	}

	adapter.Delete(ctx, "key")
	if got, ok := load("key"); ok {
		t.Errorf("after Delete: Load() = %v, want not found", got)
	}

	adapter.Store(ctx, "key", "again")
	load("key")
	if _, _, ok := adapter.Next(ctx); !ok {
		t.Fatal("Next() found nothing")
	}
	if got, ok := load("key"); ok {
		t.Errorf("after Next: Load() = %v, want not found", got)
	}

	adapter.Store(ctx, "key", "once more")
	load("key")
	adapter.Clear(ctx)
	if got, ok := load("key"); ok {
		t.Errorf("after Clear: Load() = %v, want not found", got)
	}
	if adapter.cache.bytes != 0 || len(adapter.cache.entries) != 0 {
		t.Errorf("cache still holds %d bytes in %d entries after Clear", adapter.cache.bytes, len(adapter.cache.entries))
	}
}

// Range walks the whole storage; putting all of that in the cache would push
// the hot values out.
func TestValueCacheIsNotFilledByRange(t *testing.T) {
	ctx := context.Background()
	adapter, _ := newCachedTestAdapter(1 << 20)
	for i := range 10 {
		adapter.Store(ctx, fmt.Sprintf("key%d", i), i)
	}
	seen := 0
	adapter.Range(ctx, func(string, any) bool {
		seen++
		return true
	})
	if seen != 10 {
		t.Fatalf("Range visited %d values, want 10", seen)
	}
	if len(adapter.cache.entries) != 0 {
		t.Errorf("cache holds %d entries after a Range, want 0", len(adapter.cache.entries))
	}
}

// storeDuringLoad stores a new value for a key right after its old bytes were
// read from the storage: the moment at which a careless cache keeps the old
// value forever.
type storeDuringLoad struct {
	*mockByteStorage[string]
	afterLoad func()
}

func (s *storeDuringLoad) Load(ctx context.Context, key string) ([]byte, bool) {
	data, ok := s.mockByteStorage.Load(ctx, key)
	if hook := s.afterLoad; hook != nil {
		s.afterLoad = nil
		hook()
	}
	return data, ok
}

func TestValueCacheDoesNotKeepAValueThatWasReplacedWhileLoading(t *testing.T) {
	ctx := context.Background()
	raw := &storeDuringLoad{mockByteStorage: newMockByteStorage[string]()}
	adapter := newMsgpackAdapter[string, any](raw)
	adapter.cache = newValueCache[string, any](1 << 20)

	adapter.Store(ctx, "key", "old")
	raw.afterLoad = func() { adapter.Store(ctx, "key", "new") }
	adapter.Load(ctx, "key") // reads "old" while "new" is being stored

	if got, ok := adapter.Load(ctx, "key"); !ok || got != "new" {
		t.Errorf("Load() = %v, %v; want new: the value read before the Store must not be cached", got, ok)
	}

	// The same for a key that is deleted while it is being loaded.
	raw.afterLoad = func() { adapter.Delete(ctx, "key") }
	adapter.cache.clear()
	adapter.Load(ctx, "key")
	if got, ok := adapter.Load(ctx, "key"); ok {
		t.Errorf("Load() = %v after a Delete during the load, want not found", got)
	}
}

// Readers and writers at the same time, for the race detector. When the
// writers are done a Load must return the value that is in the storage.
func TestValueCacheWithConcurrentWriters(t *testing.T) {
	ctx := context.Background()
	adapter, raw := newCachedTestAdapter(1 << 20)
	keys := []string{"a", "b", "c", "d"}
	const rounds = 2000

	var wg sync.WaitGroup
	for _, key := range keys {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := range rounds {
				adapter.Store(ctx, key, i)
				if i%7 == 0 {
					adapter.Delete(ctx, key)
				}
			}
			adapter.Store(ctx, key, rounds)
		}()
		go func() {
			defer wg.Done()
			for range rounds {
				adapter.Load(ctx, key)
			}
		}()
	}
	wg.Wait()

	for _, key := range keys {
		for range 2 { // the second Load comes from the cache
			got, ok := adapter.Load(ctx, key)
			if !ok || got != rounds {
				t.Errorf("Load(%q) = %v, %v; want %d", key, got, ok, rounds)
			}
		}
	}
	if adapter.cache.bytes > adapter.cache.maxBytes {
		t.Errorf("cache holds %d bytes, more than its limit", adapter.cache.bytes)
	}
	_ = raw
}

// The option on the Badger storage switches the cache on; without it the
// storage behaves as before.
func TestBadgerValueCacheOption(t *testing.T) {
	ctx := context.Background()
	item := newRoundTripItem(3)
	sameObjectOnSecondLoad := func(t *testing.T, opts ...OptionFuncBadger) bool {
		t.Helper()
		store := NewMightyMapBadgerStorage[string, any](opts...)
		defer store.Close(ctx)
		store.Store(ctx, "item", &item)
		first, ok := store.Load(ctx, "item")
		if !ok || !reflect.DeepEqual(first, &item) {
			t.Fatalf("Load() = %+v, %v; want the stored item", first, ok)
		}
		second, _ := store.Load(ctx, "item")
		return first.(*roundTripItem) == second.(*roundTripItem)
	}

	if !sameObjectOnSecondLoad(t, WithValueCacheSize(1<<20)) {
		t.Error("with a value cache the second Load must return the value that is already in memory")
	}
	if sameObjectOnSecondLoad(t) {
		t.Error("without the option the storage must not cache values")
	}
	if sameObjectOnSecondLoad(t, WithValueCacheSize(0)) {
		t.Error("a cache size of 0 must leave the cache off")
	}
}

// Reading a hot value: straight from Badger against out of the value cache.
func BenchmarkBadgerLoadHotValue(b *testing.B) {
	ctx := context.Background()
	item := newRoundTripItem(120)
	for name, opts := range map[string][]OptionFuncBadger{
		"no cache":    nil,
		"value cache": {WithValueCacheSize(64 << 20)},
	} {
		store := NewMightyMapBadgerStorage[string, any](opts...)
		for i := range 100 {
			store.Store(ctx, fmt.Sprintf("key%d", i), &item)
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, ok := store.Load(ctx, fmt.Sprintf("key%d", i%100)); !ok {
					b.Fatal("value not found")
				}
			}
		})
		_ = store.Close(ctx)
	}
}
