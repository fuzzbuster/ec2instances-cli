package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	outputDirMu sync.RWMutex
	outputDir   string
)

func SetOutputDir(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve output directory %q: %w", path, err)
	}
	absolute = filepath.Clean(absolute)
	if err := os.MkdirAll(absolute, 0o755); err != nil {
		return "", fmt.Errorf("create output directory %q: %w", absolute, err)
	}

	outputDirMu.Lock()
	outputDir = absolute
	outputDirMu.Unlock()
	return absolute, nil
}

func OutputPath(relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("output path must be a non-empty relative path: %q", relative)
	}
	cleaned := filepath.Clean(relative)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("output path escapes output directory: %q", relative)
	}

	outputDirMu.RLock()
	root := outputDir
	outputDirMu.RUnlock()
	if root == "" {
		return "", fmt.Errorf("output directory is not configured")
	}
	return filepath.Join(root, cleaned), nil
}
