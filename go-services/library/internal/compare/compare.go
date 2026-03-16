//go:generate go run ../../cmd/assertgen/main.go
package compare

import (
	"cmp"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"

	"go-services/library/pretty"
)

const labelWidth = 9 // Adjusted to 9 to comfortably fit "mismatch:" or "index:"
var stringerTransformer = gocmp.Transformer("Stringer", func(s fmt.Stringer) string {
	return s.String()
})

// Error asserts that got is not nil.
func Error(t *testing.T, got error, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	if got != nil {
		return "", true
	}

	return fail("expected error", msg, msgArgs,
		rowRaw("got", "<nil>"),
		rowRaw("want", "error"),
	)
}

// NoError asserts that got is nil.
func NoError(t *testing.T, got error, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	if got == nil {
		return "", true
	}

	return fail("unexpected error", msg, msgArgs,
		row("got", got),
		rowRaw("want", "no error"),
	)
}

// ErrorContains asserts that got is not nil and its message contains the substring want.
func ErrorContains(t *testing.T, got error, want, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	if failMsg, ok := Error(t, got, msg, msgArgs...); !ok {
		return failMsg, false
	}

	gotMsg := got.Error()
	if strings.Contains(gotMsg, want) {
		return "", true
	}

	return fail("error does not contain expected substring", msg, msgArgs,
		row("got", gotMsg),
		rowRaw("want", "contains %q", want),
	)
}

// ErrorIs asserts that got wraps want (using errors.Is).
func ErrorIs(t *testing.T, got, want error, msg string, msgArgs ...any) (string, bool) {
	t.Helper()

	if errors.Is(got, want) {
		return "", true
	}

	return fail("error mismatch (errors.Is)", msg, msgArgs,
		row("got", got),
		row("want", want),
	)
}

// ErrorAs asserts that got can be assigned to want (using errors.As).
func ErrorAs(t *testing.T, got error, want any, msg string, msgArgs ...any) (string, bool) {
	t.Helper()

	if errors.As(got, want) {
		return "", true
	}

	targetType := reflect.TypeOf(want)
	return fail("error type mismatch (errors.As)", msg, msgArgs,
		row("got", got),
		rowRaw("want", "type %v", targetType),
	)
}

// Panics asserts that the function f panics.
func Panics(t *testing.T, f func(), msg string, msgArgs ...any) (m string, ok bool) {
	t.Helper()

	ok = true

	defer func() {
		if r := recover(); r == nil {
			// If we are here, it means f() returned normally (no panic)
			// This is a failure.
			m, ok = fail(
				"failed to panic",
				msg,
				msgArgs,
				rowRaw("got", "no panic"),
				rowRaw("want", "panic"),
			)
		}
	}()

	f()
	return m, ok
}

// Zero asserts that got is the zero value for its type.
func Zero[T comparable](t *testing.T, got T, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	var zero T
	if got == zero {
		return "", true
	}

	return fail("expected zero value", msg, msgArgs,
		row("got", got),
		row("want", zero),
	)
}

// NotZero asserts that got is not the zero value for its type.
func NotZero[T comparable](t *testing.T, got T, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	var zero T
	if got != zero {
		return "", true
	}

	return fail("expected non-zero value", msg, msgArgs,
		row("got", got),
		rowRaw("want", "non-zero value"),
	)
}

// EqualOpt asserts that got and want are equal.
// It accepts a slice of cmp.Options to customize the comparison logic
// (e.g., cmpopts.IgnoreFields).
func EqualOpt[T any](t *testing.T, got, want T, cmpOpts []gocmp.Option, msg string, msgArgs ...any) (string, bool) {
	t.Helper()

	if gocmp.Equal(got, want, cmpOpts...) {
		return "", true
	}

	return reportMismatch(t, got, want, fmt.Sprintf("values not equal :: "+msg, msgArgs...), cmpOpts...), false
}

// Equal asserts that got and want are equal.
func Equal[T any](t *testing.T, got, want T, msg string, msgArgs ...any) (string, bool) {
	t.Helper()

	return EqualOpt(t, got, want, nil, msg, msgArgs...)
}

// Greater asserts that got is greater than want.
func Greater[T cmp.Ordered](t *testing.T, got, want T, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	if got > want {
		return "", true
	}

	return fail("value not greater", msg, msgArgs,
		row("got", got),
		rowRaw("want", "greater than %s", pretty.Value(want)),
	)
}

