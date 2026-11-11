package storage

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"
)

// newTestSegmentStorage opens a segment storage of any values in a directory
// of its own and closes it when the test ends.
func newTestSegmentStorage(t *testing.T, opts ...OptionFuncSegment) (IMightyMapStorage[string, any], string) {
	t.Helper()
	dir := t.TempDir()
	store := NewMightyMapSegmentStorage[string, any](append([]OptionFuncSegment{WithSegmentDir(dir)}, opts...)...)
	t.Cleanup(func() { _ = store.Close(context.Background()) })
	return store, dir
}

// newTestSegmentBytes opens the storage below the msgpack adapter, to look at
// what ends up in the files.
func newTestSegmentBytes(t *testing.T, segmentSize int64) (*segmentStorage[string], string) {
	t.Helper()
	dir := t.TempDir()
	store := newSegmentStorage[string](dir, segmentSize)
	t.Cleanup(func() { _ = store.Close(context.Background()) })
	return store, dir
}

func segmentFiles(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "mightymap-*.seg"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

// segmentValue is a value of size bytes that is different for every key.
func segmentValue(key string, size int) []byte {
	return bytes.Repeat([]byte(key+"|"), size/(len(key)+1)+1)[:size]
}

func TestSegmentStorageStoreAndLoad(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestSegmentStorage(t)
	item := newRoundTripItem(3)
	stored := map[string]any{
		"struct":  item,
		"pointer": &item,
		"slice":   []roundTripItem{item, item},
		"string":  "plain",
		"number":  42,
		"empty":   "",
	}
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
	if got, ok := store.Load(ctx, "missing"); ok || got != nil {
		t.Errorf("Load of a key that was never stored = %v, %v; want nil, false", got, ok)
	}
	if got := store.Len(ctx); got != len(stored) {
		t.Errorf("Len() = %d, want %d", got, len(stored))
	}
}

func TestSegmentStorageStoreReplacesTheValue(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestSegmentStorage(t)
	store.Store(ctx, "key", "old")
	store.Store(ctx, "other", "untouched")
	store.Store(ctx, "key", "new and longer than the old one")

	if got, ok := store.Load(ctx, "key"); !ok || got != "new and longer than the old one" {
		t.Errorf("Load() = %v, %v; want the value stored last", got, ok)
	}
	if got, _ := store.Load(ctx, "other"); got != "untouched" {
		t.Errorf("Load(other) = %v, want untouched", got)
	}
	if got := store.Len(ctx); got != 2 {
		t.Errorf("Len() = %d, want 2: a key that is stored twice counts once", got)
	}
}

func TestSegmentStorageDelete(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestSegmentStorage(t)
	for _, key := range []string{"a", "b", "c", "d"} {
		store.Store(ctx, key, "value of "+key)
	}

	store.Delete(ctx) // no keys: nothing happens
	store.Delete(ctx, "a", "c", "never stored", "a")

	for _, key := range []string{"a", "c"} {
		if got, ok := store.Load(ctx, key); ok {
			t.Errorf("Load(%q) = %v after Delete, want not found", key, got)
		}
	}
	for _, key := range []string{"b", "d"} {
		if got, ok := store.Load(ctx, key); !ok || got != "value of "+key {
			t.Errorf("Load(%q) = %v, %v; want it untouched", key, got, ok)
		}
	}
	if got := store.Len(ctx); got != 2 {
		t.Errorf("Len() = %d, want 2", got)
	}

	// A deleted key can be stored again.
	store.Store(ctx, "a", "back")
	if got, ok := store.Load(ctx, "a"); !ok || got != "back" {
		t.Errorf("Load(a) = %v, %v; want back", got, ok)
	}
}

func TestSegmentStorageRange(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestSegmentStorage(t)

	store.Range(ctx, func(key string, _ any) bool {
		t.Errorf("Range over an empty storage visited %q", key)
		return true
	})

	want := map[string]any{}
	for i := range 20 {
		key := fmt.Sprintf("key%02d", i)
		want[key] = i
		store.Store(ctx, key, i)
	}

	got := map[string]any{}
	store.Range(ctx, func(key string, value any) bool {
		if _, twice := got[key]; twice {
			t.Errorf("Range visited %q twice", key)
		}
		got[key] = value
		return true
	})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Range visited %v, want %v", got, want)
	}

	visited := 0
	store.Range(ctx, func(string, any) bool {
		visited++
		return visited < 3
	})
	if visited != 3 {
		t.Errorf("Range went on for %d values after the function returned false at 3", visited)
	}
}

