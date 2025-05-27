package mightymap_test

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/thisisdevelopment/mightymap"
	"github.com/thisisdevelopment/mightymap/storage"
)

func ExampleMap() {
	ctx := context.Background()
	cm := mightymap.New[int, string](true)
	cm.Store(ctx, 1, "one")
	fmt.Println(cm.Load(ctx, 1))
	// Output: one true
}

func ExampleMap_Load() {
	ctx := context.Background()
	// Using Badger storage
	// Load example
	cm := mightymap.New[int, string](true, storage.NewMightyMapBadgerStorage[int, string]())
	cm.Store(ctx, 1, "one")
	fmt.Println(cm.Load(ctx, 1))
	// Output: one true
}

func ExampleMap_Next() {
	ctx := context.Background()
	// Using Badger storage
	// Next example
	cm := mightymap.New[int, string](true, storage.NewMightyMapBadgerStorage[int, string]())
	cm.Store(ctx, 1, "one")
	fmt.Println(cm.Next(ctx))
	// Output: one 1 true
}

func ExampleMap_Range() {
	ctx := context.Background()
	cm := mightymap.New[int, string](true)
	cm.Store(ctx, 1, "one")
	cm.Store(ctx, 2, "two")

	values := []string{}
	cm.Range(ctx, func(key int, value string) bool {
		values = append(values, value)
		return true
	})
	slices.Sort(values)
	fmt.Println(strings.Join(values, "\n"))
	// Output:
	// one
	// two
}

func ExampleMap_Delete() {
	ctx := context.Background()
	cm := mightymap.New[int, string](true)
	cm.Store(ctx, 1, "one")
	cm.Store(ctx, 2, "two")
	cm.Delete(ctx, 1, 2)
	value, ok := cm.Load(ctx, 1)
	fmt.Printf("Value: %v, OK: %v\n", value, ok)
	// Output: Value: , OK: false
}
