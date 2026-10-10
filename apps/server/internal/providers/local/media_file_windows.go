package local

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// openMediaFile lets deletion hide an object while existing downloads retain
// their handle. Windows otherwise denies removal until the last read closes.
func openMediaFile(name string) (*os.File, error) {
	absolute, err := filepath.Abs(name)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	// Preserve support for long temporary paths, including UNC shares.
	if !strings.HasPrefix(absolute, `\\?\`) {
		if strings.HasPrefix(absolute, `\\`) {
			absolute = `\\?\UNC\` + absolute[2:]
		} else {
			absolute = `\\?\` + absolute
		}
	}
	encoded, err := syscall.UTF16PtrFromString(absolute)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	handle, err := syscall.CreateFile(encoded, syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	return os.NewFile(uintptr(handle), name), nil
}
