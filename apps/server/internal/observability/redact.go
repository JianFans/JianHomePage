package observability

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const maxString = 4096
const maxAttributes = 32
const maxRecordBytes = 16 << 10
const maxRecordAttributes = 128

// Allow eight cause/index group pairs, their leaf fields and an Any error wrapper.
const maxAttributeDepth = 18

var urlPattern = regexp.MustCompile(`(?i)(?:https?|postgres(?:ql)?|redis|mysql)://[^\s<>"']+`)
var bearerPattern = regexp.MustCompile(`(?i)\bBearer\s+[^\s,;"']+`)
var jwtPattern = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)
var credentialPattern = regexp.MustCompile(`(?i)\b(password|passwd|pwd|client[_-]?secret|secret[_-]?access[_-]?key|access[_-]?key[_-]?id|token|access[_-]?token|refresh[_-]?token|id[_-]?token|authorization|cookie|set-cookie)["']?\s*[:=]\s*(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s,;&]+)`)
var queryCodePattern = regexp.MustCompile(`(?i)\bcode=(?:"(?:\\.|[^"\\\r\n])*"|'[^'\r\n]*'|[^\s,;&"'<>]+)`)
var jsonCodePattern = regexp.MustCompile(`(?i)("code"\s*:\s*)"(?:\\.|[^"\\\r\n])*"`)

type redactor struct{ secrets []string }

// newRedactor freezes longest-first secrets so shorter overlaps cannot reveal suffixes.
func newRedactor(secrets []string) *redactor {
	values := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		if secret != "" {
			values = append(values, secret)
		}
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	return &redactor{secrets: values}
}

// text removes credentials before truncating, including URL userinfo, query and fragment.
func (redactor *redactor) text(value string) string {
	value = strings.ToValidUTF8(redactor.sanitize(value), "\uFFFD")
	if len(value) > maxString {
		limit := maxString
		for limit > 0 && !utf8.RuneStart(value[limit]) {
			limit--
		}
		value = value[:limit] + "…[truncated]"
	}
	return value
}

// sanitize removes configured and recognizable credentials without truncating future redaction context.
func (redactor *redactor) sanitize(value string) string {
	// Consume complete JSON strings before other substitutions can alter their escapes.
	value = jsonCodePattern.ReplaceAllString(value, `$1"[REDACTED]"`)
	for _, secret := range redactor.secrets {
		value = strings.ReplaceAll(value, secret, "[REDACTED]")
	}
	value = urlPattern.ReplaceAllStringFunc(value, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "[REDACTED URL]"
		}
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		parsed.Fragment = ""
		parsed.RawFragment = ""
		return parsed.String()
	})
	value = bearerPattern.ReplaceAllString(value, "Bearer [REDACTED]")
	value = jwtPattern.ReplaceAllString(value, "[REDACTED JWT]")
	value = credentialPattern.ReplaceAllString(value, "$1=[REDACTED]")
	value = queryCodePattern.ReplaceAllString(value, "code=[REDACTED]")
	return value
}

// secretKey covers structured fields whose values must not be rendered at all.
func secretKey(key string) bool {
	key = strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
	for _, part := range []string{"password", "passwd", "secret", "token", "cookie", "authorization", "credential"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return key == "code" || key == "pwd" || key == "databaseurl" || key == "dsn" || key == "accesskeyid" ||
		key == "query" || key == "rawquery" || key == "fragment" || key == "rawfragment" || key == "sql" || key == "statement"
}

type recordBudget struct{ bytes, attributes int }

// recordBudgetAfter subtracts already bound attributes from per-record limits
// so chained logger contexts cannot reset the available logging budget.
func recordBudgetAfter(bytesUsed, attributesUsed int) *recordBudget {
	return &recordBudget{bytes: max(0, maxRecordBytes-bytesUsed), attributes: max(0, maxRecordAttributes-attributesUsed)}
}

// newRecordBudget allocates independent byte and attribute limits for records
// without previously bound context, avoiding shared mutable accounting.
func newRecordBudget() *recordBudget {
	return &recordBudget{bytes: maxRecordBytes, attributes: maxRecordAttributes}
}

// take redacts before charging the remaining byte budget, then truncates at a
// UTF-8 boundary so truncation cannot bypass redaction or split an encoded rune.
func (budget *recordBudget) take(redactor *redactor, value string) string {
	value = redactor.text(value)
	if budget.bytes <= 0 {
		return ""
	}
	if len(value) <= budget.bytes {
		budget.bytes -= len(value)
		return value
	}
	marker := "…[truncated]"
	limit := budget.bytes - len(marker)
	if limit < 0 {
		limit = budget.bytes
		marker = ""
	}
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	budget.bytes = 0
	return value[:limit] + marker
}

// attr resolves lazy values before recursively sanitizing nested attributes.
func (redactor *redactor) attr(attr slog.Attr, depth int, budget *recordBudget) slog.Attr {
	budget.attributes--
	attr.Key = budget.take(redactor, attr.Key)
	if secretKey(attr.Key) {
		return slog.String(attr.Key, budget.take(redactor, "[REDACTED]"))
	}
	if depth > maxAttributeDepth {
		return slog.String(attr.Key, budget.take(redactor, "[truncated]"))
	}
	value := attr.Value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		attr.Value = slog.StringValue(budget.take(redactor, value.String()))
	case slog.KindGroup:
		items := value.Group()
		clean := make([]slog.Attr, 0, min(len(items), maxAttributes))
		for _, item := range items[:min(len(items), maxAttributes)] {
			if budget.attributes <= 0 {
				break
			}
			clean = append(clean, redactor.attr(item, depth+1, budget))
		}
		attr.Value = slog.GroupValue(clean...)
	case slog.KindAny:
		if err, ok := value.Any().(error); ok {
			attr.Value = redactor.attr(Error(err), depth+1, budget).Value
		} else {
			attr.Value = slog.AnyValue(redactor.anyValue(reflect.ValueOf(value.Any()), depth+1, budget))
		}
	default:
		attr.Value = value
	}
	return attr
}

