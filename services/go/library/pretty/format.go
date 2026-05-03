package pretty

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"go-services/library/gsync"
	"go-services/library/redact"
)

var defaultCfg = newConfig()

var statePool = gsync.Pool[map[uintptr]bool]{
	New: func() map[uintptr]bool {
		return make(map[uintptr]bool)
	},
}

var builderPool = gsync.Pool[*strings.Builder]{
	New: func() *strings.Builder {
		b := &strings.Builder{}
		b.Grow(1024)
		return b
	},
}

func Value(v any, opts ...Option) string {
	if len(opts) == 0 {
		return valueOpt(v, defaultCfg)
	}

	cfg := newConfig(opts...)
	return valueOpt(v, cfg)
}

func valueOpt(v any, cfg *config) string {
	b := builderPool.Get()
	b.Reset()

	visited := statePool.Get()
	clear(visited)

	formatRecursive(cfg, b, visited, reflect.ValueOf(v), 0)

	result := b.String()

	if len(visited) <= 2048 {
		statePool.Put(visited)
	}

	if b.Cap() <= 1024*64 {
		builderPool.Put(b)
	}

	return result
}

func formatRecursive(cfg *config, b *strings.Builder, visited map[uintptr]bool, v reflect.Value, depth int) {
	if depth > cfg.maxDepth {
		b.WriteString("...")
		return
	}

	if !v.IsValid() {
		b.WriteString("<nil>")
		return

	}

	if v.Kind() == reflect.Interface {
		v = v.Elem()
		if !v.IsValid() {
			b.WriteString("<nil>")
			return
		}
	}

	// High-Speed Path for Literal Types
	if v.CanInterface() {
		valI := v.Interface()
		if r, ok := valI.(redact.Redactor); ok {
			b.WriteString(r.Redact())
			return
		}
		if t, ok := valI.(time.Time); ok {
			b.WriteString(t.Format(time.RFC3339))
			return
		}
		if s, ok := valI.(fmt.Stringer); ok {
			b.WriteString(s.String())
			return
		}

		if v.CanAddr() {
			ptrI := v.Addr().Interface()
			if r, ok := ptrI.(redact.Redactor); ok {
				b.WriteString(r.Redact())
				return
			}
			if t, ok := ptrI.(time.Time); ok {
				b.WriteString(t.Format(time.RFC3339))
				return
			}
			if s, ok := ptrI.(fmt.Stringer); ok {
				b.WriteString(s.String())
				return
			}
		}

		switch t := valI.(type) {
		case int:
			b.WriteString(strconv.Itoa(t))
			return
		case string:
			formatString(b, t, cfg.maxBytes)
			return
		case bool:
			b.WriteString(strconv.FormatBool(t))
			return
		case []byte:
			(formatBytes(b, t, cfg.maxBytes))
			return
		case error:
			b.WriteString(t.Error())
			return
		}
	}

	formatBasedOnKind(cfg, b, visited, v, depth)
}

func formatSlice(cfg *config, b *strings.Builder, visited map[uintptr]bool, v reflect.Value, depth int) {
	// Arrays cannot be nil; only check for Slices
	if v.Kind() == reflect.Slice && v.IsNil() {
		b.WriteString("<nil>")
		return
	}

	b.WriteString(v.Type().String())

	length := v.Len()
	if length == 0 {
		b.WriteString("{}")
		return
	}

	b.WriteByte('{')
	limit := min(length, cfg.maxSliceItems)

	for i := range limit {
		if i > 0 {
			b.WriteString(", ")
		}
		formatRecursive(cfg, b, visited, v.Index(i), depth+1)
	}

	if length > cfg.maxSliceItems {
		// Use strconv to avoid fmt.Sprintf overhead
		b.WriteString(", ... and ")
		b.WriteString(strconv.Itoa(length - cfg.maxSliceItems))
		b.WriteString(" more")
	}
	b.WriteByte('}')
}

