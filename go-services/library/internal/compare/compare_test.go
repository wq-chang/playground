package compare_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"go-services/library/internal/compare"
	"go-services/library/pretty"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestError(t *testing.T) {
	t.Run("returns true and empty string when error is present", func(t *testing.T) {
		err := errors.New("something went wrong")

		msg, ok := compare.Error(t, err, "it should have errored")

		if !ok {
			t.Errorf("expected ok to be true when error is present, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message when error is present, got %q", msg)
		}
	})

	t.Run("returns false and formatted message when error is nil", func(t *testing.T) {
		customMsg := "database should fail"

		msg, ok := compare.Error(t, nil, "%s", customMsg)

		if ok {
			t.Fatalf("expected ok to be false when error is nil, got true")
		}

		expectedGot := rowRaw("got", "<nil>")
		expectedWant := rowRaw("want", "error")
		expectedMsg := fmt.Sprintf("expected error :: %s\n%s\n%s", customMsg, expectedGot, expectedWant)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestNoError(t *testing.T) {
	t.Run("returns true when error is nil", func(t *testing.T) {
		msg, ok := compare.NoError(t, nil, "should be fine")

		if !ok {
			t.Errorf("expected ok to be true when error is nil, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and formatted message when error exists", func(t *testing.T) {
		err := errors.New("boom")
		customMsg := "action should succeed"

		msg, ok := compare.NoError(t, err, "status: %s", customMsg)

		if ok {
			t.Fatalf("expected ok to be false when error exists, got true")
		}

		expectedGot := rowRaw("got", "boom")
		expectedWant := rowRaw("want", "no error")

		expectedMsg := fmt.Sprintf(
			"unexpected error :: %s\n%s\n%s",
			"status: action should succeed",
			expectedGot,
			expectedWant,
		)

		if msg != expectedMsg {
			t.Errorf("\nmessage mismatch!\ngot:  %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestErrorContains(t *testing.T) {
	t.Run("returns true when error contains substring", func(t *testing.T) {
		err := errors.New("database connection refused")
		msg, ok := compare.ErrorContains(t, err, "connection", "check db")

		if !ok {
			t.Errorf("expected success, but got failure message: %q", msg)
		}
	})

	t.Run("returns false (via Error) when error is nil", func(t *testing.T) {
		msg, ok := compare.ErrorContains(t, nil, "any", "context: %s", "API")

		if ok {
			t.Fatal("expected failure because error is nil")
		}

		if !strings.HasPrefix(msg, "expected error :: context: API") {
			t.Errorf("unexpected message prefix: %q", msg)
		}
	})

	t.Run("returns false and formatted message when substring is missing", func(t *testing.T) {
		err := errors.New("permission denied")
		substring := "timeout"
		customMsg := "file access"

		msg, ok := compare.ErrorContains(t, err, substring, "%s", customMsg)

		if ok {
			t.Fatalf("Expected failure because substring %q is missing from %q", substring, err)
		}

		expectedGot := row("got", err.Error())
		expectedWant := rowRaw("want", "contains %q", substring)

		expectedMsg := fmt.Sprintf(
			"error does not contain expected substring :: %s\n%s\n%s",
			customMsg,
			expectedGot,
			expectedWant,
		)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot:  %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestErrorIs(t *testing.T) {
	baseErr := errors.New("sentinel error")
	wrappedErr := fmt.Errorf("additional context: %w", baseErr)

	t.Run("returns true when error is in the chain", func(t *testing.T) {
		msg, ok := compare.ErrorIs(t, wrappedErr, baseErr, "database check")

		if !ok {
			t.Errorf("expected ok to be true when error is found, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and formatted message when error is not in chain", func(t *testing.T) {
		got := errors.New("actual error")
		want := baseErr

		msg, ok := compare.ErrorIs(t, got, want, "api call")

		if ok {
			t.Fatalf("expected ok to be false when error is missing, got true")
		}

		expectedGot := row("got", got)
		expectedWant := row("want", want)
		expectedMsg := fmt.Sprintf("error mismatch (errors.Is) :: api call\n%s\n%s", expectedGot, expectedWant)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestErrorAs(t *testing.T) {
	t.Run("returns true when error can be cast to target", func(t *testing.T) {
		var target CustomError
		got := fmt.Errorf("wrapped: %w", CustomError{Message: "fail"})

		msg, ok := compare.ErrorAs(t, got, &target, "type check")

		if !ok {
			t.Errorf("expected ok to be true when type matches, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and formatted message when type does not match", func(t *testing.T) {
		var target CustomError
		got := errors.New("standard error")

		msg, ok := compare.ErrorAs(t, got, &target, "custom error check")

		if ok {
			t.Fatalf("expected ok to be false for type mismatch, got true")
		}

		expectedGot := row("got", got)
		// reflect.TypeOf on a pointer to CustomError
		expectedWant := rowRaw("want", "type *compare_test.CustomError")
		expectedMsg := fmt.Sprintf("error type mismatch (errors.As) :: custom error check\n%s\n%s", expectedGot, expectedWant)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestPanics(t *testing.T) {
	t.Run("returns true when function panics", func(t *testing.T) {
		// The function panics as expected, so Panic should return false (no assertion failure)
		msg, ok := compare.Panics(t, func() {
			panic("error")
		}, "should panic")

		if !ok {
			t.Errorf("expected ok to be true when function panics, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and formatted message when function does not panic", func(t *testing.T) {
		customMsg := "execution flow"

		// The function does NOT panic, so Panic should "fail" (return true)
		msg, ok := compare.Panics(t, func() {
			// do nothing
		}, "check %s", customMsg)

		if ok {
			t.Fatalf("expected ok to be false when function does not panic, got true")
		}

		expectedGot := rowRaw("got", "no panic")
		expectedWant := rowRaw("want", "panic")

		expectedMsg := fmt.Sprintf(
			"failed to panic :: %s\n%s\n%s",
			"check execution flow",
			expectedGot,
			expectedWant,
		)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot:  %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestZero(t *testing.T) {
	t.Run("returns true when value is zero", func(t *testing.T) {
		// Testing with an int zero value
		msg, ok := compare.Zero(t, 0, "check integer")

		if !ok {
			t.Errorf("expected ok to be true for zero value, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and formatted message when value is not zero", func(t *testing.T) {
		val := "hello"
		customMsg := "string check"

		// "hello" is not the zero value for string (""), so it should fail (return true)
		msg, ok := compare.Zero(t, val, "performing %s", customMsg)

		if ok {
			t.Fatalf("expected ok to be false for non-zero value, got true")
		}

		// Constructing expected message using your formatting helpers
		expectedGot := row("got", val)
		expectedWant := row("want", "") // zero value for string

		expectedMsg := fmt.Sprintf(
			"expected zero value :: %s\n%s\n%s",
			"performing string check",
			expectedGot,
			expectedWant,
		)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", expectedMsg, msg)
		}
	})
}

func TestNotZero(t *testing.T) {
	t.Run("returns true when value is not zero", func(t *testing.T) {
		msg, ok := compare.NotZero(t, "foo", "check string")

		if !ok {
			t.Errorf("expected ok to be true for non-zero value, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and formatted message when value is zero", func(t *testing.T) {
		val := 0
		customMsg := "counter"

		// 0 is the zero value, so NotZero should "fail" (return true)
		msg, ok := compare.NotZero(t, val, "evaluating %s", customMsg)

		if ok {
			t.Fatalf("expected ok to be false for zero value, got true")
		}

		expectedGot := row("got", val)
		expectedWant := rowRaw("want", "non-zero value")

		expectedMsg := fmt.Sprintf(
			"expected non-zero value :: %s\n%s\n%s",
			"evaluating counter",
			expectedGot,
			expectedWant,
		)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestEqualOpt(t *testing.T) {
	type User struct {
		Name string
		ID   int
	}

	t.Run("returns true when ignoring fields with cmpopts", func(t *testing.T) {
		// IDs are different, but we will ignore them
		got := User{ID: 1, Name: "Alice"}
		want := User{ID: 2, Name: "Alice"}

		opts := []cmp.Option{cmpopts.IgnoreFields(User{}, "ID")}

		msg, ok := compare.EqualOpt(t, got, want, opts, "user comparison")

		if !ok {
			t.Errorf("expected ok to be true when ignoring ID field, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and diff when complex structs mismatch", func(t *testing.T) {
		got := User{ID: 1, Name: "Alice"}
		want := User{ID: 1, Name: "Bob"}

		msg, ok := compare.EqualOpt(t, got, want, nil, "check name")

		if ok {
			t.Fatalf("expected ok to be false for struct mismatch, got true")
		}

		// reportMismatch uses reflect to detect Structs and triggers gocmp.Diff
		diff := strings.TrimSuffix(cmp.Diff(want, got), "\n")
		expectedHeader := "values not equal :: check name"
		expectedDetail := fmt.Sprintf("%s\n%s", rowRaw("mismatch", "(-want +got)"), diff)
		expectedMsg := expectedHeader + "\n" + expectedDetail

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestEqual(t *testing.T) {
	t.Run("returns true when simple values match", func(t *testing.T) {
		msg, ok := compare.Equal(t, 100, 100, "check integer")

		if !ok {
			t.Errorf("expected ok to be true for matching integers, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and mismatch report when values differ", func(t *testing.T) {
		got, want := "apple", "orange"
		msg, ok := compare.Equal(t, got, want, "fruit check")

		if ok {
			t.Fatalf("expected ok to be false for mismatch, got true")
		}

		// Since these are strings, reportMismatch uses the default (non-diff) branch
		expectedHeader := "values not equal :: fruit check"
		expectedDetail := fmt.Sprintf("%s\n%s", row("got", got), row("want", want))
		expectedMsg := expectedHeader + "\n" + expectedDetail

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})

	t.Run("reports exact message for numeric type mismatch", func(t *testing.T) {
		var got any = int32(42)
		var want any = int64(42)
		msg, ok := compare.Equal(t, got, want, "type test")

		if ok {
			t.Fatalf("expected ok to be false")
		}

		expectedHeader := "values not equal :: type test"
		expectedDetail := fmt.Sprintf("%s\n%s", rowRaw("got", "int32(%d)", got), rowRaw("want", "int64(%d)", want))
		expectedMsg := expectedHeader + "\n" + expectedDetail

		if msg != expectedMsg {
			t.Errorf("mismatch in generated error message\ngot:  %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestGreater(t *testing.T) {
	t.Run("returns true when got is greater than want", func(t *testing.T) {
		msg, ok := compare.Greater(t, 10, 5, "check threshold")

		if !ok {
			t.Errorf("expected ok to be true when value is greater, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false when got is equal to want", func(t *testing.T) {
		got, want := 10, 10
		msg, ok := compare.Greater(t, got, want, "limit test")

		if ok {
			t.Fatalf("expected ok to be false when values are equal, got true")
		}

		expectedGot := row("got", got)
		expectedWant := rowRaw("want", "greater than 10")
		expectedMsg := fmt.Sprintf("value not greater :: limit test\n%s\n%s", expectedGot, expectedWant)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestGreaterOrEqual(t *testing.T) {
	t.Run("returns true when values are equal", func(t *testing.T) {
		msg, ok := compare.GreaterOrEqual(t, 10, 10, "edge case")

		if !ok {
			t.Errorf("expected ok to be true when values are equal, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false when got is smaller than want", func(t *testing.T) {
		got, want := 5, 10
		msg, ok := compare.GreaterOrEqual(t, got, want, "size check")

		if ok {
			t.Fatalf("expected ok to be false when got is smaller, got true")
		}

		expectedGot := row("got", got)
		expectedWant := rowRaw("want", "at least 10")
		expectedMsg := fmt.Sprintf("value too small :: size check\n%s\n%s", expectedGot, expectedWant)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestLess(t *testing.T) {
	t.Run("returns true when got is less than want", func(t *testing.T) {
		msg, ok := compare.Less(t, 5, 10, "check under limit")

		if !ok {
			t.Errorf("expected ok to be true when value is less, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false when got is equal to want", func(t *testing.T) {
		got, want := 10, 10
		msg, ok := compare.Less(t, got, want, "boundary check")

		if ok {
			t.Fatalf("expected ok to be false when values are equal, got true")
		}

		expectedGot := row("got", got)
		expectedWant := rowRaw("want", "less than 10")
		expectedMsg := fmt.Sprintf("value not less :: boundary check\n%s\n%s", expectedGot, expectedWant)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestLessOrEqual(t *testing.T) {
	t.Run("returns true when values are equal", func(t *testing.T) {
		msg, ok := compare.LessOrEqual(t, 10, 10, "max limit")

		if !ok {
			t.Errorf("expected ok to be true when values are equal, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false when got is larger than want", func(t *testing.T) {
		got, want := 15, 10
		msg, ok := compare.LessOrEqual(t, got, want, "capacity check")

		if ok {
			t.Fatalf("expected ok to be false when got is larger, got true")
		}

		expectedGot := row("got", got)
		expectedWant := rowRaw("want", "at most 10")
		expectedMsg := fmt.Sprintf("too large :: capacity check\n%s\n%s", expectedGot, expectedWant)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestTrue(t *testing.T) {
	t.Run("returns true when got is true", func(t *testing.T) {
		msg, ok := compare.True(t, true, "check boolean")

		if !ok {
			t.Errorf("expected ok to be true when value is true, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and formatted message when got is false", func(t *testing.T) {
		got := false
		customMsg := "flag status"

		msg, ok := compare.True(t, got, "checking %s", customMsg)

		if ok {
			t.Fatalf("expected ok to be false when value is false, got true")
		}

		expectedGot := rowRaw("got", "false")
		expectedWant := rowRaw("want", "true")
		expectedMsg := fmt.Sprintf(
			"condition was false :: checking flag status\n%s\n%s",
			expectedGot,
			expectedWant,
		)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestFalse(t *testing.T) {
	t.Run("returns true when got is false", func(t *testing.T) {
		msg, ok := compare.False(t, false, "check boolean")

		if !ok {
			t.Errorf("expected ok to be true when value is false, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and formatted message when got is true", func(t *testing.T) {
		got := true
		customMsg := "error state"

		msg, ok := compare.False(t, got, "verifying %s", customMsg)

		if ok {
			t.Fatalf("expected ok to be false when value is true, got true")
		}

		expectedGot := rowRaw("got", "true")
		expectedWant := rowRaw("want", "false")
		expectedMsg := fmt.Sprintf(
			"condition was true :: verifying error state\n%s\n%s",
			expectedGot,
			expectedWant,
		)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestNil(t *testing.T) {
	t.Run("returns true when value is literal nil", func(t *testing.T) {
		msg, ok := compare.Nil(t, nil, "direct nil check")

		if !ok {
			t.Errorf("expected ok to be true for literal nil, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns true when value is a typed nil pointer", func(t *testing.T) {
		var ptr *int = nil
		msg, ok := compare.Nil(t, ptr, "typed nil check")

		if !ok {
			t.Errorf("expected ok to be true for (*int)(nil), got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and message when value is not nil", func(t *testing.T) {
		got := 100
		msg, ok := compare.Nil(t, got, "value check")

		if ok {
			t.Fatalf("expected ok to be false for integer value, got true")
		}

		expectedGot := row("got", got)
		expectedWant := rowRaw("want", "<nil>")
		expectedMsg := fmt.Sprintf("expected nil :: value check\n%s\n%s", expectedGot, expectedWant)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestNotNil(t *testing.T) {
	t.Run("returns true when value is an actual object", func(t *testing.T) {
		msg, ok := compare.NotNil(t, "hello", "string check")

		if !ok {
			t.Errorf("expected ok to be true for string, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false when value is nil", func(t *testing.T) {
		msg, ok := compare.NotNil(t, nil, "nil check")

		if ok {
			t.Fatalf("expected ok to be false for nil, got true")
		}

		expectedGot := rowRaw("got", "<nil>")
		expectedWant := rowRaw("want", "<non-nil>")
		expectedMsg := fmt.Sprintf("expected non-nil value :: nil check\n%s\n%s", expectedGot, expectedWant)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestSliceContainsOpt(t *testing.T) {
	type Item struct {
		Name string
		ID   int
	}

	t.Run("returns true when ignoring fields via options", func(t *testing.T) {
		got := []Item{
			{ID: 1, Name: "A"},
			{ID: 2, Name: "B"},
		}
		// We want an item named "B", ignoring the ID mismatch
		want := Item{ID: 999, Name: "B"}
		opts := []cmp.Option{cmpopts.IgnoreFields(Item{}, "ID")}

		msg, ok := compare.SliceContainsOpt(t, got, want, opts, "inventory check")

		if !ok {
			t.Errorf("expected ok to be true with ignored ID field, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false when no items match even with options", func(t *testing.T) {
		got := []Item{{ID: 1, Name: "A"}}
		want := Item{ID: 1, Name: "C"}

		msg, ok := compare.SliceContainsOpt(t, got, want, nil, "exact match check")

		if ok {
			t.Fatalf("expected ok to be false for non-matching item, got true")
		}

		// Verifying the want line uses Go-syntax representation for structs
		expectedWant := rowRaw("want", "contains %s", pretty.Value(want))
		if !strings.Contains(msg, expectedWant) {
			t.Errorf("message did not contain expected want line\ngot: %q\nwant: %q", msg, expectedWant)
		}
	})
}

func TestSliceContains(t *testing.T) {
	t.Run("returns true when slice contains simple value", func(t *testing.T) {
		got := []int{10, 20, 30}
		msg, ok := compare.SliceContains(t, got, 20, "search list")

		if !ok {
			t.Errorf("expected ok to be true when value exists, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and message when value is missing", func(t *testing.T) {
		got := []string{"apple", "banana"}
		want := "orange"
		msg, ok := compare.SliceContains(t, got, want, "fruit check")

		if ok {
			t.Fatalf("expected ok to be false when value missing, got true")
		}

		expectedGot := row("got", got)
		// Using %#v ensures strings are quoted in the "want" line
		expectedWant := rowRaw("want", `contains "orange"`)
		expectedMsg := fmt.Sprintf("slice does not contain value :: fruit check\n%s\n%s", expectedGot, expectedWant)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestSliceNotContainsOpt(t *testing.T) {
	type Token struct {
		Value string
		ID    int
	}

	t.Run("returns true when no match found with options", func(t *testing.T) {
		got := []Token{{Value: "secret", ID: 1}}
		want := Token{Value: "public", ID: 1}

		msg, ok := compare.SliceNotContainsOpt(t, got, want, nil, "check")

		if !ok {
			t.Errorf("expected ok to be true for non-matching struct, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and correct message when match found using options", func(t *testing.T) {
		got := []Token{{Value: "secret", ID: 1}}
		// Different ID, but we'll ignore it via options to trigger a "match" (which is a failure for NotContains)
		want := Token{Value: "secret", ID: 99}
		opts := []cmp.Option{cmpopts.IgnoreFields(Token{}, "ID")}

		msg, ok := compare.SliceNotContainsOpt(t, got, want, opts, "token check for %s", "security")

		if ok {
			t.Fatalf("expected ok to be false when value matches via options, got true")
		}

		// Verify the components of the failure message
		expectedHeader := "slice contains unexpected value :: token check for security"
		expectedGot := row("got", got)
		// %#v should show the field names for the struct in the "want" line
		expectedWant := rowRaw("want", "does not contain %s", pretty.Value(want))

		expectedMsg := fmt.Sprintf("%s\n%s\n%s", expectedHeader, expectedGot, expectedWant)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestSliceNotContains(t *testing.T) {
	t.Run("returns true when value is absent", func(t *testing.T) {
		got := []int{1, 2, 3}
		msg, ok := compare.SliceNotContains(t, got, 4, "exclusion check")

		if !ok {
			t.Errorf("expected ok to be true when value is absent, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and message when value is present", func(t *testing.T) {
		got := []string{"admin", "editor", "viewer"}
		want := "admin"
		msg, ok := compare.SliceNotContains(t, got, want, "security check")

		if ok {
			t.Fatalf("expected ok to be false when forbidden value exists, got true")
		}

		expectedGot := row("got", got)
		// %#v ensures the string is quoted in the output
		expectedWant := rowRaw("want", `does not contain "admin"`)
		expectedMsg := fmt.Sprintf("slice contains unexpected value :: security check\n%s\n%s", expectedGot, expectedWant)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestSliceLen(t *testing.T) {
	t.Run("returns true when length matches", func(t *testing.T) {
		got := []int{10, 20, 30}
		msg, ok := compare.SliceLen(t, got, 3, "checking item count")

		if !ok {
			t.Errorf("expected ok to be true when length matches, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and formatted message when length mismatches", func(t *testing.T) {
		got := []string{"a", "b"}
		want := 5
		customMsg := "buffer size"

		msg, ok := compare.SliceLen(t, got, want, "verifying %s", customMsg)

		if ok {
			t.Fatalf("expected ok to be false for length mismatch, got true")
		}

		expectedGotLen := row("got len", 2)
		expectedWantLen := row("want len", 5)

		expectedMsg := fmt.Sprintf(
			"slice length mismatch :: verifying buffer size\n%s\n%s",
			expectedGotLen,
			expectedWantLen,
		)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestSliceIndex(t *testing.T) {
	t.Run("returns true when index is within bounds", func(t *testing.T) {
		got := []string{"first", "second"}
		msg, ok := compare.SliceIndex(t, got, 1, "index check")

		if !ok {
			t.Errorf("expected ok to be true for valid index, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and detailed message when index is out of bounds", func(t *testing.T) {
		got := []int{100, 200, 300}
		index := 5

		msg, ok := compare.SliceIndex(t, got, index, "accessing element")

		if ok {
			t.Fatalf("expected ok to be false for out of bounds, got true")
		}

		// Verifying the descriptive range string [0, len-1]
		expectedGot := rowRaw("got", "index in range [0, 2] (total length: 3)")
		expectedWant := rowRaw("want", "index 5")

		expectedMsg := fmt.Sprintf(
			"index out of bound :: accessing element\n%s\n%s",
			expectedGot,
			expectedWant,
		)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestSliceAtOpt(t *testing.T) {
	type User struct {
		Name string
		Age  int
	}

	t.Run("returns true when ignoring fields via options", func(t *testing.T) {
		got := []User{{Name: "Alice", Age: 30}}
		want := User{Name: "Alice", Age: 99} // Age differs
		opts := []cmp.Option{cmpopts.IgnoreFields(User{}, "Age")}

		msg, ok := compare.SliceAtOpt(t, got, 0, want, opts, "user check")

		if !ok {
			t.Errorf("expected ok to be true when ignoring Age, got false (msg: %q)", msg)
		}
	})
}

func TestSliceAt(t *testing.T) {
	t.Run("returns true when index is valid and value matches", func(t *testing.T) {
		slice := []string{"a", "b", "c"}
		msg, ok := compare.SliceAt(t, slice, 1, "b", "check element")

		if !ok {
			t.Errorf("expected ok to be true when index and value match, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and out-of-bounds message when index is invalid", func(t *testing.T) {
		slice := []int{10, 20}
		index := 5
		customMsg := "out check"

		msg, ok := compare.SliceAt(t, slice, index, 99, "doing %s", customMsg)

		if ok {
			t.Fatalf("expected ok to be false for out-of-bounds index, got true")
		}

		expectedGot := rowRaw("got", "index in range [0, 1] (total length: 2)")
		expectedWant := rowRaw("want", "index 5")

		expectedMsg := fmt.Sprintf(
			"index out of bound :: doing out check\n%s\n%s",
			expectedGot,
			expectedWant,
		)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})

	t.Run("returns false and mismatch message when value at index is wrong", func(t *testing.T) {
		slice := []string{"apple", "orange"}
		index := 0
		got, want := "apple", "banana"

		msg, ok := compare.SliceAt(t, slice, index, want, "fruit verify")

		if ok {
			t.Fatalf("expected ok to be false for value mismatch, got true")
		}

		// This branch calls reportMismatch, which for strings uses row()
		expectedHeader := "wrong value at index 0 :: fruit verify"
		expectedDetail := fmt.Sprintf("%s\n%s", row("got", got), row("want", want))
		expectedMsg := expectedHeader + "\n" + expectedDetail

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})

	t.Run("returns false and gocmp diff for complex mismatch", func(t *testing.T) {
		type user struct {
			Name string
			ID   int
		}
		got := user{ID: 1, Name: "Alice"}
		want := user{ID: 1, Name: "Bob"}
		slice := []user{got}

		msg, ok := compare.SliceAt(t, slice, 0, want, "user comparison")

		if ok {
			t.Fatalf("expected failure for mismatched structs")
		}

		// Complex types use gocmp.Diff
		diff := strings.TrimSuffix(cmp.Diff(want, got), "\n")
		expectedHeader := "wrong value at index 0 :: user comparison"
		expectedDetail := fmt.Sprintf(
			"%s\n%s",
			rowRaw("mismatch", "(-want +got)"),
			diff,
		)
		expectedMsg := expectedHeader + "\n" + expectedDetail

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})

	t.Run("reports exact message for type mismatch in interface slice", func(t *testing.T) {
		got := []any{100.5} // float64
		index := 0
		var want any = 100 // int
		msg, ok := compare.SliceAt(t, got, index, want, "slice type test")

		if ok {
			t.Fatalf("expected ok to be false")
		}

		expectedHeader := "wrong value at index 0 :: slice type test"
		expectedDetail := fmt.Sprintf("%s\n%s",
			rowRaw("got", "float64(%v)", 100.5),
			rowRaw("want", "int(%v)", 100),
		)
		expectedMsg := expectedHeader + "\n" + expectedDetail

		if msg != expectedMsg {
			t.Errorf("mismatch in generated error message\ngot:  %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestSliceEmpty(t *testing.T) {
	t.Run("returns true when slice is empty", func(t *testing.T) {
		// Testing with an empty slice of integers
		var got []int
		msg, ok := compare.SliceEmpty(t, got, "checking capacity")

		if !ok {
			t.Errorf("expected ok to be true for empty slice, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and formatted message when slice is not empty", func(t *testing.T) {
		got := []string{"item1", "item2"}
		customMsg := "buffer check"

		// Slice has items, so SliceEmpty should "fail" (return true)
		msg, ok := compare.SliceEmpty(t, got, "validating %s", customMsg)

		if ok {
			t.Fatalf("expected ok to be false for non-empty slice, got true")
		}

		expectedGot := row("got", got)
		expectedWant := rowRaw("want", "empty slice")

		expectedMsg := fmt.Sprintf(
			"slice is not empty :: validating buffer check\n%s\n%s",
			expectedGot,
			expectedWant,
		)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestSliceNotEmpty(t *testing.T) {
	t.Run("returns true when slice has elements", func(t *testing.T) {
		msg, ok := compare.SliceNotEmpty(t, []int{1}, "check items")

		if !ok {
			t.Errorf("expected ok to be true for non-empty slice, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and formatted message when slice is empty", func(t *testing.T) {
		var got []int
		msg, ok := compare.SliceNotEmpty(t, got, "validating list")

		if ok {
			t.Fatalf("expected ok to be false for empty slice, got true")
		}

		expectedGot := row("got", got)
		expectedWant := rowRaw("want", "non-empty slice")
		expectedMsg := fmt.Sprintf("slice is empty :: validating list\n%s\n%s", expectedGot, expectedWant)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestStringContains(t *testing.T) {
	t.Run("returns true when string contains substring", func(t *testing.T) {
		msg, ok := compare.StringContains(t, "hello world", "world", "content check")

		if !ok {
			t.Errorf("expected ok to be true when substring exists, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false when substring is missing", func(t *testing.T) {
		got, sub := "hello", "world"
		msg, ok := compare.StringContains(t, got, sub, "check %s", "greeting")

		if ok {
			t.Fatalf("expected ok to be false when substring missing, got true")
		}

		expectedGot := row("got", got)
		expectedWant := rowRaw("want", `contains "world"`)
		expectedMsg := fmt.Sprintf("string does not contain expected substring :: check greeting\n%s\n%s", expectedGot, expectedWant)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestStringNotContains(t *testing.T) {
	t.Run("returns true when substring is absent", func(t *testing.T) {
		msg, ok := compare.StringNotContains(t, "apple pie", "banana", "exclusion check")

		if !ok {
			t.Errorf("expected ok to be true when substring absent, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false when substring is present", func(t *testing.T) {
		got, sub := "secret token", "token"
		msg, ok := compare.StringNotContains(t, got, sub, "security check")

		if ok {
			t.Fatalf("expected ok to be false when substring present, got true")
		}

		expectedGot := row("got", got)
		expectedWant := rowRaw("want", `does not contain "token"`)
		expectedMsg := fmt.Sprintf("string contains forbidden substring :: security check\n%s\n%s", expectedGot, expectedWant)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestMapContainsKey(t *testing.T) {
	t.Run("returns true when map contains the key", func(t *testing.T) {
		got := map[string]int{"a": 1, "b": 2}
		msg, ok := compare.MapContainsKey(t, got, "a", "check config")

		if !ok {
			t.Errorf("expected ok to be true when key exists, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and formatted message when key is missing", func(t *testing.T) {
		got := map[int]string{1: "active"}
		wantKey := 2
		customMsg := "user map"

		msg, ok := compare.MapContainsKey(t, got, wantKey, "verifying %s", customMsg)

		if ok {
			t.Fatalf("expected ok to be false when key is missing, got true")
		}

		expectedGot := row("got", got)
		expectedWant := rowRaw("want", "contains key 2")
		expectedMsg := fmt.Sprintf(
			"map missing required key :: verifying user map\n%s\n%s",
			expectedGot,
			expectedWant,
		)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestMapNotContainsKey(t *testing.T) {
	t.Run("returns true when map does not contain the key", func(t *testing.T) {
		got := map[string]bool{"active": true}
		msg, ok := compare.MapNotContainsKey(t, got, "deleted", "status check")

		if !ok {
			t.Errorf("expected ok to be true when key is absent, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and formatted message when map contains forbidden key", func(t *testing.T) {
		got := map[string]int{"admin": 1}
		forbiddenKey := "admin"

		msg, ok := compare.MapNotContainsKey(t, got, forbiddenKey, "security check")

		if ok {
			t.Fatalf("expected ok to be false when forbidden key exists, got true")
		}

		expectedGot := row("got", got)
		expectedWant := rowRaw("want", `does not contain key "admin"`)
		expectedMsg := fmt.Sprintf(
			"map has forbidden key :: security check\n%s\n%s",
			expectedGot,
			expectedWant,
		)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestMapLen(t *testing.T) {
	t.Run("returns true when map length matches", func(t *testing.T) {
		got := map[string]int{"a": 1, "b": 2}
		msg, ok := compare.MapLen(t, got, 2, "checking map size")

		if !ok {
			t.Errorf("expected ok to be true when length matches, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and formatted message when map length mismatches", func(t *testing.T) {
		got := map[int]string{1: "one"}
		want := 3
		customMsg := "user registry"

		msg, ok := compare.MapLen(t, got, want, "verifying %s", customMsg)

		if ok {
			t.Fatalf("expected ok to be false for length mismatch, got true")
		}

		expectedGotLen := row("got len", 1)
		expectedWantLen := row("want len", 3)

		expectedMsg := fmt.Sprintf(
			"map length mismatch :: verifying user registry\n%s\n%s",
			expectedGotLen,
			expectedWantLen,
		)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestMapAtOpt(t *testing.T) {
	type Data struct {
		Val string
		ID  int
	}

	t.Run("returns true when value matches via cmpopts", func(t *testing.T) {
		m := map[int]Data{
			1: {ID: 101, Val: "primary"},
		}

		// Want Val to match, but we don't care about ID
		want := Data{ID: 999, Val: "primary"}
		opts := []cmp.Option{cmpopts.IgnoreFields(Data{}, "ID")}

		msg, ok := compare.MapAtOpt(t, m, 1, want, opts, "ignoring ID")

		if !ok {
			t.Errorf("expected ok to be true with ignored ID, got false: %s", msg)
		}
	})
}

func TestMapAt(t *testing.T) {
	t.Run("returns true when key exists and value matches", func(t *testing.T) {
		m := map[string]int{"status": 200}
		msg, ok := compare.MapAt(t, m, "status", 200, "check response")

		if !ok {
			t.Errorf("expected ok to be true when key and value match, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and key missing message when key is not in map", func(t *testing.T) {
		m := map[string]string{"env": "prod"}
		key := "debug"

		// MapAt calls MapContainsKey internally; we expect that specific failure format
		msg, ok := compare.MapAt(t, m, key, "true", "config check")

		if ok {
			t.Fatalf("expected ok to be false when key is missing, got true")
		}

		expectedGot := row("got", m)
		expectedWant := rowRaw("want", `contains key "debug"`)
		expectedMsg := fmt.Sprintf(
			"map missing required key :: config check\n%s\n%s",
			expectedGot,
			expectedWant,
		)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})

	t.Run("returns false and mismatch message when value for key is wrong", func(t *testing.T) {
		m := map[int]string{1: "Alice"}
		key := 1
		got, want := "Alice", "Bob"

		msg, ok := compare.MapAt(t, m, key, want, "user lookup")

		if ok {
			t.Fatalf("expected ok to be false for value mismatch, got true")
		}

		// This branch uses reportMismatch with a custom header and extra map context
		expectedHeader := "wrong value for key 1 :: user lookup"
		expectedDetail := fmt.Sprintf("%s\n%s", row("got", got), row("want", want))
		expectedMsg := expectedHeader + "\n" + expectedDetail

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})

	t.Run("returns false and gocmp diff when map value is a complex struct mismatch", func(t *testing.T) {
		type data struct {
			Tags  []string
			Score int
		}

		m := map[string]data{
			"player1": {Score: 10, Tags: []string{"pro"}},
		}
		key := "player1"

		// The 'got' value has Tags:["pro"], but we 'want' Tags:["noob"]
		want := data{Score: 10, Tags: []string{"noob"}}

		msg, ok := compare.MapAt(t, m, key, want, "checking player stats")

		if ok {
			t.Fatalf("expected ok to be false for complex struct mismatch, got true")
		}

		// Calculate expected diff
		diff := strings.TrimSuffix(cmp.Diff(want, m[key]), "\n")

		expectedHeader := `wrong value for key "player1" :: checking player stats`
		expectedDetail := fmt.Sprintf("%s\n%s", rowRaw("mismatch", "(-want +got)"), diff)

		expectedMsg := expectedHeader + "\n" + expectedDetail

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})

	t.Run("reports exact message for type mismatch in map value", func(t *testing.T) {
		m := map[string]any{"count": int32(5)}
		key := "count"
		var want any = 5 // int (default for literal)
		msg, ok := compare.MapAt(t, m, key, want, "map type test")

		if ok {
			t.Fatalf("expected ok to be false")
		}

		// MapAt uses pretty.Value(key) for the header
		expectedHeader := fmt.Sprintf("wrong value for key %s :: map type test", pretty.Value(key))
		expectedDetail := fmt.Sprintf("%s\n%s",
			rowRaw("got", "int32(%v)", 5),
			rowRaw("want", "int(%v)", 5),
		)
		expectedMsg := expectedHeader + "\n" + expectedDetail

		if msg != expectedMsg {
			t.Errorf("mismatch in generated error message\ngot:  %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestMapEmpty(t *testing.T) {
	t.Run("returns true when map is empty", func(t *testing.T) {
		var got map[string]int // nil map has len 0
		msg, ok := compare.MapEmpty(t, got, "checking initialization")

		if !ok {
			t.Errorf("expected ok to be true for empty map, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and formatted message when map is not empty", func(t *testing.T) {
		got := map[string]int{"a": 1}
		customMsg := "cache state"

		msg, ok := compare.MapEmpty(t, got, "verifying %s", customMsg)

		if ok {
			t.Fatalf("expected ok to be false for non-empty map, got true")
		}

		expectedGot := row("got", got)
		expectedWant := rowRaw("want", "empty map")
		expectedMsg := fmt.Sprintf(
			"map is not empty :: verifying cache state\n%s\n%s",
			expectedGot,
			expectedWant,
		)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

func TestMapNotEmpty(t *testing.T) {
	t.Run("returns true when map has entries", func(t *testing.T) {
		got := map[int]string{1: "ok"}
		msg, ok := compare.MapNotEmpty(t, got, "check entries")

		if !ok {
			t.Errorf("expected ok to be true for non-empty map, got false")
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("returns false and formatted message when map is empty", func(t *testing.T) {
		got := make(map[string]bool) // empty initialized map has len 0
		customMsg := "registry"

		msg, ok := compare.MapNotEmpty(t, got, "validating %s", customMsg)

		if ok {
			t.Fatalf("expected ok to be false for empty map, got true")
		}

		expectedGot := row("got", got)
		expectedWant := rowRaw("want", "non-empty map")
		expectedMsg := fmt.Sprintf(
			"map is empty :: validating registry\n%s\n%s",
			expectedGot,
			expectedWant,
		)

		if msg != expectedMsg {
			t.Errorf("message mismatch\ngot: %q\nwant: %q", msg, expectedMsg)
		}
	})
}

type CustomError struct{ Message string }

func (e CustomError) Error() string { return e.Message }

func row(label string, value any) string {
	return fmt.Sprintf("%-9s %s", label+":", pretty.Value(value))
}

func rowRaw(label, value string, args ...any) string {
	return fmt.Sprintf("%-9s %s", label+":", fmt.Sprintf(value, args...))
}