// The storage may be changed from inside Range: a value that is deleted before
// Range gets to it is not visited.
func TestSegmentStorageRangeWhileChanging(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestSegmentStorage(t)
	keys := []string{"a", "b", "c", "d", "e"}
	for _, key := range keys {
		store.Store(ctx, key, key)
	}

	var visited []string
	store.Range(ctx, func(key string, _ any) bool {
		if len(visited) == 0 {
			for _, other := range keys {
				if other != key {
					store.Delete(ctx, other)
				}
			}
			store.Store(ctx, "new", "stored during Range")
		}
		visited = append(visited, key)
		return true
	})

	if len(visited) != 1 {
		t.Errorf("Range visited %v, want only the first value: the others were deleted", visited)
	}
	if got := store.Len(ctx); got != 2 {
		t.Errorf("Len() = %d, want 2", got)
	}
}

func TestSegmentStorageKeys(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestSegmentStorage(t)
	if got := store.Keys(ctx); len(got) != 0 {
		t.Errorf("Keys() of an empty storage = %v", got)
	}

	for _, key := range []string{"c", "a", "b", "a"} {
		store.Store(ctx, key, 1)
	}
	store.Delete(ctx, "b")

	got := store.Keys(ctx)
	sort.Strings(got)
	if want := []string{"a", "c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Keys() = %v, want %v", got, want)
	}
}

func TestSegmentStorageNext(t *testing.T) {
	ctx := context.Background()
	store, dir := newTestSegmentStorage(t, WithSegmentSize(64))
	if key, value, ok := store.Next(ctx); ok {
		t.Errorf("Next() on an empty storage = %q, %v", key, value)
	}

	want := map[string]any{}
	for i := range 10 {
		key := fmt.Sprintf("key%d", i)
		want[key] = fmt.Sprintf("value %d", i)
		store.Store(ctx, key, want[key])
	}

	got := map[string]any{}
	for range len(want) {
		key, value, ok := store.Next(ctx)
		if !ok {
			t.Fatalf("Next() stopped after %d of %d values", len(got), len(want))
		}
		if _, ok := store.Load(ctx, key); ok {
			t.Errorf("%q is still stored after Next() returned it", key)
		}
		got[key] = value
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Next() gave %v, want %v", got, want)
	}
	if _, _, ok := store.Next(ctx); ok {
		t.Error("Next() found a value after the storage was drained")
	}
	if got := store.Len(ctx); got != 0 {
		t.Errorf("Len() = %d after draining, want 0", got)
	}
	if files := segmentFiles(t, dir); len(files) > 1 {
		t.Errorf("%d segment files left after draining, want at most the current one", len(files))
	}
}

func TestSegmentStorageClear(t *testing.T) {
	ctx := context.Background()
	store, dir := newTestSegmentStorage(t, WithSegmentSize(64))
	store.Clear(ctx) // empty: nothing happens

	for i := range 10 {
		store.Store(ctx, fmt.Sprintf("key%d", i), "a value of some length")
	}
	if len(segmentFiles(t, dir)) < 2 {
		t.Fatal("test needs more than one segment file")
	}

	store.Clear(ctx)

	if got := store.Len(ctx); got != 0 {
		t.Errorf("Len() = %d after Clear, want 0", got)
	}
	if got := store.Keys(ctx); len(got) != 0 {
		t.Errorf("Keys() = %v after Clear", got)
	}
	if got, ok := store.Load(ctx, "key3"); ok {
		t.Errorf("Load() = %v after Clear, want not found", got)
	}
	if files := segmentFiles(t, dir); len(files) != 0 {
		t.Errorf("segment files left after Clear: %v", files)
	}

	// The storage is still usable.
	store.Store(ctx, "key", "after clear")
	if got, ok := store.Load(ctx, "key"); !ok || got != "after clear" {
		t.Errorf("Load() = %v, %v; want the value stored after Clear", got, ok)
	}
}

func TestSegmentStorageClose(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "segments")
	store := NewMightyMapSegmentStorage[string, any](WithSegmentDir(dir), WithSegmentSize(64))
	for i := range 10 {
		store.Store(ctx, fmt.Sprintf("key%d", i), "a value of some length")
	}

	if err := store.Close(ctx); err != nil {
		t.Errorf("Close() = %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the directory is still there after Close (stat: %v); its files cannot be opened again", err)
	}
	if err := store.Close(ctx); err != nil {
		t.Errorf("second Close() = %v", err)
	}
}

