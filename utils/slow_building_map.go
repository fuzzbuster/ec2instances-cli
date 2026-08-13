package utils

import (
	"maps"
	"sync"
	"time"
)

// SlowBuildingMap is a map that is built slowly in the background.
// When a item is got, it will wait for the key to appear or for the map to be built.
type SlowBuildingMap[K comparable, V any] struct {
	mu   sync.RWMutex
	map_ map[K]V
	done bool
	err  error
}

// Get returns the value for the key.
func (m *SlowBuildingMap[K, V]) Get(key K) (V, bool, error) {
	for {
		m.mu.RLock()
		val, ok := m.map_[key]

		if m.done {
			err := m.err
			m.mu.RUnlock()
			return val, ok, err
		}

		m.mu.RUnlock()

		if ok {
			return val, true, nil
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// Wait blocks until the builder finishes and returns its final error.
func (m *SlowBuildingMap[K, V]) Wait() error {
	for {
		m.mu.RLock()
		if m.done {
			err := m.err
			m.mu.RUnlock()
			return err
		}
		m.mu.RUnlock()
		time.Sleep(10 * time.Millisecond)
	}
}

// NewSlowBuildingMap returns a new SlowBuildingMap.
func NewSlowBuildingMap[K comparable, V any](
	builder func(pushChunk func(map[K]V)) error,
) *SlowBuildingMap[K, V] {
	x := &SlowBuildingMap[K, V]{
		map_: make(map[K]V),
	}
	go func() {
		err := builder(func(chunk map[K]V) {
			x.mu.Lock()
			maps.Copy(x.map_, chunk)
			x.mu.Unlock()
		})
		x.mu.Lock()
		x.err = err
		x.done = true
		x.mu.Unlock()
	}()
	return x
}
