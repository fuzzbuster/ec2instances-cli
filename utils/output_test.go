package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOutputPath(t *testing.T) {
	root, err := SetOutputDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	got, err := OutputPath(filepath.Join("azure", "instances.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "azure", "instances.json")
	if got != want {
		t.Fatalf("OutputPath() = %q, want %q", got, want)
	}
}

func TestSaveInstancesWritesCompressedVariants(t *testing.T) {
	root, err := SetOutputDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveInstances([]string{"vm"}, filepath.Join("provider", "instances.json")); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", ".gz", ".br"} {
		path := filepath.Join(root, "provider", "instances.json"+suffix)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("stat %s: %v", path, err)
		}
	}
}

func TestOutputPathRejectsEscape(t *testing.T) {
	if _, err := SetOutputDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", "..", filepath.Join("..", "outside"), filepath.Clean("/tmp/outside")} {
		if _, err := OutputPath(path); err == nil {
			t.Errorf("OutputPath(%q) returned no error", path)
		}
	}
}