// GreaterOrEqual asserts that got is greater than or equal to want.
func GreaterOrEqual[T cmp.Ordered](t *testing.T, got, want T, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	if got >= want {
		return "", true
	}

	return fail("value too small", msg, msgArgs,
		row("got", got),
		rowRaw("want", "at least %s", pretty.Value(want)),
	)
}

// Less asserts that got is less than want.
func Less[T cmp.Ordered](t *testing.T, got, want T, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	if got < want {
		return "", true
	}

	return fail("value not less", msg, msgArgs,
		row("got", got),
		rowRaw("want", "less than %s", pretty.Value(want)),
	)
}

// LessOrEqual asserts that got is less than or equal to want.
func LessOrEqual[T cmp.Ordered](t *testing.T, got, want T, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	if got <= want {
		return "", true
	}

	return fail("too large", msg, msgArgs,
		row("got", got),
		rowRaw("want", "at most %s", pretty.Value(want)),
	)
}

// True asserts that got is false.
func True(t *testing.T, got bool, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	if got {
		return "", true
	}

	return fail("condition was false", msg, msgArgs,
		rowRaw("got", "false"),
		rowRaw("want", "true"),
	)
}

// False asserts that got is true.
func False(t *testing.T, got bool, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	if !got {
		return "", true
	}

	return fail("condition was true", msg, msgArgs,
		rowRaw("got", "true"),
		rowRaw("want", "false"),
	)
}

// Nil asserts that got is nil.
func Nil(t *testing.T, got any, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	if isNil(got) {
		return "", true
	}

	return fail("expected nil", msg, msgArgs,
		row("got", got),
		rowRaw("want", "<nil>"),
	)
}

// NotNil asserts that got is not nil.
func NotNil(t *testing.T, got any, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	if !isNil(got) {
		return "", true
	}

	return fail("expected non-nil value", msg, msgArgs,
		rowRaw("got", "<nil>"),
		rowRaw("want", "<non-nil>"),
	)
}

// SliceContainsOpt asserts that the slice got contains the value want.
// It accepts a slice of cmp.Options to customize the comparison logic
// (e.g., cmpopts.IgnoreFields).
func SliceContainsOpt[T any](
	t *testing.T,
	got []T,
	want T,
	cmpOpts []gocmp.Option,
	msg string,
	msgArgs ...any,
) (string, bool) {
	t.Helper()
	for _, v := range got {
		if gocmp.Equal(v, want, cmpOpts...) {
			return "", true
		}
	}

	return fail("slice does not contain value", msg, msgArgs,
		row("got", got),
		rowRaw("want", "contains %s", pretty.Value(want)),
	)
}

// SliceContains asserts that the slice got contains the value want.
func SliceContains[T any](t *testing.T, got []T, want T, msg string, msgArgs ...any) (string, bool) {
	t.Helper()

	return SliceContainsOpt(t, got, want, nil, msg, msgArgs...)
}

// SliceNotContainsOpt asserts that the slice got does not contain the value want.
// It accepts a slice of cmp.Options to customize the comparison logic
// (e.g., cmpopts.IgnoreFields).
func SliceNotContainsOpt[T any](
	t *testing.T,
	got []T,
	want T,
	cmpOpts []gocmp.Option,
	msg string,
	msgArgs ...any,
) (string, bool) {
	t.Helper()
	for _, v := range got {
		if gocmp.Equal(v, want, cmpOpts...) {
			return fail("slice contains unexpected value", msg, msgArgs,
				row("got", got),
				rowRaw("want", "does not contain %s", pretty.Value(want)),
			)
		}
	}

	return "", true
}

// SliceNotContains asserts that the slice got does not contain the value want.
func SliceNotContains[T any](t *testing.T, got []T, want T, msg string, msgArgs ...any) (string, bool) {
	t.Helper()

	return SliceNotContainsOpt(t, got, want, nil, msg, msgArgs...)
}

// SliceLen asserts that the length of the slice got is equal to want.
func SliceLen[T any](t *testing.T, got []T, want int, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	sliceLength := len(got)
	if sliceLength == want {
		return "", true
	}

	return fail("slice length mismatch", msg, msgArgs,
		row("got len", sliceLength),
		row("want len", want),
	)
}

// SliceIndex asserts that the slice got contains the specified index want.
func SliceIndex[T any](t *testing.T, got []T, want int, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	if want >= 0 && want < len(got) {
		return "", true
	}

	return fail("index out of bound", msg, msgArgs,
		rowRaw("got", "index in range [0, %d] (total length: %d)", len(got)-1, len(got)),
		rowRaw("want", "index %d", want),
	)
}

