package utils

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestFunctionGroupRunsAll verifies that every added function is executed
// exactly once.
func TestFunctionGroupRunsAll(t *testing.T) {
	const n = 100
	var executed int64

	fg := FunctionGroup{}
	for i := 0; i < n; i++ {
		fg.Add(func() error {
			atomic.AddInt64(&executed, 1)
			return nil
		})
	}
	if err := fg.Run(); err != nil {
		t.Fatal(err)
	}

	if got := atomic.LoadInt64(&executed); got != n {
		t.Fatalf("expected %d functions to run, got %d", n, got)
	}
}

func TestFunctionGroupReturnsAllErrors(t *testing.T) {
	first := errors.New("first")
	second := errors.New("second")
	var group FunctionGroup
	group.Add(func() error { return first })
	group.Add(func() error { return nil })
	group.Add(func() error { return second })

	err := group.Run()
	if !errors.Is(err, first) || !errors.Is(err, second) {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(err.Error(), "first\nsecond") {
		t.Fatalf("Run() error order = %q", err)
	}
}

// TestFunctionGroupBoundsConcurrency verifies that no more than
// maxConcurrentFetches functions run simultaneously.
func TestFunctionGroupBoundsConcurrency(t *testing.T) {
	const n = 200
	var current int64
	var maxObserved int64

	fg := FunctionGroup{}
	for i := 0; i < n; i++ {
		fg.Add(func() error {
			c := atomic.AddInt64(&current, 1)
			// Track the high-water mark of concurrent executions.
			for {
				m := atomic.LoadInt64(&maxObserved)
				if c <= m || atomic.CompareAndSwapInt64(&maxObserved, m, c) {
					break
				}
			}
			// Hold the slot briefly so concurrent functions overlap and the
			// semaphore is actually exercised.
			time.Sleep(2 * time.Millisecond)
			atomic.AddInt64(&current, -1)
			return nil
		})
	}
	if err := fg.Run(); err != nil {
		t.Fatal(err)
	}

	if got := atomic.LoadInt64(&maxObserved); got > maxConcurrentFetches {
		t.Fatalf("observed %d concurrent executions, exceeds cap of %d", got, maxConcurrentFetches)
	}
	if got := atomic.LoadInt64(&maxObserved); got == 0 {
		t.Fatalf("no functions appear to have run concurrently")
	}
}
