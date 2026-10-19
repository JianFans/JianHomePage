package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"yujian.me/server/internal/sitegen"
	"yujian.me/server/internal/webassets"
)

// runCommand handles informational commands and exports local previews with the
// embedded renderer. An empty directory prevents accidental replacement of data.
func runCommand(args []string, output io.Writer) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if len(args) == 1 && args[0] == "--version" {
		_, err := fmt.Fprintln(output, "yujian-server "+Version)
		return true, err
	}
	if len(args) == 1 && args[0] == "--help" {
		_, err := fmt.Fprintln(output, "yujian-server: start the management server using environment configuration\nyujian-server --version: show application version\nyujian-server --export-site snapshot.json --output empty-directory: export a local static preview")
		return true, err
	}
	if len(args) != 4 || args[0] != "--export-site" || args[2] != "--output" || args[1] == "" || args[3] == "" {
		return true, errors.New("usage: --export-site snapshot.json --output empty-directory")
	}
	target, err := filepath.Abs(args[3])
	if err != nil {
		return true, err
	}
	entries, err := os.ReadDir(target)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return true, err
	}
	if len(entries) != 0 {
		return true, errors.New("output directory must be empty")
	}
	file, err := os.Open(args[1])
	if err != nil {
		return true, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	if err != nil {
		return true, err
	}
	renderer, err := webassets.Renderer()
	if err != nil {
		return true, err
	}
	assets, err := webassets.Public()
	if err != nil {
		return true, err
	}
	generator, err := sitegen.New(renderer, assets)
	if err != nil {
		return true, err
	}
	files, err := generator.Generate(context.Background(), raw)
	if err != nil {
		return true, err
	}
	for name, data := range files {
		destination := filepath.Join(target, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			return true, err
		}
		if err := os.WriteFile(destination, data, 0644); err != nil {
			return true, err
		}
	}
	_, err = fmt.Fprintln(output, target)
	return true, err
}