// Close only removes what the storage made: other files in the directory stay.
func TestSegmentStorageCloseKeepsOtherFiles(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	other := filepath.Join(dir, "not-ours.txt")
	if err := os.WriteFile(other, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewMightyMapSegmentStorage[string, any](WithSegmentDir(dir))
	store.Store(ctx, "key", "value")

	if err := store.Close(ctx); err != nil {
		t.Errorf("Close() = %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("Close removed a file that is not a segment: %v", err)
	}
	if files := segmentFiles(t, dir); len(files) != 0 {
		t.Errorf("segment files left after Close: %v", files)
	}
}

func TestSegmentStorageStartsANewFileWhenTheSegmentIsFull(t *testing.T) {
	ctx := context.Background()
	store, dir := newTestSegmentBytes(t, 1000)
	want := map[string][]byte{}
	for i := range 9 { // 400 bytes each: a segment is full after three
		key := fmt.Sprintf("key%d", i)
		want[key] = segmentValue(key, 400)
		store.Store(ctx, key, want[key])
	}

	files := segmentFiles(t, dir)
	if len(files) != 3 {
		t.Fatalf("%d segment files for 9 values of 400 bytes in segments of 1000, want 3", len(files))
	}
	for _, file := range files {
		if info, err := os.Stat(file); err != nil || info.Size() != 1200 {
			t.Errorf("%s: size %d, err %v; want 1200", filepath.Base(file), info.Size(), err)
		}
	}
	for key, value := range want {
		if got, ok := store.Load(ctx, key); !ok || !bytes.Equal(got, value) {
			t.Errorf("Load(%q) = %d bytes, %v; want the %d bytes that were stored", key, len(got), ok, len(value))
		}
	}
}

// What the storage is made for: a set of values that was stored in one go is
// deleted as a whole, and its files go without anything being rewritten.
func TestSegmentStorageRemovesTheFilesOfDeletedValues(t *testing.T) {
	ctx := context.Background()
	store, dir := newTestSegmentBytes(t, 1000)
	storeVersion := func(version string, count int) (keys []string) {
		for i := range count {
			key := fmt.Sprintf("%s:key%d", version, i)
			keys = append(keys, key)
			store.Store(ctx, key, segmentValue(key, 400))
		}
		return keys
	}

	v1 := storeVersion("v1", 10) // three full files and one value in the fourth
	v2 := storeVersion("v2", 8)  // fills the fourth and two more
	before := segmentFiles(t, dir)
	if len(before) != 6 {
		t.Fatalf("%d segment files, want 6", len(before))
	}
	contents := map[string][]byte{}
	for _, file := range before[3:] {
		contents[file], _ = os.ReadFile(file)
	}

	store.Delete(ctx, v1...)

	after := segmentFiles(t, dir)
	if !reflect.DeepEqual(after, before[3:]) {
		t.Errorf("files after deleting v1: %v, want the three that hold values of v2: %v", after, before[3:])
	}
	for file, want := range contents {
		if got, _ := os.ReadFile(file); !bytes.Equal(got, want) {
			t.Errorf("%s was rewritten", filepath.Base(file))
		}
	}
	for _, key := range v2 {
		if got, ok := store.Load(ctx, key); !ok || !bytes.Equal(got, segmentValue(key, 400)) {
			t.Errorf("Load(%q) is wrong after deleting v1", key)
		}
	}

	// The file that is being written to stays, also when nothing in it is
	// live; it goes once it is full.
	store.Delete(ctx, v2...)
	if files := segmentFiles(t, dir); len(files) != 1 {
		t.Errorf("%d segment files after deleting everything, want only the current one", len(files))
	}
	v3 := storeVersion("v3", 3)
	if files := segmentFiles(t, dir); len(files) != 1 || files[0] == before[5] {
		t.Errorf("files after storing on: %v; want one new file, the emptied one removed", files)
	}
	for _, key := range v3 {
		if _, ok := store.Load(ctx, key); !ok {
			t.Errorf("Load(%q) did not find the value", key)
		}
	}
}

// Replacing a value also frees the file of the old value.
func TestSegmentStorageRemovesTheFilesOfReplacedValues(t *testing.T) {
	ctx := context.Background()
	store, dir := newTestSegmentBytes(t, 1000)
	keys := []string{"a", "b", "c"}
	for _, key := range keys {
		store.Store(ctx, key, segmentValue(key, 400))
	}
	first := segmentFiles(t, dir)

	for _, key := range keys {
		store.Store(ctx, key, segmentValue(key+"2", 400))
	}

	if files := segmentFiles(t, dir); len(files) != 1 || files[0] == first[0] {
		t.Errorf("files after replacing every value: %v; want only the new file", files)
	}
	for _, key := range keys {
		if got, ok := store.Load(ctx, key); !ok || !bytes.Equal(got, segmentValue(key+"2", 400)) {
			t.Errorf("Load(%q) did not return the replaced value", key)
		}
	}
	if got := store.Len(ctx); got != 3 {
		t.Errorf("Len() = %d, want 3", got)
	}
}

func TestSegmentStorageValueLargerThanASegment(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestSegmentBytes(t, 100)
	big := segmentValue("big", 5000)
	store.Store(ctx, "small", []byte("x"))
	store.Store(ctx, "big", big)
	store.Store(ctx, "after", []byte("y"))

	if got, ok := store.Load(ctx, "big"); !ok || !bytes.Equal(got, big) {
		t.Errorf("Load(big) = %d bytes, %v; want all %d", len(got), ok, len(big))
	}
	if got, ok := store.Load(ctx, "after"); !ok || string(got) != "y" {
		t.Errorf("Load(after) = %q, %v", got, ok)
	}
}

func TestSegmentStorageEmptyValue(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestSegmentBytes(t, 100)
	store.Store(ctx, "empty", nil)

	got, ok := store.Load(ctx, "empty")
	if !ok || len(got) != 0 {
		t.Errorf("Load() = %v, %v; want an empty value that is found", got, ok)
	}
	if key, value, ok := store.Next(ctx); !ok || key != "empty" || len(value) != 0 {
		t.Errorf("Next() = %q, %v, %v", key, value, ok)
	}
}

// The index is in memory only, so segment files of an earlier run cannot be
// read again: they are removed when the storage opens. Nothing else is touched.
func TestSegmentStorageRemovesFilesOfAnEarlierRunOnOpen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	earlier := NewMightyMapSegmentStorage[string, any](WithSegmentDir(dir), WithSegmentSize(64))
	for i := range 10 {
		earlier.Store(ctx, fmt.Sprintf("key%d", i), "a value of some length")
	}
	left := segmentFiles(t, dir) // the earlier run ends without Close
	if len(left) < 2 {
		t.Fatal("test needs segment files of an earlier run")
	}
	other := filepath.Join(dir, "not-ours.seg")
	if err := os.WriteFile(other, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewMightyMapSegmentStorage[string, any](WithSegmentDir(dir))
	defer func() { _ = store.Close(ctx) }()

	if files := segmentFiles(t, dir); len(files) != 0 {
		t.Errorf("after opening: %v; want the files of the earlier run gone", files)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("opening removed a file that is not a segment: %v", err)
	}
	if got := store.Len(ctx); got != 0 {
		t.Errorf("Len() = %d, want 0: values of an earlier run are gone", got)
	}
	store.Store(ctx, "key", "value")
	if got, ok := store.Load(ctx, "key"); !ok || got != "value" {
		t.Errorf("Load() = %v, %v", got, ok)
	}
}

func TestSegmentStorageDefaults(t *testing.T) {
	ctx := context.Background()
	opts := getDefaultSegmentOptions()
	if opts.segmentSize != 256<<20 {
		t.Errorf("default segment size = %d, want 256 MB", opts.segmentSize)
	}
	if opts.valueCacheSize != 0 {
		t.Errorf("default value cache size = %d, want 0 (off)", opts.valueCacheSize)
	}

	// A size that makes no sense leaves the default in place.
	WithSegmentSize(0)(opts)
	WithSegmentSize(-1)(opts)
	if opts.segmentSize != 256<<20 {
		t.Errorf("segment size = %d after WithSegmentSize(0) and (-1), want the default", opts.segmentSize)
	}

	// Without a directory every storage makes one of its own in the temp
	// directory, and removes it again on Close.
	first := newSegmentStorage[string](opts.dir, opts.segmentSize)
	second := newSegmentStorage[string](opts.dir, opts.segmentSize)
	if filepath.Dir(first.dir) != filepath.Clean(os.TempDir()) {
		t.Errorf("default directory = %q, want one in %q", first.dir, os.TempDir())
	}
	if first.dir == second.dir {
		t.Errorf("two storages share the default directory %q", first.dir)
	}
	for _, store := range []*segmentStorage[string]{first, second} {
		if err := store.Close(ctx); err != nil {
			t.Errorf("Close() = %v", err)
		}
		if _, err := os.Stat(store.dir); !os.IsNotExist(err) {
			t.Errorf("default directory %q is still there after Close", store.dir)
		}
	}

	// Without options the storage works.
	store := NewMightyMapSegmentStorage[string, string]()
	store.Store(ctx, "key", "value")
	if got, ok := store.Load(ctx, "key"); !ok || got != "value" {
		t.Errorf("Load() = %q, %v", got, ok)
	}
	if err := store.Close(ctx); err != nil {
		t.Errorf("Close() = %v", err)
	}
}

func TestSegmentStoragePanicsWhenTheDirectoryCannotBeMade(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if recover() == nil {
			t.Error("no panic for a directory below a regular file")
		}
	}()
	NewMightyMapSegmentStorage[string, any](WithSegmentDir(filepath.Join(file, "segments")))
}

// The same when the next segment file cannot be made.
func TestSegmentStoragePanicsWhenASegmentFileCannotBeMade(t *testing.T) {
	ctx := context.Background()
	store, dir := newTestSegmentBytes(t, 1000)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("no panic for a segment file that could not be made")
			}
		}()
		store.Store(ctx, "lost", []byte("value"))
	}()
	if got := store.Len(ctx); got != 0 {
		t.Errorf("Len() = %d, want 0", got)
	}
}

