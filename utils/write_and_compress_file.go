package utils

import (
	"compress/gzip"
	"fmt"
	"os"

	"github.com/andybalholm/brotli"
)

// WriteAndCompressFile writes a file and matching gzip and Brotli files.
func WriteAndCompressFile(relativePath string, data []byte) error {
	path, err := OutputPath(relativePath)
	if err != nil {
		return err
	}
	if err := ensureParent(path); err != nil {
		return err
	}
	return writeAndCompressFile(path, data)
}

func writeAndCompressFile(path string, data []byte) error {
	var group FunctionGroup
	group.Add(func() error {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("write %q: %w", path, err)
		}
		return nil
	})
	group.Add(func() error {
		return writeGzip(path+".gz", data)
	})
	group.Add(func() error {
		return writeBrotli(path+".br", data)
	})
	return group.Run()
}

func writeGzip(path string, data []byte) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %q: %w", path, err)
	}
	writer := gzip.NewWriter(file)
	if _, err := writer.Write(data); err != nil {
		_ = writer.Close()
		_ = file.Close()
		return fmt.Errorf("write %q: %w", path, err)
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		return fmt.Errorf("close gzip %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %q: %w", path, err)
	}
	return nil
}

func writeBrotli(path string, data []byte) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %q: %w", path, err)
	}
	writer := brotli.NewWriter(file)
	if _, err := writer.Write(data); err != nil {
		_ = writer.Close()
		_ = file.Close()
		return fmt.Errorf("write %q: %w", path, err)
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		return fmt.Errorf("close Brotli %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %q: %w", path, err)
	}
	return nil
}
