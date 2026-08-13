package utils

import (
	"errors"
	"testing"
)

func TestBlockUntilDoneReturnsError(t *testing.T) {
	want := errors.New("background failed")
	get := BlockUntilDone(func() (int, error) {
		return 42, want
	})

	value, err := get()
	if value != 42 || !errors.Is(err, want) {
		t.Fatalf("get() = (%d, %v)", value, err)
	}
}

func TestSlowBuildingMapReturnsBuilderError(t *testing.T) {
	want := errors.New("builder failed")
	values := NewSlowBuildingMap(func(push func(map[string]int)) error {
		push(map[string]int{"ready": 1})
		return want
	})

	if err := values.Wait(); !errors.Is(err, want) {
		t.Fatalf("Wait() error = %v", err)
	}
	value, ok, err := values.Get("ready")
	if value != 1 || !ok || !errors.Is(err, want) {
		t.Fatalf("Get(ready) = (%d, %t, %v)", value, ok, err)
	}
	if _, ok, err := values.Get("missing"); ok || !errors.Is(err, want) {
		t.Fatalf("Get(missing) = (ok=%t, err=%v)", ok, err)
	}
}