// A value that cannot be written must not look stored; like the other
// storages this one panics.
func TestSegmentStoragePanicsWhenAValueCannotBeWritten(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestSegmentBytes(t, 1000)
	store.Store(ctx, "key", []byte("value"))
	_ = store.current.file.Close() // the disk goes away

	func() {
		defer func() {
			if recover() == nil {
				t.Error("no panic for a value that could not be written")
			}
		}()
		store.Store(ctx, "lost", []byte("value"))
	}()
	if _, ok := store.Load(ctx, "lost"); ok {
		t.Error("a value that could not be written is found")
	}
	if got := store.Len(ctx); got != 1 {
		t.Errorf("Len() = %d, want 1", got)
	}
}

// A value that cannot be read is not found. This is also what a Load sees
// that is overtaken by the removal of the file it was about to read.
func TestSegmentStorageValueThatCannotBeRead(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestSegmentBytes(t, 1000)
	store.Store(ctx, "a", []byte("value"))
	store.Store(ctx, "b", []byte("value"))
	_ = store.current.file.Close()

	if got, ok := store.Load(ctx, "a"); ok {
		t.Errorf("Load() = %q, want not found", got)
	}
	visited := 0
	store.Range(ctx, func(string, []byte) bool {
		visited++
		return true
	})
	if visited != 0 {
		t.Errorf("Range visited %d values that cannot be read", visited)
	}
	if key, _, ok := store.Next(ctx); ok {
		t.Errorf("Next() = %q, want nothing", key)
	}
	if got := store.Len(ctx); got != 0 {
		t.Errorf("Len() = %d, want 0: Next drops values it cannot read", got)
	}
}

