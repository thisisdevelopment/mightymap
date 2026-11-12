package storage

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
)

// FuzzMsgpackRoundTrip stores values the way Badger, SQLite, Redis and the
// segment files do: encoded with MessagePack next to the name of their type.
//
// Invariant: what is read back equals what was stored and has the same type,
// whether it went in as a struct, a pointer, a slice or a plain value, and
// whatever bytes its strings hold, with and without the names of the struct
// fields in the encoding.
func FuzzMsgpackRoundTrip(f *testing.F) {
	f.Add("5083739", "Dulux Gloss 500ml", true, 2.5, int64(3))
	f.Add("", "", false, 0.0, int64(0))
	f.Add("\x00", "\xff\xfe", true, -1.0, int64(-9))
	f.Add("ключ", "a\nb", false, 1e308, int64(1<<62))

	f.Fuzz(func(t *testing.T, id, title string, private bool, size float64, count int64) {
		if size != size {
			size = 0 // NaN never equals itself
		}
		item := roundTripItem{
			ID:      id,
			Title:   title,
			Private: private,
			Sizes:   []float64{size, -size},
			Attrs:   map[string]string{id: title, title: id},
		}
		for i := range uint64(count) % 4 {
			item.Labels = append(item.Labels, roundTripLabel{Key: fmt.Sprint(i), Value: title})
		}
		if private {
			item.Parent = &roundTripItem{ID: title}
		}

		for name, want := range map[string]any{
			"struct":  item,
			"pointer": &item,
			"slice":   []roundTripItem{item, item},
			"map":     map[string]roundTripItem{id: item},
			"string":  title,
			"int":     count,
			"float":   size,
			"bool":    private,
		} {
			// with the names of the struct fields, and without them as the
			// segment storage can write them
			for _, compact := range []bool{false, true} {
				var buf bytes.Buffer
				if err := msgpackEncodeInto(&buf, want, compact); err != nil {
					t.Fatalf("%s: encode: %v", name, err)
				}
				got, err := msgpackDecodeValue[any](buf.Bytes())
				if err != nil {
					t.Fatalf("%s, compact %v: decode: %v", name, compact, err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("%s, compact %v: got %T %#v, want %T %#v", name, compact, got, got, want, want)
				}
			}
		}

		encoded, err := msgpackEncodeValue(item)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := msgpackDecodeValue[roundTripItem](encoded); err != nil || !reflect.DeepEqual(got, item) {
			t.Fatalf("typed: got %#v, err %v; want %#v", got, err, item)
		}
	})
}
