//go:build !windows

package local

import "os"

// openMediaFile relies on Unix file handles remaining readable after unlink.
func openMediaFile(name string) (*os.File, error) {
	return os.Open(name)
}