func TestSegmentStorageValueCacheOption(t *testing.T) {
	ctx := context.Background()
	item := newRoundTripItem(3)
	sameObjectOnSecondLoad := func(t *testing.T, opts ...OptionFuncSegment) bool {
		t.Helper()
		store, _ := newTestSegmentStorage(t, opts...)
		store.Store(ctx, "item", &item)
		first, ok := store.Load(ctx, "item")
		if !ok || !reflect.DeepEqual(first, &item) {
			t.Fatalf("Load() = %+v, %v; want the stored item", first, ok)
		}
		second, _ := store.Load(ctx, "item")
		return first.(*roundTripItem) == second.(*roundTripItem)
	}

	if !sameObjectOnSecondLoad(t, WithSegmentValueCacheSize(1<<20)) {
		t.Error("with a value cache the second Load must return the value that is already in memory")
	}
	if sameObjectOnSecondLoad(t) {
		t.Error("without the option the storage must not cache values")
	}
	if sameObjectOnSecondLoad(t, WithSegmentValueCacheSize(0)) {
		t.Error("a cache size of 0 must leave the cache off")
	}
}

func TestSegmentStorageKeyTypes(t *testing.T) {
	ctx := context.Background()
	type pair struct {
		Domain string
		ID     int
	}

	ints := NewMightyMapSegmentStorage[int, string](WithSegmentDir(t.TempDir()))
	defer func() { _ = ints.Close(ctx) }()
	ints.Store(ctx, 1, "one")
	ints.Store(ctx, 2, "two")
	if got, ok := ints.Load(ctx, 2); !ok || got != "two" {
		t.Errorf("int key: Load() = %q, %v", got, ok)
	}

	pairs := NewMightyMapSegmentStorage[pair, []int](WithSegmentDir(t.TempDir()))
	defer func() { _ = pairs.Close(ctx) }()
	pairs.Store(ctx, pair{"nl", 1}, []int{1, 2, 3})
	if got, ok := pairs.Load(ctx, pair{"nl", 1}); !ok || !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Errorf("struct key: Load() = %v, %v", got, ok)
	}
	if _, ok := pairs.Load(ctx, pair{"nl", 2}); ok {
		t.Error("struct key: found a key that was never stored")
	}
}