// SliceAtOpt asserts that the element at the specified index in the slice got is equal to want.
// It accepts a slice of cmp.Options to customize the comparison logic
// (e.g., cmpopts.IgnoreFields).
func SliceAtOpt[T any](
	t *testing.T,
	got []T,
	index int,
	want T,
	cmpOpts []gocmp.Option,
	msg string,
	msgArgs ...any,
) (string, bool) {
	t.Helper()

	formattedMsg := fmt.Sprintf(msg, msgArgs...)

	if failMsg, ok := SliceIndex(t, got, index, msg, msgArgs...); !ok {
		return failMsg, false
	}

	gotAtIndex := got[index]
	if gocmp.Equal(gotAtIndex, want, cmpOpts...) {
		return "", true
	}

	formattedMsgWithIndex := fmt.Sprintf("wrong value at index %d :: %s", index, formattedMsg)
	return reportMismatch(t, gotAtIndex, want, formattedMsgWithIndex, cmpOpts...), false
}

// SliceAt asserts that the element at the specified index in the slice got is equal to want.
func SliceAt[T any](t *testing.T, got []T, index int, want T, msg string, msgArgs ...any) (string, bool) {
	t.Helper()

	return SliceAtOpt(t, got, index, want, nil, msg, msgArgs...)
}

// SliceEmpty asserts that the slice got is empty (length is 0).
func SliceEmpty[T any](t *testing.T, got []T, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	if len(got) == 0 {
		return "", true
	}

	return fail("slice is not empty", msg, msgArgs,
		row("got", got),
		rowRaw("want", "empty slice"),
	)
}

// SliceNotEmpty asserts that the slice got is not empty (length is greater than 0).
func SliceNotEmpty[T any](t *testing.T, got []T, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	if len(got) != 0 {
		return "", true
	}

	return fail("slice is empty", msg, msgArgs,
		row("got", got),
		rowRaw("want", "non-empty slice"),
	)
}

// StringContains asserts that the string got contains the substring want.
func StringContains(t *testing.T, got, want, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	if strings.Contains(got, want) {
		return "", true
	}

	return fail("string does not contain expected substring", msg, msgArgs,
		row("got", got),
		rowRaw("want", "contains %q", want),
	)
}

// StringNotContains asserts that the string got does not contain the substring want.
func StringNotContains(t *testing.T, got, want, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	if !strings.Contains(got, want) {
		return "", true
	}

	return fail("string contains forbidden substring", msg, msgArgs,
		row("got", got),
		rowRaw("want", "does not contain %q", want),
	)
}

// MapContainsKey asserts that the map got contains the specified key want.
func MapContainsKey[K comparable, V any](
	t *testing.T,
	got map[K]V,
	want K,
	msg string,
	msgArgs ...any,
) (string, bool) {
	t.Helper()
	if _, ok := got[want]; ok {
		return "", true
	}

	return fail("map missing required key", msg, msgArgs,
		row("got", got),
		rowRaw("want", "contains key %s", pretty.Value(want)),
	)
}

// MapNotContainsKey asserts that the map got does not contain the specified key want.
func MapNotContainsKey[K comparable, V any](
	t *testing.T,
	got map[K]V,
	want K,
	msg string,
	msgArgs ...any,
) (string, bool) {
	t.Helper()
	if _, ok := got[want]; !ok {
		return "", true
	}

	return fail("map has forbidden key", msg, msgArgs,
		row("got", got),
		rowRaw("want", "does not contain key %s", pretty.Value(want)),
	)
}

// MapLen asserts that the length of the map got is equal to want.
func MapLen[K comparable, V any](t *testing.T, got map[K]V, want int, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	mapLength := len(got)
	if mapLength == want {
		return "", true
	}

	return fail("map length mismatch", msg, msgArgs,
		row("got len", mapLength),
		row("want len", want),
	)
}

