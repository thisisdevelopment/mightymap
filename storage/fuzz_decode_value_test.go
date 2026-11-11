package storage

import (
	"testing"
)

// FuzzMsgpackDecodeValue feeds arbitrary bytes to the decoder that reads
// values back from Badger, SQLite, Redis and the segment files. Those bytes
// may have been written by another version or another program, or be damaged.
//
// Invariant: decoding any bytes gives a value or an error and never panics,
// whatever type the caller asks for. A value that does decode can be stored
// again.
func FuzzMsgpackDecodeValue(f *testing.F) {
	item := newRoundTripItem(2)
	RegisterMsgpackType(item)
	for _, value := range []any{item, &item, []roundTripItem{item}, "text", 42, 1.5, true, []string{"a", "b"}, map[string]any{"a": 1}, nil} {
		encoded, err := msgpackEncodeValue(value)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(encoded)
		f.Add(encoded[:len(encoded)/2]) // cut off halfway
	}
	f.Add([]byte{})
	f.Add([]byte{0xc1})                                                 // a byte MessagePack never uses
	f.Add([]byte("\x82\xa4type\xa3int\xa4data\xdd\xff\xff\xff\xff"))    // an array that claims 4 billion elements
	f.Add([]byte("\x82\xa4type\xa6string\xa4data\xdb\xff\xff\xff\xff")) // a string that claims 4 GB
	f.Add([]byte("\x81\xa4data\xdf\xff\xff\xff\xff"))                   // a map that claims 4 billion entries

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = msgpackDecodeValue[roundTripItem](data)
		_, _ = msgpackDecodeValue[*roundTripItem](data)
		_, _ = msgpackDecodeValue[string](data)
		_, _ = msgpackDecodeValue[[]int](data)

		value, err := msgpackDecodeValue[any](data)
		if err != nil {
			return
		}
		if _, err := msgpackEncodeValue(value); err != nil {
			t.Fatalf("decoded to %T %#v, which cannot be encoded again: %v", value, value, err)
		}
	})
}
