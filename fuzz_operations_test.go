package mightymap_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/thisisdevelopment/mightymap"
	"github.com/thisisdevelopment/mightymap/storage"
)

// The FuzzOperations targets, one per storage, share this file.
//
// Invariant: whatever sequence of Store, Load, Has, Delete, Pop, Next, Len,
// Keys, Range and Clear a caller runs, with whatever keys and values, a Map
// answers exactly like a plain Go map that had the same things done to it.
//
// The fuzz input is read as that sequence. Keys are a mix of awkward fixed
// ones (empty, NUL bytes, invalid UTF-8, text that means something to SQL or
// to a Redis key pattern) and raw bytes, so the same key comes back often
// enough to hit overwrites and deletes.

var fuzzKeys = []string{
	"", "a", "b", "key", "KEY", "a b", "\x00", "a\x00b", "ключ", "\xff\xfe",
	"'; DROP TABLE kv;--", "*", "a*", "[a-z]", "?", "mightymap:", "\n", "%", "_", `\`, `"`,
	strings.Repeat("k", 300),
}

const fuzzMaxOperations = 200

// fuzzInput hands out the bytes of one fuzz input.
type fuzzInput struct{ data []byte }

func (in *fuzzInput) next() byte {
	if len(in.data) == 0 {
		return 0
	}
	b := in.data[0]
	in.data = in.data[1:]
	return b
}

func (in *fuzzInput) take(n int) string {
	n = min(n, len(in.data))
	s := string(in.data[:n])
	in.data = in.data[n:]
	return s
}

func (in *fuzzInput) key() string {
	b := in.next()
	if b&0x80 == 0 {
		return fuzzKeys[int(b)%len(fuzzKeys)]
	}
	return in.take(int(b & 0x0f))
}

func (in *fuzzInput) value() string { return in.take(int(in.next()) % 48) }