// anyValue converts application-owned structured values into bounded safe JSON-compatible data.
func (redactor *redactor) anyValue(value reflect.Value, depth int, budget *recordBudget) any {
	if !value.IsValid() {
		return nil
	}
	if depth > 8 {
		return "[truncated]"
	}
	if value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil
		}
		return redactor.anyValue(value.Elem(), depth+1, budget)
	}
	switch value.Kind() {
	case reflect.String:
		return budget.take(redactor, value.String())
	case reflect.Map:
		result := map[string]any{}
		iterator := value.MapRange()
		count := 0
		for iterator.Next() {
			if count >= maxAttributes || budget.attributes <= 0 {
				break
			}
			count++
			budget.attributes--
			key := budget.take(redactor, fmt.Sprint(iterator.Key().Interface()))
			if secretKey(key) {
				result[key] = budget.take(redactor, "[REDACTED]")
			} else {
				result[key] = redactor.anyValue(iterator.Value(), depth+1, budget)
			}
		}
		return result
	case reflect.Array, reflect.Slice:
		result := make([]any, 0, min(value.Len(), maxAttributes))
		for i := 0; i < min(value.Len(), maxAttributes) && budget.attributes > 0; i++ {
			budget.attributes--
			result = append(result, redactor.anyValue(value.Index(i), depth+1, budget))
		}
		return result
	case reflect.Struct:
		result := map[string]any{}
		kind := value.Type()
		for i := 0; i < value.NumField() && i < maxAttributes && budget.attributes > 0; i++ {
			field := kind.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			budget.attributes--
			name = budget.take(redactor, name)
			if secretKey(name) {
				result[name] = budget.take(redactor, "[REDACTED]")
			} else {
				result[name] = redactor.anyValue(value.Field(i), depth+1, budget)
			}
		}
		return result
	case reflect.Bool:
		return value.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return value.Uint()
	case reflect.Float32, reflect.Float64:
		return value.Float()
	default:
		return budget.take(redactor, fmt.Sprint(value.Interface()))
	}
}

type redactingHandler struct {
	next            slog.Handler
	redactor        *redactor
	suppressGroup   bool
	boundBytes      int
	boundAttributes int
}

// Enabled delegates filtering without evaluating or formatting disabled records.
func (handler *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return handler.next.Enabled(ctx, level)
}

// Handle sanitizes both message and current attributes before any output formatter runs.
func (handler *redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	budget := recordBudgetAfter(handler.boundBytes, handler.boundAttributes)
	clean := slog.NewRecord(record.Time, record.Level, budget.take(handler.redactor, record.Message), record.PC)
	count := 0
	record.Attrs(func(attr slog.Attr) bool {
		if count >= maxAttributes || budget.attributes <= 0 {
			return false
		}
		count++
		if handler.suppressGroup {
			clean.AddAttrs(slog.String(budget.take(handler.redactor, attr.Key), budget.take(handler.redactor, "[REDACTED]")))
		} else {
			clean.AddAttrs(handler.redactor.attr(attr, 0, budget))
		}
		return true
	})
	return handler.next.Handle(ctx, clean)
}

// WithAttrs eagerly freezes safe bound values, matching standard slog handler semantics.
func (handler *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, 0, min(len(attrs), maxAttributes))
	budget := recordBudgetAfter(handler.boundBytes, handler.boundAttributes)
	for _, attr := range attrs[:min(len(attrs), maxAttributes)] {
		if budget.attributes <= 0 {
			break
		}
		if handler.suppressGroup {
			clean = append(clean, slog.String(budget.take(handler.redactor, attr.Key), budget.take(handler.redactor, "[REDACTED]")))
		} else {
			clean = append(clean, handler.redactor.attr(attr, 0, budget))
		}
	}
	return &redactingHandler{next: handler.next.WithAttrs(clean), redactor: handler.redactor,
		suppressGroup: handler.suppressGroup, boundBytes: maxRecordBytes - budget.bytes, boundAttributes: maxRecordAttributes - budget.attributes}
}

// WithGroup prevents secret-bearing group names from bypassing final formatting safeguards.
func (handler *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{next: handler.next.WithGroup(handler.redactor.text(name)), redactor: handler.redactor,
		suppressGroup: handler.suppressGroup || secretKey(name), boundBytes: handler.boundBytes, boundAttributes: handler.boundAttributes}
}
