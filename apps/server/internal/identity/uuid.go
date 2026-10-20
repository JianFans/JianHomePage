// Package identity generates and validates typed UUIDv4 identifiers.
package identity

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var prefixPattern = regexp.MustCompile(`^[a-z][a-z0-9]*_$`)

// New returns a cryptographically random UUIDv4 with a lowercase type prefix.
func New(prefix string) (string, error) {
	if !prefixPattern.MatchString(prefix) {
		return "", errors.New("identifier prefix must be lowercase and end with an underscore")
	}
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate UUIDv4: %w", err)
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80

	encoded := hex.EncodeToString(value[:])
	return prefix + encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}

// MustNew returns New's value or panics if the system random source is unavailable.
func MustNew(prefix string) string {
	value, err := New(prefix)
	if err != nil {
		panic("secure random source unavailable")
	}
	return value
}

// Valid reports whether value contains a canonical lowercase UUIDv4 and prefix.
func Valid(value, prefix string) bool {
	if !prefixPattern.MatchString(prefix) || !strings.HasPrefix(value, prefix) {
		return false
	}
	raw := value[len(prefix):]
	if len(raw) != 36 || raw[8] != '-' || raw[13] != '-' || raw[18] != '-' || raw[23] != '-' {
		return false
	}
	for index, char := range raw {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	decoded, err := hex.DecodeString(strings.ReplaceAll(raw, "-", ""))
	return err == nil && decoded[6]>>4 == 4 && decoded[8]&0xc0 == 0x80
}
