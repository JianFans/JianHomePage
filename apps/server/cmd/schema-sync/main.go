package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	root := filepath.Clean(filepath.Join(".", "..", ".."))
	pairs := [][2]string{
		{
			filepath.Join(root, "packages", "schema", "schema", "content-snapshot.schema.json"),
			filepath.Join("internal", "contract", "schema.json"),
		},
		{
			filepath.Join(root, "packages", "schema", "openapi", "admin.yaml"),
			filepath.Join("internal", "httpapi", "openapi.yaml"),
		},
	}
	for _, pair := range pairs {
		if err := syncFile(pair[0], pair[1]); err != nil {
			panic(err)
		}
	}
}

func syncFile(source, destination string) error {
	value, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read %s: %w", source, err)
	}
	if len(value) == 0 {
		return errors.New("cannot sync an empty contract")
	}
	current, readErr := os.ReadFile(destination)
	if readErr == nil && bytes.Equal(current, value) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}
	if err := os.WriteFile(destination, value, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", destination, err)
	}
	return nil
}
