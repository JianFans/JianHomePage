package httpurl

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// ParseAbsolute parses a usable absolute HTTP(S) URL without embedded credentials.
func ParseAbsolute(value string) (*url.URL, error) {
	if value == "" || strings.TrimSpace(value) != value {
		return nil, errors.New("URL must not be empty or contain surrounding whitespace")
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return nil, errors.New("URL must be an absolute HTTP(S) URL without credentials")
	}
	if port := parsed.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return nil, errors.New("URL port must be between 1 and 65535")
		}
	}
	return parsed, nil
}
