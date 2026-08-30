package snapshot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
)

const (
	maxJSONBIntegerDigits  = 131072
	maxJSONBFractionDigits = 16383
)

// CanonicalJSON normalizes a single JSON value into a representation that is
// stable across PostgreSQL jsonb storage and retrieval.
func CanonicalJSON(raw []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("JSON contains trailing data")
	}
	value, err := normalizeJSONValue(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func normalizeJSONValue(value any) (any, error) {
	switch typed := value.(type) {
	case json.Number:
		return normalizeJSONNumber(typed)
	case []any:
		for index, item := range typed {
			normalized, err := normalizeJSONValue(item)
			if err != nil {
				return nil, err
			}
			typed[index] = normalized
		}
	case map[string]any:
		for key, item := range typed {
			normalized, err := normalizeJSONValue(item)
			if err != nil {
				return nil, err
			}
			typed[key] = normalized
		}
	}
	return value, nil
}

func normalizeJSONNumber(number json.Number) (json.Number, error) {
	raw := number.String()
	negative := strings.HasPrefix(raw, "-")
	if negative {
		raw = strings.TrimPrefix(raw, "-")
	}

	exponent := 0
	if index := strings.IndexAny(raw, "eE"); index >= 0 {
		parsed, err := strconv.Atoi(raw[index+1:])
		if err != nil {
			return "", errors.New("JSON number exponent is out of range")
		}
		exponent = parsed
		raw = raw[:index]
	}

	integer, fraction, _ := strings.Cut(raw, ".")
	digits := integer + fraction
	if strings.Trim(digits, "0") == "" {
		return json.Number("0"), nil
	}
	if exponent > maxJSONBIntegerDigits+maxJSONBFractionDigits ||
		exponent < -(maxJSONBIntegerDigits+maxJSONBFractionDigits) {
		return "", errors.New("JSON number exceeds PostgreSQL jsonb limits")
	}

	decimalPosition := len(integer) + exponent
	var expanded string
	switch {
	case decimalPosition <= 0:
		expanded = "0." + strings.Repeat("0", -decimalPosition) + digits
	case decimalPosition >= len(digits):
		expanded = digits + strings.Repeat("0", decimalPosition-len(digits))
	default:
		expanded = digits[:decimalPosition] + "." + digits[decimalPosition:]
	}

	integer, fraction, _ = strings.Cut(expanded, ".")
	integer = strings.TrimLeft(integer, "0")
	if integer == "" {
		integer = "0"
	}
	fraction = strings.TrimRight(fraction, "0")
	if len(integer) > maxJSONBIntegerDigits || len(fraction) > maxJSONBFractionDigits {
		return "", errors.New("JSON number exceeds PostgreSQL jsonb limits")
	}
	normalized := integer
	if fraction != "" {
		normalized += "." + fraction
	}
	if negative {
		normalized = "-" + normalized
	}
	return json.Number(normalized), nil
}

func Checksum(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}
