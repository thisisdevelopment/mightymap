package mightymap_test

import (
	"fmt"
	"slices"
	"strings"

	"github.com/thisisdevelopment/mightymap"
	"github.com/thisisdevelopment/mightymap/storage"
)

func ExampleMap() {
	cm := mightymap.New[int, string](true)
	cm.Store(1, "one")
	fmt.Println(cm.Load(1))
	// Output: one true
}

func ExampleMap_Load() {
	// Using Badger storage
	// Load example
	cm := mightymap.New[int, string](true, storage.NewMightyMapBadgerStorage[int, string]())
	cm.Store(1, "one")
	fmt.Println(cm.Load(1))
	// Output: one true
}

func ExampleMap_Next() {
	// Using Badger storage
	// Next example
	cm := mightymap.New[int, string](true, storage.NewMightyMapBadgerStorage[int, string]())
	cm.Store(1, "one")
	fmt.Println(cm.Next())
	// Output: one 1 true
}

func ExampleMap_Range() {
	cm := mightymap.New[int, string](true)
	cm.Store(1, "one")
	cm.Store(2, "two")

	values := []string{}
	cm.Range(func(key int, value string) bool {
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
	cm := mightymap.New[int, string](true)
	cm.Store(1, "one")
	cm.Store(2, "two")
	cm.Delete(1, 2)
	value, ok := cm.Load(1)
	fmt.Printf("Value: %v, OK: %v\n", value, ok)
	// Output: Value: , OK: false
}
