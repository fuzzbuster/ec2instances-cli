package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// SaveInstances saves the instances to a file
func SaveInstances(sortedInstances any, relativePath string) error {
	jsonData, err := json.MarshalIndent(sortedInstances, "", " ")
	if err != nil {
		return fmt.Errorf("marshal instances: %w", err)
	}
	jsonString, err := strconv.Unquote(strings.ReplaceAll(strconv.Quote(string(jsonData)), `\\u`, `\u`))
	if err != nil {
		return fmt.Errorf("normalize JSON: %w", err)
	}

	path, err := OutputPath(relativePath)
	if err != nil {
		return err
	}
	if err := ensureParent(path); err != nil {
		return err
	}
	return writeAndCompressFile(path, []byte(jsonString))
}

func ensureParent(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %q: %w", path, err)
	}
	return nil
}

// AppendUnique appends v to s if v is not already present.
func AppendUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
