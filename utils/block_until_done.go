package utils

import "sync"

// BlockUntilDone starts fn and returns a getter that waits for its result.
func BlockUntilDone[T any](fn func() (T, error)) func() (T, error) {
	mu := sync.RWMutex{}
	mu.Lock()

	var val T
	var err error
	go func() {
		val, err = fn()
		mu.Unlock()
	}()

	return func() (T, error) {
		mu.RLock()
		defer mu.RUnlock()
		return val, err
	}
}
