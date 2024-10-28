package mightymap_test

import (
	"sync"
	"testing"

	"github.com/thisisdevelopment/mightymap"
)

func TestConcurrentMap_Concurrency(t *testing.T) {
	// Testing concurrent access to ConcurrentMap

	cm := mightymap.NewConcurrentMap[int, int](true)
	var wg sync.WaitGroup

	t.Run("Concurrent Store", func(t *testing.T) {
		for i := 0; i < 1000; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				cm.Store(i, i*i)
			}(i)
		}
		wg.Wait()

		if cm.Len() != 1000 {
			t.Errorf("Expected 1000 items, got %d", cm.Len())
		}
	})

	t.Run("Concurrent Load and Delete", func(t *testing.T) {
		for i := 0; i < 1000; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				value, ok := cm.Load(i)
				if !ok || value != i*i {
					t.Errorf("Expected to load %d, got %d", i*i, value)
				}
				cm.Delete(i)
			}(i)
		}
		wg.Wait()

		if cm.Len() != 0 {
			t.Errorf("Expected map to be empty after deletes, got %d", cm.Len())
		}
	})
}
