package storage

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"testing"

	msgpack "github.com/vmihailenco/msgpack/v5"
)

type roundTripLabel struct {
	Key   string
	Value string
}

// roundTripItem looks like the entities a cache keeps: strings, nested slices,
// a map and a pointer to another struct.
type roundTripItem struct {
	ID      string
	Title   string
	Brand   string
	Private bool
	Sizes   []float64
	Labels  []roundTripLabel
	Attrs   map[string]string
	Parent  *roundTripItem
}

func newRoundTripItem(labels int) roundTripItem {
	item := roundTripItem{
		ID:      "5083739",
		Title:   "Dulux Paint Mixing Gloss 500ml",
		Brand:   "Dulux",
		Private: true,
		Sizes:   []float64{0.5, 1, 2.5, 5, 10},
		Attrs:   map[string]string{"finish": "gloss", "base": "water", "unit": "LTR"},
		Parent:  &roundTripItem{ID: "9e74f0ff-77e6-4ad5-85bb-a37f008e70fa", Title: "Dulux Colour Tester"},
	}
	for i := range labels {
		item.Labels = append(item.Labels, roundTripLabel{Key: fmt.Sprintf("label-%d", i), Value: "Not_Translated"})
	}
	return item
}

// A value stored in a map of any must come back as the type it was stored as.
// Pointers used to come back as untyped maps.
func TestMsgpackRoundTripKeepsType(t *testing.T) {
	item := newRoundTripItem(3)
	itemPtr := &item
	cases := map[string]any{
		"struct":                     item,
		"pointer to struct":          &item,
		"pointer to pointer":         &itemPtr,
		"slice of structs":           []roundTripItem{item, item},
		"slice of pointers":          []*roundTripItem{&item},
		"map of structs":             map[string]roundTripItem{"a": item},
		"int":                        42,
		"string":                     "hello",
		"pointer to string":          &item.Title,
		"empty struct pointer":       &roundTripItem{},
		"slice of strings":           []string{"a", "b"},
		"pointer to slice of struct": &[]roundTripItem{item},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			encoded, err := msgpackEncodeValue(want)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			got, err := msgpackDecodeValue[any](encoded)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got %T %+v, want %T %+v", got, got, want, want)
			}
		})
	}
}

// The same through the adapter, the way a storage backend uses it.
func TestMsgpackAdapterRoundTripsPointers(t *testing.T) {
	ctx := context.Background()
	adapter := newMsgpackAdapter[string, any](newMockByteStorage[string]())
	item := newRoundTripItem(3)
	adapter.Store(ctx, "item", &item)

	check := func(from string, got any) {
		t.Helper()
		ptr, ok := got.(*roundTripItem)
		if !ok {
			t.Fatalf("%s returned %T, want *roundTripItem", from, got)
		}
		if !reflect.DeepEqual(*ptr, item) {
			t.Errorf("%s returned %+v, want %+v", from, *ptr, item)
		}
	}

	got, ok := adapter.Load(ctx, "item")
	if !ok {
		t.Fatal("Load did not find the item")
	}
	check("Load", got)
	adapter.Range(ctx, func(_ string, value any) bool {
		check("Range", value)
		return true
	})
	_, got, ok = adapter.Next(ctx)
	if !ok {
		t.Fatal("Next did not find the item")
	}
	check("Next", got)
}

// A typed map decodes straight into its value type, pointer or not.
func TestMsgpackRoundTripTypedValues(t *testing.T) {
	item := newRoundTripItem(3)

	encoded, err := msgpackEncodeValue(&item)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	gotPtr, err := msgpackDecodeValue[*roundTripItem](encoded)
	if err != nil || !reflect.DeepEqual(gotPtr, &item) {
		t.Errorf("pointer: got %+v, err %v", gotPtr, err)
	}

	encoded, err = msgpackEncodeValue(item)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	gotValue, err := msgpackDecodeValue[roundTripItem](encoded)
	if err != nil || !reflect.DeepEqual(gotValue, item) {
		t.Errorf("value: got %+v, err %v", gotValue, err)
	}
}

// Data written by earlier versions must stay readable: the wrapper is a map
// with "data" and "type" in any order, and "type" names the element type of a
// pointer with a leading star.
func TestMsgpackDecodeValue_EarlierEncodings(t *testing.T) {
	item := newRoundTripItem(3)
	RegisterMsgpackType(item)

	wrapper := func(parts ...any) []byte {
		var buf bytes.Buffer
		enc := msgpack.NewEncoder(&buf)
		if err := enc.EncodeMapLen(len(parts) / 2); err != nil {
			t.Fatal(err)
		}
		for _, part := range parts {
			if err := enc.Encode(part); err != nil {
				t.Fatal(err)
			}
		}
		return buf.Bytes()
	}
	typeName := reflect.TypeOf(item).String()

	cases := map[string]struct {
		data []byte
		want any
	}{
		"data before type":          {wrapper("data", item, "type", typeName), item},
		"type before data":          {wrapper("type", typeName, "data", item), item},
		"pointer, data before type": {wrapper("data", &item, "type", "*"+typeName), &item},
		"no type":                   {wrapper("data", "plain"), "plain"},
		"unknown type":              {wrapper("type", "gone.Type", "data", "plain"), "plain"},
		"extra field":               {wrapper("version", 2, "type", typeName, "data", item), item},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := msgpackDecodeValue[any](tc.data)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %T %+v, want %T %+v", got, got, tc.want, tc.want)
			}
		})
	}

	// Without a "data" field the bytes are the value itself.
	plain, err := msgpack.Marshal(map[string]int{"a": 1})
	if err != nil {
		t.Fatal(err)
	}
	got, err := msgpackDecodeValue[map[string]int](plain)
	if err != nil || !reflect.DeepEqual(got, map[string]int{"a": 1}) {
		t.Errorf("unwrapped value: got %v, err %v", got, err)
	}
}

// One read of a cache entity out of a map of any: a struct of about 4 KB encoded.
func BenchmarkMsgpackDecodeValue(b *testing.B) {
	item := newRoundTripItem(120)
	for name, value := range map[string]any{"struct": item, "pointer": &item} {
		encoded, err := msgpackEncodeValue(value)
		if err != nil {
			b.Fatal(err)
		}
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

func BenchmarkMsgpackEncodeValue(b *testing.B) {
	var value any = newRoundTripItem(120)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := msgpackEncodeValue(value); err != nil {
			b.Fatal(err)
		}
	}
}
