package assert

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func Zero[T comparable](t *testing.T, got T, msg string, msgArgs ...any) {
	t.Helper()
	var zero T
	if got != zero {
		t.Errorf("%s\ngot: %#v\nwant: zero value (%#v)", fmt.Sprintf(msg, msgArgs...), got, zero)
	}
}

func NotZero[T comparable](t *testing.T, got T, msg string, msgArgs ...any) {
	t.Helper()
	var zero T
	if got == zero {
		t.Errorf("%s\ngot: %#v\nwant: non-zero value", fmt.Sprintf(msg, msgArgs...), got)
	}
}

func Equal[T comparable](t *testing.T, got T, want T, msg string, msgArgs ...any) {
	t.Helper()
	if got != want {
		t.Errorf("%s\ngot: %#v\nwant: %#v", fmt.Sprintf(msg, msgArgs...), got, want)
	}
}

func DeepEqual[T any](t *testing.T, got T, want T, msg string, msgArgs ...any) {
	t.Helper()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("%s\nmismatch (-want +got):\n%s", fmt.Sprintf(msg, msgArgs...), diff)
	}
}

func True(t *testing.T, got bool, msg string, msgArgs ...any) {
	t.Helper()
	if !got {
		t.Errorf("%s\ngot: true\nwant: false", fmt.Sprintf(msg, msgArgs...))
	}
}

func False(t *testing.T, got bool, msg string, msgArgs ...any) {
	t.Helper()
	if got {
		t.Errorf("%s\ngot: false\nwant: true", fmt.Sprintf(msg, msgArgs...))
	}
}

func Nil(t *testing.T, got any, msg string, msgArgs ...any) {
	t.Helper()
	if got != nil {
		t.Errorf("%s\ngot: %v\nwant: nil", fmt.Sprintf(msg, msgArgs...), got)
	}
}

func NotNil(t *testing.T, got any, msg string, msgArgs ...any) {
	t.Helper()
	if got == nil {
		t.Errorf("%s\ngot: %v\nwant: non-nil", fmt.Sprintf(msg, msgArgs...), got)
	}
}

func SliceContains[T any](t *testing.T, got []T, want T, msg string, msgArgs ...any) {
	t.Helper()
	for _, v := range got {
		if cmp.Equal(v, want) {
			return
		}
	}

	t.Errorf("%s\ngot: %#v\nwant to contain: %#v", fmt.Sprintf(msg, msgArgs...), got, want)
}

func SliceNotContains[T any](t *testing.T, got []T, want T, msg string, msgArgs ...any) {
	t.Helper()
	for _, v := range got {
		if cmp.Equal(v, want) {
			t.Errorf(
				"%s\ngot: %#v\nwant not to contain: %#v",
				fmt.Sprintf(msg, msgArgs...),
				got,
				want,
			)
			return
		}
	}
}

func SliceIndex[T any](t *testing.T, slice []T, index int, want T, msg string, msgArgs ...any) {
	t.Helper()
	if index < 0 || index >= len(slice) {
		t.Errorf(
			"%s\ngot slice length: %d\nwant index in range: %d",
			fmt.Sprintf(msg, msgArgs...),
			len(slice),
			index,
		)
		return
	}

	got := slice[index]
	if !cmp.Equal(got, want) {
		t.Errorf("%s\ngot: %#v\nwant: %#v at index %d\nslice: %#v",
			fmt.Sprintf(msg, msgArgs...), got, want, index, slice)
	}
}

func SliceEmpty[T any](t *testing.T, got []T, msg string, msgArgs ...any) {
	t.Helper()
	if len(got) != 0 {
		t.Errorf("%s\ngot: %#v\nwant: empty slice", fmt.Sprintf(msg, msgArgs...), got)
	}
}

func SliceNotEmpty[T any](t *testing.T, got []T, msg string, msgArgs ...any) {
	t.Helper()
	if len(got) == 0 {
		t.Errorf("%s\ngot: empty slice\nwant: non-empty slice", fmt.Sprintf(msg, msgArgs...))
	}
}

func StringContains(t *testing.T, got, substring, msg string, msgArgs ...any) {
	t.Helper()
	if !strings.Contains(got, substring) {
		t.Errorf("%s\ngot: %q\nwant substring: %q", fmt.Sprintf(msg, msgArgs...), got, substring)
	}
}

func StringNotContains(t *testing.T, got, substring, msg string, msgArgs ...any) {
	t.Helper()
	if strings.Contains(got, substring) {
		t.Errorf("%s\ngot: %q\nwant: does not contain %q", fmt.Sprintf(msg, msgArgs...), got, substring)
	}
}

func StringContainsAll(t *testing.T, got string, substrings []string, msg string, msgArgs ...any) {
	t.Helper()
	for _, sub := range substrings {
		if !strings.Contains(got, sub) {
			t.Errorf("%s\ngot: %q\nwant to contain substring: %q", fmt.Sprintf(msg, msgArgs...), got, sub)
		}
	}
}

func StringNotContainsAll(t *testing.T, got string, substrings []string, msg string, msgArgs ...any) {
	t.Helper()
	for _, sub := range substrings {
		if strings.Contains(got, sub) {
			t.Errorf("%s\ngot: %q\nwant not to contain substring: %q", fmt.Sprintf(msg, msgArgs...), got, sub)
		}
	}
}

func MapContainsKey[K comparable, V any](t *testing.T, got map[K]V, want K, msg string, msgArgs ...any) {
	t.Helper()
	if _, ok := got[want]; !ok {
		t.Errorf("%s\ngot: %#v\nwant to contain key: %#v", fmt.Sprintf(msg, msgArgs...), got, want)
	}
}

func MapNotContainsKey[K comparable, V any](t *testing.T, got map[K]V, want K, msg string, msgArgs ...any) {
	t.Helper()
	if _, ok := got[want]; ok {
		t.Errorf("%s\ngot: %#v\nwant not to contain key: %#v", fmt.Sprintf(msg, msgArgs...), got, want)
	}
}

func MapValue[K comparable, V any](t *testing.T, m map[K]V, key K, want V, msg string, msgArgs ...any) {
	t.Helper()

	got, ok := m[key]
	if !ok {
		t.Errorf(
			"%s\ngot: key %v not present\nwant: key to exist with value %#v",
			fmt.Sprintf(msg, msgArgs...),
			key,
			want,
		)
		return
	}

	if !cmp.Equal(got, want) {
		t.Errorf("%s\ngot: %#v\nwant: %#v for key %v", fmt.Sprintf(msg, msgArgs...), got, want, key)
	}
}

func MapEmpty[K comparable, V any](t *testing.T, got map[K]V, msg string, msgArgs ...any) {
	t.Helper()
	if len(got) != 0 {
		t.Errorf("%s\ngot: %#v\nwant: empty map", fmt.Sprintf(msg, msgArgs...), got)
	}
}

func MapNotEmpty[K comparable, V any](t *testing.T, got map[K]V, msg string, msgArgs ...any) {
	t.Helper()
	if len(got) == 0 {
		t.Errorf("%s\ngot: empty map\nwant: non-empty map", fmt.Sprintf(msg, msgArgs...))
	}
}
