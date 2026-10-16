package pages

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxArtifactBytes = 64 << 20
const maxArtifactFiles = 10000

// safePath requires one canonical relative POSIX name to prevent archive traversal and aliases.
func safePath(name string) bool {
	return len(name) > 0 && len(name) <= 2048 && utf8.ValidString(name) && !strings.HasPrefix(name, "/") && name != "." && name != ".." && !strings.HasPrefix(name, "../") && path.Clean(name) == name && !strings.ContainsAny(name, `\:?#`) && strings.IndexFunc(name, unicode.IsControl) < 0
}

// prepareArchive validates the entire artifact before allocating or contacting cloud services.
func prepareArchive(ctx context.Context, files map[string][]byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(files) == 0 || len(files) > maxArtifactFiles {
		return nil, errors.New("invalid Pages artifact file count")
	}
	names := make([]string, 0, len(files))
	total := 0
	for name, data := range files {
		if !safePath(name) {
			return nil, errors.New("invalid Pages artifact path")
		}
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if _, exists := files[parent]; exists {
				return nil, errors.New("conflicting Pages artifact paths")
			}
		}
		if len(data) > maxArtifactBytes-total {
			return nil, errors.New("Pages artifact exceeds size limit")
		}
		total += len(data)
		names = append(names, name)
	}
	sort.Strings(names)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Stored entries bound CPU use and output growth for already compressed browser resources.
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			return nil, errors.New("Pages artifact could not be archived")
		}
		if _, err = entry.Write(files[name]); err != nil {
			return nil, errors.New("Pages artifact could not be archived")
		}
	}
	if writer.Close() != nil || buffer.Len() > maxArtifactBytes+(8<<20) {
		return nil, errors.New("Pages archive exceeds size limit")
	}
	return buffer.Bytes(), nil
}
