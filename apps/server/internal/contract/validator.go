package contract

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

//go:embed schema.json
var schemaBytes []byte

type Validator struct {
	rootSchema map[string]any
}

type schemaError struct {
	path    string
	message string
}

func (err *schemaError) Error() string {
	path := err.path
	if path == "" {
		path = "/"
	}
	return fmt.Sprintf("validation failed at %s: %s", path, err.message)
}

func NewValidator() *Validator {
	var schema map[string]any
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		panic("embedded content schema is invalid: " + err.Error())
	}
	return &Validator{rootSchema: schema}
}

func (validator *Validator) Validate(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("validation failed at /: invalid JSON")
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return fmt.Errorf("validation failed at /: multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return fmt.Errorf("validation failed at /: invalid JSON")
	}
	if err := validator.validateSchema(value, validator.rootSchema, ""); err != nil {
		return err
	}
	return validateSnapshotSemantics(value)
}

func (validator *Validator) validateSchema(value any, schema map[string]any, path string) error {
	if ref, ok := schema["$ref"].(string); ok {
		resolved, err := validator.resolveSchemaRef(ref)
		if err != nil {
			return validationFailure(path, err.Error())
		}
		if err := validator.validateSchema(value, resolved, path); err != nil {
			return err
		}
	}
	if expected, exists := schema["const"]; exists && !schemaValuesEqual(value, expected) {
		return validationFailure(path, "must equal the schema constant")
	}
	if values, ok := schema["enum"].([]any); ok {
		matched := false
		for _, candidate := range values {
			if schemaValuesEqual(value, candidate) {
				matched = true
				break
			}
		}
		if !matched {
			return validationFailure(path, "must be one of the allowed values")
		}
	}
	if typeName, ok := schema["type"].(string); ok && !schemaTypeMatches(value, typeName) {
		return validationFailure(path, "has an invalid type")
	}
	if object, ok := value.(map[string]any); ok {
		if err := validator.validateSchemaObject(object, schema, path); err != nil {
			return err
		}
	}
	if list, ok := value.([]any); ok {
		if err := validator.validateSchemaArray(list, schema, path); err != nil {
			return err
		}
	}
	if stringValue, ok := value.(string); ok {
		if err := validateSchemaString(stringValue, schema, path); err != nil {
			return err
		}
	}
	if number, ok := schemaNumberValue(value); ok {
		if err := validateSchemaNumber(number, schema, path); err != nil {
			return err
		}
	}
	if err := validateSchemaFormat(value, schema, path); err != nil {
		return err
	}
	return validator.validateOneOf(value, schema, path)
}

func (validator *Validator) validateOneOf(value any, schema map[string]any, path string) error {
	branches, ok := schema["oneOf"].([]any)
	if !ok {
		return nil
	}
	matches := 0
	var deepest *schemaError
	for _, branch := range branches {
		branchSchema, ok := branch.(map[string]any)
		if !ok {
			continue
		}
		err := validator.validateSchema(value, branchSchema, path)
		if err == nil {
			matches++
			continue
		}
		var candidate *schemaError
		if errors.As(err, &candidate) && (deepest == nil || len(candidate.path) > len(deepest.path)) {
			deepest = candidate
		}
	}
	if matches == 1 {
		return nil
	}
	if matches > 1 {
		return validationFailure(path, "matches more than one allowed schema")
	}
	if deepest != nil {
		return deepest
	}
	return validationFailure(path, "does not match any allowed schema")
}