// MapAtOpt asserts that the value at the specified key in the map got is equal to want.
// It accepts a slice of cmp.Options to customize the comparison logic
// (e.g., cmpopts.IgnoreFields).
func MapAtOpt[K comparable, V any](
	t *testing.T,
	got map[K]V,
	key K,
	want V,
	cmpOpts []gocmp.Option,
	msg string,
	msgArgs ...any,
) (string, bool) {
	t.Helper()

	if failMsg, ok := MapContainsKey(t, got, key, msg, msgArgs...); !ok {
		return failMsg, false
	}

	gotAtKey := got[key]
	if gocmp.Equal(gotAtKey, want, cmpOpts...) {
		return "", true
	}

	formattedMsg := fmt.Sprintf(msg, msgArgs...)
	formattedMsgWithKey := fmt.Sprintf("wrong value for key %s :: %s", pretty.Value(key), formattedMsg)
	return reportMismatch(t, gotAtKey, want, formattedMsgWithKey, cmpOpts...), false
}

// MapAt asserts that the value at the specified key in the map got is equal to want.
func MapAt[K comparable, V any](t *testing.T, got map[K]V, key K, want V, msg string, msgArgs ...any) (string, bool) {
	t.Helper()

	return MapAtOpt(t, got, key, want, nil, msg, msgArgs...)
}

// MapEmpty asserts that the map got is empty (length is 0).
func MapEmpty[K comparable, V any](t *testing.T, got map[K]V, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	if len(got) == 0 {
		return "", true
	}

	return fail("map is not empty", msg, msgArgs,
		row("got", got),
		rowRaw("want", "empty map"),
	)
}

// MapNotEmpty asserts that the map got is not empty (length is greater than 0).
func MapNotEmpty[K comparable, V any](t *testing.T, got map[K]V, msg string, msgArgs ...any) (string, bool) {
	t.Helper()
	if len(got) != 0 {
		return "", true
	}

	return fail("map is empty", msg, msgArgs,
		row("got", got),
		rowRaw("want", "non-empty map"),
	)
}

// fail constructs the standard error message format.
func fail(header, msg string, args []any, rows ...string) (string, bool) {
	formattedMsg := fmt.Sprintf(msg, args...)
	detail := strings.Join(rows, "\n")
	return fmt.Sprintf("%s :: %s\n%s", header, formattedMsg, detail), false
}

// reportMismatch performs a comparison between got and want. It uses deep comparison
// for maps, slices, structs, and pointers, and direct comparison (==) for other types.
// If they do not match, it returns a formatted failure string.
func reportMismatch(t *testing.T, got, want any, header string, cmpOpts ...gocmp.Option) string {
	t.Helper()

	var detail string

	// Check for type mismatch specifically for better error messages
	if reflect.TypeOf(got) != reflect.TypeOf(want) && got != nil && want != nil {
		detail = fmt.Sprintf(
			"%s\n%s",
			rowRaw("got", "%T(%s)", got, pretty.Value(got)),
			rowRaw("want", "%T(%s)", want, pretty.Value(want)),
		)
		return fmt.Sprintf("%s\n%s", header, detail)
	}

	rv := reflect.ValueOf(got)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct, reflect.Pointer, reflect.Interface:
		internalOpts := []gocmp.Option{stringerTransformer}
		allOpts := append(internalOpts, cmpOpts...)
		diff := gocmp.Diff(want, got, allOpts...)
		// Remove trailing newline from Diff if present
		diff = strings.TrimSuffix(diff, "\n")
		detail = fmt.Sprintf("%s\n%s", rowRaw("mismatch", "(-want +got)"), diff)
	default:
		// Simple types (int, string, bool) - avoid Diff overhead
		detail = fmt.Sprintf("%s\n%s", row("got", got), row("want", want))
	}

	return fmt.Sprintf("%s\n%s", header, detail)
}

// isNil returns true if the input is a literal nil or if it is a
// nillable type (Pointer, Chan, Map, Slice, Func, Interface) that
// contains a nil value.
func isNil(got any) bool {
	if got == nil {
		return true
	}
	val := reflect.ValueOf(got)
	switch val.Kind() {
	case reflect.Chan,
		reflect.Func,
		reflect.Map,
		reflect.Pointer,
		reflect.UnsafePointer,
		reflect.Slice:
		return val.IsNil()
	}
	return false
}

// row returns a formatted string with a padded label and its value.
func row(label string, value any) string {
	return fmt.Sprintf("%-*s %s", labelWidth, label+":", pretty.Value(value))
}

// rowRaw returns a formatted string without %#v (useful for pre-formatted strings).
func rowRaw(label, value string, valueArgs ...any) string {
	return fmt.Sprintf("%-*s %s", labelWidth, label+":", fmt.Sprintf(value, valueArgs...))
}
