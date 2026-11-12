package storage

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"testing"
)

// With compact structs a value comes back as what it was stored as, like
// without the option.
func TestSegmentCompactStructsKeepValuesAndTypes(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestSegmentStorage(t, WithSegmentCompactStructs())

	item := newRoundTripItem(3)
	values := map[string]any{
		"struct":            item,
		"pointer":           &item,
		"slice of structs":  []roundTripItem{item, item},
		"slice of pointers": []*roundTripItem{&item},
		"map of structs":    map[string]roundTripItem{"a": item},
		"empty struct":      roundTripItem{},
		"string":            "hello",
		"int":               42,
		"nil":               nil,
	}
	for key, value := range values {
		store.Store(ctx, key, value)
	}
	for key, want := range values {
		got, ok := store.Load(ctx, key)
		if !ok {
			t.Errorf("%s: not found", key)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %T %#v, want %T %#v", key, got, got, want, want)
		}
	}
}

// The point of the option: the names of the fields are not in the files.
func TestSegmentCompactStructsLeaveTheFieldNamesOut(t *testing.T) {
	ctx := context.Background()
	item := newRoundTripItem(3)

	written := func(opts ...OptionFuncSegment) []byte {
		store, dir := newTestSegmentStorage(t, opts...)
		store.Store(ctx, "a", item)
		files := segmentFiles(t, dir)
		if len(files) != 1 {
			t.Fatalf("got %d segment files, want 1", len(files))
		}
		data, err := os.ReadFile(files[0])
		if err != nil {
			t.Fatal(err)
		}
		return data
	}

	withNames := written()
	compact := written(WithSegmentCompactStructs())

	if !bytes.Contains(withNames, []byte("Title")) {
		t.Fatal("without the option the field names are expected in the file")
	}
	for _, name := range []string{"Title", "Brand", "Labels", "Value"} {
		if bytes.Contains(compact, []byte(name)) {
			t.Errorf("field name %q is in the file", name)
		}
	}
	if !bytes.Contains(compact, []byte(item.Title)) {
		t.Error("the value of a field is missing from the file")
	}
	if len(compact) >= len(withNames) {
		t.Errorf("compact is %d bytes, with names %d bytes", len(compact), len(withNames))
	}
}

// Both encodings are read by the same decoder, whatever a storage writes.
func TestMsgpackDecodeValueReadsStructsWithAndWithoutFieldNames(t *testing.T) {
	item := newRoundTripItem(3)
	for _, compact := range []bool{false, true} {
		var buf bytes.Buffer
		if err := msgpackEncodeInto[any](&buf, &item, compact); err != nil {
			t.Fatal(err)
		}
		got, err := msgpackDecodeValue[any](buf.Bytes())
		if err != nil {
			t.Fatalf("compact %v: %v", compact, err)
		}
		if !reflect.DeepEqual(got, &item) {
			t.Errorf("compact %v: got %#v, want %#v", compact, got, &item)
		}

		typed, err := msgpackDecodeValue[*roundTripItem](buf.Bytes())
		if err != nil || !reflect.DeepEqual(typed, &item) {
			t.Errorf("compact %v, typed: got %#v, err %v", compact, typed, err)
		}
	}
}

// The option is per storage: another storage in the same process keeps
// writing the field names.
func TestSegmentCompactStructsIsPerStorage(t *testing.T) {
	ctx := context.Background()
	compact, _ := newTestSegmentStorage(t, WithSegmentCompactStructs())
	plain, plainDir := newTestSegmentStorage(t)

	item := newRoundTripItem(1)
	compact.Store(ctx, "a", item)
	plain.Store(ctx, "a", item)

	data, err := os.ReadFile(segmentFiles(t, plainDir)[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("Title")) {
		t.Error("the storage without the option lost its field names")
	}
}

// One read of a cache entity out of a map of any, with and without the names
// of its fields in the encoded value.
func BenchmarkMsgpackDecodeValueCompact(b *testing.B) {
	var value any = newRoundTripItem(120)
	for name, compact := range map[string]bool{"names": false, "compact": true} {
		var buf bytes.Buffer
		if err := msgpackEncodeInto(&buf, value, compact); err != nil {
			b.Fatal(err)
		}
		encoded := buf.Bytes()
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(encoded)))
			for i := 0; i < b.N; i++ {
				got, err := msgpackDecodeValue[any](encoded)
				if err != nil || got == nil {
					b.Fatal(got, err)
				}
			}
		})
	}
}