// Everything at the same time, for the race detector: one version is loaded
// while the previous one is read and then deleted, as a cache does on a swap.
func TestSegmentStorageConcurrentUse(t *testing.T) {
	ctx := context.Background()
	store, dir := newTestSegmentBytes(t, 4096)
	const versions, perVersion, writers = 6, 400, 4
	key := func(version, i int) string { return fmt.Sprintf("v%d:key%d", version, i) }

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for range 4 { // readers that never see a wrong value
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				k := key(i%versions, i%perVersion)
				if got, ok := store.Load(ctx, k); ok && !bytes.Equal(got, segmentValue(k, 100)) {
					t.Errorf("Load(%q) returned the bytes of another value", k)
					return
				}
				if i%50 == 0 {
					store.Range(ctx, func(k string, got []byte) bool {
						if !bytes.Equal(got, segmentValue(k, 100)) {
							t.Errorf("Range gave %q the bytes of another value", k)
						}
						return false
					})
					store.Keys(ctx)
					store.Len(ctx)
				}
			}
		}()
	}

	for version := range versions {
		var load sync.WaitGroup
		for w := range writers {
			load.Add(1)
			go func() {
				defer load.Done()
				for i := w; i < perVersion; i += writers {
					k := key(version, i)
					store.Store(ctx, k, segmentValue(k, 100))
				}
			}()
		}
		load.Wait()
		if version > 0 {
			var old []string
			for i := range perVersion {
				old = append(old, key(version-1, i))
			}
			store.Delete(ctx, old...)
		}
	}
	close(stop)
	wg.Wait()

	if got := store.Len(ctx); got != perVersion {
		t.Errorf("Len() = %d, want the %d values of the last version", got, perVersion)
	}
	for i := range perVersion {
		k := key(versions-1, i)
		if got, ok := store.Load(ctx, k); !ok || !bytes.Equal(got, segmentValue(k, 100)) {
			t.Errorf("Load(%q) is wrong after the swaps", k)
		}
	}
	// 400 values of 100 bytes are 10 files of 4 KB; the five versions before
	// it must be gone apart from files they share with the last one.
	if files := segmentFiles(t, dir); len(files) > 12 {
		t.Errorf("%d segment files left for one live version, want about 10", len(files))
	}
}

func BenchmarkSegmentStorageStore(b *testing.B) {
	ctx := context.Background()
	store := NewMightyMapSegmentStorage[int, string](WithSegmentDir(b.TempDir()))
	defer func() { _ = store.Close(ctx) }()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		store.Store(ctx, i, "value")
	}
}

func BenchmarkSegmentStorageLoad(b *testing.B) {
	ctx := context.Background()
	store := NewMightyMapSegmentStorage[int, string](WithSegmentDir(b.TempDir()))
	defer func() { _ = store.Close(ctx) }()
	for i := range 100000 {
		store.Store(ctx, i, "value")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Load(ctx, i%100000)
	}
}

func BenchmarkSegmentStorageDelete(b *testing.B) {
	ctx := context.Background()
	store := NewMightyMapSegmentStorage[int, string](WithSegmentDir(b.TempDir()))
	defer func() { _ = store.Close(ctx) }()
	for i := range 100000 {
		store.Store(ctx, i, "value")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Delete(ctx, i%100000)
	}
}