func formatMap(cfg *config, b *strings.Builder, visited map[uintptr]bool, v reflect.Value, depth int) {
	if v.IsNil() {
		b.WriteString("<nil>")
		return
	}

	b.WriteString(v.Type().String())

	length := v.Len()
	if length == 0 {
		b.WriteString("{}")
		return
	}

	b.WriteString("{")

	// MapRange is the high-performance way to iterate
	iter := v.MapRange()
	count := 0
	for iter.Next() && count < cfg.maxMapItems {
		if count > 0 {
			b.WriteString(", ")
		}

		// iter.Key() and iter.Value() return the current pair
		formatRecursive(cfg, b, visited, iter.Key(), depth+1)
		b.WriteString(": ")
		formatRecursive(cfg, b, visited, iter.Value(), depth+1)
		count++
	}

	if length > count {
		b.WriteString(", ... and ")
		b.WriteString(strconv.Itoa(length - count))
		b.WriteString(" more")
	}
	b.WriteByte('}')
}

func formatStruct(cfg *config, b *strings.Builder, visited map[uintptr]bool, v reflect.Value, depth int) {
	typ := v.Type()
	name := typ.String()
	if name == "" {
		name = "struct"
	}

	b.WriteString(name)
	b.WriteByte('{')

	n := v.NumField()
	actualFields := 0

	for i := range n {
		f := typ.Field(i)

		// Skip unexported
		if f.PkgPath != "" {
			continue
		}

		// If we hit the limit, add ellipsis and stop
		if actualFields >= cfg.maxStructFields {
			if actualFields > 0 {
				b.WriteString(", ")
			}
			b.WriteString("...")
			break
		}

		// Add comma separator for fields after the first one
		if actualFields > 0 {
			b.WriteString(", ")
		}

		b.WriteString(f.Name)
		b.WriteString(": ")

		// Redaction check
		if _, sensitive := cfg.sensitiveFields[strings.ToLower(f.Name)]; sensitive {
			b.WriteString("\"<REDACTED>\"")
		} else {
			formatRecursive(cfg, b, visited, v.Field(i), depth+1)
		}

		actualFields++
	}

	b.WriteByte('}')
}

func formatString(b *strings.Builder, s string, maxBytes int) {
	b.WriteByte('"')
	if len(s) > maxBytes {
		// Safe truncation: back off until we find a valid UTF-8 start
		limit := maxBytes
		for limit > 0 && !utf8.RuneStart(s[limit]) {
			limit--
		}
		b.WriteString(s[:limit])
		b.WriteString("... [truncated]")
	} else {
		b.WriteString(s)
	}
	b.WriteByte('"')
}

func formatBytes(b *strings.Builder, body []byte, maxBytes int) {
	if len(body) <= maxBytes {
		b.Write(body)
		return
	}

	b.Write(body[:maxBytes])
	b.WriteString("... [truncated]")
}

func formatBasedOnKind(cfg *config, b *strings.Builder, visited map[uintptr]bool, v reflect.Value, depth int) {
	kind := v.Kind()

	// Circularity Guard (References only)
	if kind == reflect.Pointer || kind == reflect.Map {
		if !v.IsNil() {
			ptr := v.Pointer()
			if visited[ptr] {
				b.WriteString("<circular>")
				return
			}
			visited[ptr] = true
		}
	}

	// Recursive Logic
	switch kind {
	case reflect.Pointer:
		if v.IsNil() {
			b.WriteString("<nil>")
			return
		}

		b.WriteByte('&')
		formatRecursive(cfg, b, visited, v.Elem(), depth+1)

	case reflect.String:
		formatString(b, v.String(), cfg.maxBytes)

	case reflect.Slice, reflect.Array:
		formatSlice(cfg, b, visited, v, depth+1)

	case reflect.Map:
		formatMap(cfg, b, visited, v, depth+1)

	case reflect.Struct:
		formatStruct(cfg, b, visited, v, depth+1)

	case reflect.Chan:
		if v.IsNil() {
			b.WriteString("<nil chan>")
			return
		}
		// Output: chan string(0xc000123...)
		b.WriteString("chan ")
		b.WriteString(v.Type().Elem().String())
		b.WriteByte('(')
		b.WriteString("0x")
		b.WriteString(strconv.FormatUint(uint64(v.Pointer()), 16))
		b.WriteByte(')')

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b.WriteString(strconv.FormatInt(v.Int(), 10))

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		b.WriteString(strconv.FormatUint(v.Uint(), 10))

	case reflect.Float32, reflect.Float64:
		b.WriteString(strconv.FormatFloat(v.Float(), 'g', -1, 64))

	default:
		fmt.Fprint(b, v.Interface())
	}
}
