package mightymap_test

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/thisisdevelopment/mightymap"
	"github.com/thisisdevelopment/mightymap/storage"
)

type segmentProduct struct {
	ID    string
	Title string
	Sizes []float64
}

// The whole Map API on top of the segment storage, the way a cache uses it:
// values of any type, no overwrites.
func TestMightyMap_SegmentStorage(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMightyMapSegmentStorage[string, any](
		storage.WithSegmentDir(t.TempDir()),
		storage.WithSegmentSize(256),
		storage.WithSegmentValueCacheSize(1<<20),
	)
	cm := mightymap.New[string, any](false, store)
	product := &segmentProduct{ID: "5083739", Title: "Dulux Gloss 500ml", Sizes: []float64{0.5, 1, 2.5}}

	t.Run("Store, Load and Has", func(t *testing.T) {
		cm.Store(ctx, "v1:product", product)
		cm.Store(ctx, "v1:count", 3)
		cm.Store(ctx, "v1:name", "catalog")

		got, ok := cm.Load(ctx, "v1:product")
		if !ok || !reflect.DeepEqual(got, product) {
			t.Errorf("Load() = %T %+v, %v; want %+v", got, got, ok, product)
		}
		if !cm.Has(ctx, "v1:count") || cm.Has(ctx, "v1:missing") {
			t.Error("Has() is wrong")
		}
		if _, ok := cm.Load(ctx, "v1:missing"); ok {
			t.Error("Load() found a key that was never stored")
		}
	})

	t.Run("Store does not overwrite", func(t *testing.T) {
		cm.Store(ctx, "v1:name", "other")
		if got, _ := cm.Load(ctx, "v1:name"); got != "catalog" {
			t.Errorf("Load() = %v, want the first value", got)
		}
	})

	t.Run("Len, Keys and Range", func(t *testing.T) {
		if got := cm.Len(ctx); got != 3 {
			t.Errorf("Len() = %d, want 3", got)
		}
		keys := cm.Keys(ctx)
		sort.Strings(keys)
		if want := []string{"v1:count", "v1:name", "v1:product"}; !reflect.DeepEqual(keys, want) {
			t.Errorf("Keys() = %v, want %v", keys, want)
		}
		seen := map[string]any{}
		cm.Range(ctx, func(key string, value any) bool {
			seen[key] = value
			return true
		})
		if want := map[string]any{"v1:count": 3, "v1:name": "catalog", "v1:product": product}; !reflect.DeepEqual(seen, want) {
			t.Errorf("Range() saw %v, want %v", seen, want)
		}
	})

	t.Run("Pop, Delete and Next", func(t *testing.T) {
		if got, ok := cm.Pop(ctx, "v1:count"); !ok || got != 3 {
			t.Errorf("Pop() = %v, %v; want 3", got, ok)
		}
		if cm.Has(ctx, "v1:count") {
			t.Error("the key is still there after Pop()")
		}
		cm.Delete(ctx, "v1:name", "v1:missing")
		value, key, ok := cm.Next(ctx)
		if !ok || key != "v1:product" || !reflect.DeepEqual(value, product) {
			t.Errorf("Next() = %+v, %q, %v; want the one value that is left", value, key, ok)
		}
		if _, _, ok := cm.Next(ctx); ok || cm.Len(ctx) != 0 {
			t.Error("the map is not empty after Next() took the last value")
		}
	})

	t.Run("Clear and Close", func(t *testing.T) {
		cm.Store(ctx, "v2:name", "catalog")
		cm.Clear(ctx)
		if cm.Len(ctx) != 0 || cm.Has(ctx, "v2:name") {
			t.Error("the map is not empty after Clear()")
		}
		cm.Store(ctx, "v3:name", "catalog")
		if got, ok := cm.Load(ctx, "v3:name"); !ok || got != "catalog" {
			t.Errorf("Load() after Clear() = %v, %v", got, ok)
		}
		if err := cm.Close(ctx); err != nil {
			t.Errorf("Close() = %v", err)
		}
	})
}