func (validator *Validator) validateSchemaObject(value map[string]any, schema map[string]any, path string) error {
	properties, _ := schema["properties"].(map[string]any)
	if required, ok := schema["required"].([]any); ok {
		for _, rawKey := range required {
			key, ok := rawKey.(string)
			if ok {
				if _, exists := value[key]; !exists {
					return validationFailure(schemaPath(path, key), "field is required")
				}
			}
		}
	}
	if additional, ok := schema["additionalProperties"].(bool); ok && !additional {
		for key := range value {
			if _, exists := properties[key]; !exists {
				return validationFailure(schemaPath(path, key), "unknown field")
			}
		}
	}
	for key, rawSchema := range properties {
		childSchema, ok := rawSchema.(map[string]any)
		if !ok {
			continue
		}
		childValue, exists := value[key]
		if exists {
			if err := validator.validateSchema(childValue, childSchema, schemaPath(path, key)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (validator *Validator) validateSchemaArray(value []any, schema map[string]any, path string) error {
	if minimum, ok := schemaIntegerValue(schema["minItems"]); ok && len(value) < minimum {
		return validationFailure(path, "has too few items")
	}
	if maximum, ok := schemaIntegerValue(schema["maxItems"]); ok && len(value) > maximum {
		return validationFailure(path, "has too many items")
	}
	if unique, ok := schema["uniqueItems"].(bool); ok && unique {
		seen := make(map[string]struct{}, len(value))
		for index, item := range value {
			encoded, _ := json.Marshal(item)
			if _, exists := seen[string(encoded)]; exists {
				return validationFailure(schemaPath(path, strconv.Itoa(index)), "must be unique")
			}
			seen[string(encoded)] = struct{}{}
		}
	}
	if itemSchema, ok := schema["items"].(map[string]any); ok {
		for index, item := range value {
			if err := validator.validateSchema(item, itemSchema, schemaPath(path, strconv.Itoa(index))); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSchemaString(value string, schema map[string]any, path string) error {
	if minimum, ok := schemaIntegerValue(schema["minLength"]); ok && len([]rune(value)) < minimum {
		return validationFailure(path, "is shorter than the minimum length")
	}
	if maximum, ok := schemaIntegerValue(schema["maxLength"]); ok && len([]rune(value)) > maximum {
		return validationFailure(path, "is longer than the maximum length")
	}
	if pattern, ok := schema["pattern"].(string); ok {
		matched, err := regexp.MatchString(pattern, value)
		if err != nil || !matched {
			return validationFailure(path, "does not match the required pattern")
		}
	}
	return nil
}

func validateSchemaNumber(value float64, schema map[string]any, path string) error {
	if minimum, ok := schemaNumberValue(schema["minimum"]); ok && value < minimum {
		return validationFailure(path, "is below the minimum")
	}
	if maximum, ok := schemaNumberValue(schema["maximum"]); ok && value > maximum {
		return validationFailure(path, "is above the maximum")
	}
	if minimum, ok := schemaNumberValue(schema["exclusiveMinimum"]); ok && value <= minimum {
		return validationFailure(path, "must be greater than the exclusive minimum")
	}
	return nil
}

func validateSchemaFormat(value any, schema map[string]any, path string) error {
	format, _ := schema["format"].(string)
	stringValue, ok := value.(string)
	if !ok || format == "" {
		return nil
	}
	switch format {
	case "date-time":
		if _, err := time.Parse(time.RFC3339, stringValue); err != nil {
			return validationFailure(path, "must be an RFC3339 date-time")
		}
	case "date":
		if _, err := time.Parse("2006-01-02", stringValue); err != nil {
			return validationFailure(path, "must be an ISO date")
		}
	case "uri":
		parsed, err := url.Parse(stringValue)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return validationFailure(path, "must be an absolute URI")
		}
	case "https-url":
		if !isHTTPSURL(stringValue) {
			return validationFailure(path, "must be an absolute HTTPS URL")
		}
	}
	return nil
}

func isHTTPSURL(value string) bool {
	if strings.TrimSpace(value) != value || strings.ContainsRune(value, '\\') {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" {
		return false
	}
	if port := parsed.Port(); port != "" {
		numericPort, err := strconv.Atoi(port)
		if err != nil || numericPort < 1 || numericPort > 65535 {
			return false
		}
	}
	return true
}

func (validator *Validator) resolveSchemaRef(ref string) (map[string]any, error) {
	if !strings.HasPrefix(ref, "#/$defs/") {
		return nil, errors.New("unsupported schema reference")
	}
	definitions, ok := validator.rootSchema["$defs"].(map[string]any)
	if !ok {
		return nil, errors.New("schema definitions are missing")
	}
	name := strings.TrimPrefix(ref, "#/$defs/")
	definition, ok := definitions[name].(map[string]any)
	if !ok {
		return nil, errors.New("schema reference is missing")
	}
	return definition, nil
}

func schemaTypeMatches(value any, typeName string) bool {
	switch typeName {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := schemaNumberValue(value)
		return ok
	case "integer":
		number, ok := value.(json.Number)
		if !ok {
			return false
		}
		_, err := strconv.ParseInt(number.String(), 10, 64)
		return err == nil
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "null":
		return value == nil
	default:
		return true
	}
}

func schemaNumberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}

func schemaIntegerValue(value any) (int, bool) {
	number, ok := schemaNumberValue(value)
	if !ok || number != float64(int(number)) {
		return 0, false
	}
	return int(number), true
}

func schemaValuesEqual(left, right any) bool { return reflect.DeepEqual(left, right) }

func schemaPath(parent, child string) string {
	return parent + "/" + escapeJSONPointer(child)
}

func validationFailure(path, message string) error {
	if path == "" {
		path = "/"
	}
	return &schemaError{path: path, message: message}
}

func escapeJSONPointer(value string) string {
	return string(bytes.ReplaceAll(bytes.ReplaceAll([]byte(value), []byte("~"), []byte("~0")), []byte("/"), []byte("~1")))
}
