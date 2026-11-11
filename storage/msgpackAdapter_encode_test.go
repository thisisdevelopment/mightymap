package storage

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"runtime/debug"
	"sync"
	"testing"
	"unsafe"
)

// raceEnabled reports whether the test binary runs with the race detector.
// sync.Pool drops items at random there, so how much is allocated says nothing.
func raceEnabled() bool {
	info, _ := debug.ReadBuildInfo()
	for _, setting := range info.Settings {
		if setting.Key == "-race" {
			return setting.Value == "true"
		}
	}
	return false
}

// allocatedPerRun is the number of bytes f allocates, averaged over runs.
func allocatedPerRun(t *testing.T, runs int, f func()) int {
	t.Helper()
	if raceEnabled() {
		t.Skip("allocation sizes are not stable under the race detector")
	}
	f() // the first run fills the buffer pool
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range runs {
		f()
	}
	runtime.ReadMemStats(&after)
	return int(after.TotalAlloc-before.TotalAlloc) / runs
}

// Storages may keep the slice they are given, so it must not hold more memory
// than the value needs.
func TestMsgpackEncodeValueReturnsExactSize(t *testing.T) {
	big := make([]byte, 2<<20) // larger than the buffers that are kept for reuse
	for name, value := range map[string]any{
		"number":      42,
		"small item":  newRoundTripItem(3),
		"medium item": newRoundTripItem(120),
		"large item":  newRoundTripItem(5000),
		"huge value":  big,
	} {
		encoded, err := msgpackEncodeValue(value)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if cap(encoded) != len(encoded) {
			t.Errorf("%s: %d bytes encoded in a slice of %d", name, len(encoded), cap(encoded))
		}
		if got, err := msgpackDecodeValue[any](encoded); err != nil || !reflect.DeepEqual(got, value) {
			t.Errorf("%s: does not decode to the value that was encoded (err %v)", name, err)
		}
	}
}

func TestMsgpackEncodeValueAllocatesLittleMoreThanTheValue(t *testing.T) {
	var value any = newRoundTripItem(120)
	size := int(encodedSize(t, value))

	got := allocatedPerRun(t, 1000, func() {
		if _, err := msgpackEncodeValue(value); err != nil {
			t.Fatal(err)
		}
	})
	if limit := size + size/4 + 256; got > limit {
		t.Errorf("encoding a value of %d bytes allocates %d bytes, want at most %d", size, got, limit)
	}
}

// A storage that keeps the slices it is given must not see them change when
// the next value is encoded.
func TestMsgpackAdapterGivesEveryValueBytesOfItsOwn(t *testing.T) {
	ctx := context.Background()
	raw := newMockByteStorage[string]()
	adapter := newMsgpackAdapter[string, any](raw)

	first, second := newRoundTripItem(120), newRoundTripItem(120)
	second.Title = "another title of the same length!!"[:len(first.Title)]
	adapter.Store(ctx, "first", first)
	firstBytes, _ := raw.Load(ctx, "first")
	adapter.Store(ctx, "second", second)
	secondBytes, _ := raw.Load(ctx, "second")

	if unsafe.SliceData(firstBytes) == unsafe.SliceData(secondBytes) {
		t.Fatal("two stored values share their bytes")
	}
	if got, ok := adapter.Load(ctx, "first"); !ok || !reflect.DeepEqual(got, first) {
		t.Errorf("the first value changed when the second was stored: %+v", got)
	}
	if got, ok := adapter.Load(ctx, "second"); !ok || !reflect.DeepEqual(got, second) {
		t.Errorf("Load(second) = %+v", got)
	}
}

// The segment storage writes a value to its file and keeps nothing of it, so
// storing must not allocate a copy of the encoded value.
func TestSegmentStorageStoreDoesNotCopyTheValue(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestSegmentStorage(t)
	var value any = newRoundTripItem(120)
	size := int(encodedSize(t, value))

	got := allocatedPerRun(t, 1000, func() { store.Store(ctx, "key", value) })
	if limit := size / 2; got > limit {
		t.Errorf("storing a value of %d bytes allocates %d bytes, want less than %d", size, got, limit)
	}
	if loaded, ok := store.Load(ctx, "key"); !ok || !reflect.DeepEqual(loaded, value) {
		t.Error("the value does not load as it was stored")
	}
}

// Many goroutines store through one adapter and share its encode buffers.
func TestSegmentStorageConcurrentTypedStores(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestSegmentStorage(t, WithSegmentSize(64<<10))
	const writers, perWriter = 8, 200
	item := func(w, i int) roundTripItem {
		value := newRoundTripItem(1 + (w*perWriter+i)%40)
		value.ID = fmt.Sprintf("%d-%d", w, i)
		return value
	}

	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perWriter {
				store.Store(ctx, fmt.Sprintf("%d-%d", w, i), item(w, i))
			}
		}()
	}
	wg.Wait()

	for w := range writers {
		for i := range perWriter {
			if got, ok := store.Load(ctx, fmt.Sprintf("%d-%d", w, i)); !ok || !reflect.DeepEqual(got, item(w, i)) {
				t.Fatalf("value %d-%d does not load as it was stored", w, i)
			}
		}
	}
}

// Storing a cache entity of about 4.5 KB encoded, through the adapter.
func BenchmarkStoreItem(b *testing.B) {
	ctx := context.Background()
	var value any = newRoundTripItem(120)
	for name, open := range map[string]func() IMightyMapStorage[string, any]{
		"segment files": func() IMightyMapStorage[string, any] {
			return NewMightyMapSegmentStorage[string, any](WithSegmentDir(b.TempDir()))
		},
		"badger on disk": func() IMightyMapStorage[string, any] {
			return NewMightyMapBadgerStorage[string, any](WithMemoryStorage(false), WithTempDir(b.TempDir()))
		},
	} {
		b.Run(name, func(b *testing.B) {
			store := open()
			defer func() { _ = store.Close(ctx) }()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				store.Store(ctx, fmt.Sprintf("key%d", i%1000), value)
			}
		})
	}
}

func TestMsgpackEncodeValueReportsWhatItCannotEncode(t *testing.T) {
	if encoded, err := msgpackEncodeValue(make(chan int)); err == nil {
		t.Errorf("encoding a channel gave %d bytes and no error", len(encoded))
	}
}