// fuzzOperations runs the target for one storage. The storage is opened once
// and used for every input; an input starts by emptying it.
func fuzzOperations(f *testing.F, store storage.IMightyMapStorage[string, string]) { //nolint:gocyclo // one case per operation
	f.Cleanup(func() { _ = store.Close(context.Background()) })

	// First byte: bit 0 allows overwrites. Then per operation its number,
	// keys as an index in fuzzKeys, values as a length and that many bytes.
	for _, seed := range []string{
		"",
		"\x01\x00\x01\x03one\x02\x01\x08\x09", // store, load, len, keys
		"\x00\x00\x01\x01x\x00\x01\x01y\x02\x01\x08",                               // no overwrite: the first value stays
		"\x01\x00\x01\x01x\x00\x01\x01y\x02\x01\x08",                               // overwrite: stored twice, counted once
		"\x01\x00\x01\x01x\x00\x02\x01y\x07\x07\x07\x08",                           // next until the map is empty
		"\x01\x00\x00\x00\x02\x00\x03\x00\x08\x09\x0a",                             // empty key with an empty value
		"\x01\x00\x03\x02ab\x06\x03\x03\x03\x04\x03\x05\x01\x02\x08",               // pop, has, delete of missing keys
		"\x01\x00\x09\x01z\x00\x0a\x01q\x00\x0b\x01s\x00\x06\x01n\x09\x0a\x0b\x08", // awkward keys, range, clear
		"\x01\x00\x83raw\x02hi\x02\x83raw\x08",                                     // a key of raw bytes
		"\x01\x00\x15\x05value\x02\x15\x06\x15\x08",                                // a key of 300 bytes
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := context.Background()
		in := &fuzzInput{data: data}
		overwrite := in.next()&1 == 1
		m := mightymap.New[string, string](overwrite, store)

		m.Delete(ctx, m.Keys(ctx)...)
		if n := m.Len(ctx); n != 0 {
			t.Fatalf("%d values left after deleting every key", n)
		}
		want := map[string]string{}

		for step := 0; len(in.data) > 0 && step < fuzzMaxOperations; step++ {
			switch op := in.next() % 12; op {
			case 0, 1: // Store, twice as often as the rest
				key, value := in.key(), in.value()
				m.Store(ctx, key, value)
				if _, exists := want[key]; overwrite || !exists {
					want[key] = value
				}
			case 2:
				key := in.key()
				got, ok := m.Load(ctx, key)
				if wantValue, wantOK := want[key]; ok != wantOK || got != wantValue {
					t.Fatalf("step %d: Load(%q) = %q, %v; want %q, %v", step, key, got, ok, wantValue, wantOK)
				}
			case 3:
				key := in.key()
				if _, wantOK := want[key]; m.Has(ctx, key) != wantOK {
					t.Fatalf("step %d: Has(%q) = %v, want %v", step, key, !wantOK, wantOK)
				}
			case 4:
				key := in.key()
				m.Delete(ctx, key)
				delete(want, key)
			case 5:
				first, second := in.key(), in.key()
				m.Delete(ctx, first, second)
				delete(want, first)
				delete(want, second)
			case 6:
				key := in.key()
				got, ok := m.Pop(ctx, key)
				if wantValue, wantOK := want[key]; ok != wantOK || got != wantValue {
					t.Fatalf("step %d: Pop(%q) = %q, %v; want %q, %v", step, key, got, ok, wantValue, wantOK)
				}
				delete(want, key)
			case 7:
				value, key, ok := m.Next(ctx)
				wantValue, known := want[key]
				switch {
				case len(want) == 0 && ok:
					t.Fatalf("step %d: Next() = %q, %q from an empty map", step, key, value)
				case len(want) > 0 && (!ok || !known || value != wantValue):
					t.Fatalf("step %d: Next() = %q, %q, %v; want one of the %d stored values", step, key, value, ok, len(want))
				}
				delete(want, key)
			case 8:
				if got := m.Len(ctx); got != len(want) {
					t.Fatalf("step %d: Len() = %d, want %d", step, got, len(want))
				}
			case 9:
				fuzzCheckKeys(t, step, m.Keys(ctx), want)
			case 10:
				got, visits := map[string]string{}, 0
				m.Range(ctx, func(key, value string) bool {
					got[key] = value
					visits++
					return true
				})
				if visits != len(want) || !fuzzSameMap(got, want) {
					t.Fatalf("step %d: Range visited %d values %q, want %q", step, visits, got, want)
				}
				visits = 0
				m.Range(ctx, func(string, string) bool {
					visits++
					return false
				})
				if visits != min(1, len(want)) {
					t.Fatalf("step %d: Range went on for %d values after it was told to stop", step, visits)
				}
			case 11:
				m.Clear(ctx)
				clear(want)
			}
		}

		// Whatever happened, the map must end up holding exactly what the model holds.
		if got := m.Len(ctx); got != len(want) {
			t.Fatalf("at the end: Len() = %d, want %d", got, len(want))
		}
		fuzzCheckKeys(t, -1, m.Keys(ctx), want)
		for key, wantValue := range want {
			if got, ok := m.Load(ctx, key); !ok || got != wantValue {
				t.Fatalf("at the end: Load(%q) = %q, %v; want %q", key, got, ok, wantValue)
			}
		}
	})
}

func fuzzCheckKeys(t *testing.T, step int, got []string, want map[string]string) {
	t.Helper()
	wantKeys := make([]string, 0, len(want))
	for key := range want {
		wantKeys = append(wantKeys, key)
	}
	sort.Strings(got)
	sort.Strings(wantKeys)
	if fmt.Sprintf("%q", got) != fmt.Sprintf("%q", wantKeys) {
		t.Fatalf("step %d: Keys() = %q, want %q", step, got, wantKeys)
	}
}

func fuzzSameMap(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		if gotValue, ok := got[key]; !ok || gotValue != value {
			return false
		}
	}
	return true
}
