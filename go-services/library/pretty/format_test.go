package pretty_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"go-services/library/pretty"
)

// MockStringer implements fmt.Stringer
type MockStringer struct {
	Msg string
}

func (m MockStringer) String() string {
	return "Stringer: " + m.Msg
}

// simpleStruct for testing basic struct formatting
type simpleStruct struct {
	Name  string
	Age   int
	admin bool // Unexported, should be ignored
}

// recursiveStruct for testing circular detection
type recursiveStruct struct {
	Next *recursiveStruct
	Val  int
}

func TestValue_Basic(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		// 1. Primitives
		{"Integer", 123, "123"},
		{"Float", 123.456, "123.456"},
		{"Bool True", true, "true"},
		{"Bool False", false, "false"},
		{"String", "hello world", `"hello world"`},

		// 2. Collections (Nil vs Empty)
		{"Nil Slice", []int(nil), "<nil>"},
		{"Empty Slice", []int{}, "[]int{}"},
		{"Populated Slice", []int{1, 2}, "[]int{1, 2}"},

		{"Nil Map", map[string]int(nil), "<nil>"},
		{"Empty Map", map[string]int{}, "map[string]int{}"},

		// 3. Pointers & Interfaces
		{"Nil Pointer", (*int)(nil), "<nil>"},
		{"Nil Interface", nil, "<nil>"},
		{"Pointer to Int", func() *int { i := 5; return &i }(), "&5"},

		// 4. Structs
		{
			"Struct",
			simpleStruct{Name: "Alice", Age: 30, admin: true},
			"pretty_test.simpleStruct{Name: \"Alice\", Age: 30}",
		},

		// 5. Time
		{"Time", time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC), "2023-01-01T12:00:00Z"},

		// 6. Stringer Interface
		{"Stringer", MockStringer{Msg: "Test"}, "Stringer: Test"},

		// 7. Channels
		{"Nil Chan", (chan int)(nil), "<nil chan>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pretty.Value(tt.input)
			if got != tt.expected {
				t.Errorf("Value() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestArrays specifically targets the potential Panic bug with IsNil on Arrays
func TestArrays(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Code panicked on Array test! Did you fix the v.IsNil() check? Panic: %v", r)
		}
	}()

	input := [2]int{1, 2}
	expected := "[2]int{1, 2}"
	got := pretty.Value(input)

	if got != expected {
		t.Errorf("Array formatting failed. Got %s, want %s", got, expected)
	}
}

func TestValue_Maps(t *testing.T) {
	// Maps are non-deterministic, so we check if the output contains expected key-values
	input := map[string]int{"a": 1, "b": 2}
	got := pretty.Value(input)

	if !strings.HasPrefix(got, "map[string]int{") || !strings.HasSuffix(got, "}") {
		t.Fatalf("Map output format wrong: %s", got)
	}

	// Check content regardless of order
	if !strings.Contains(got, "\"a\": 1") || !strings.Contains(got, "\"b\": 2") {
		t.Errorf("Map output missing keys/values: %s", got)
	}
}

func TestValue_Circularity(t *testing.T) {
	// Create a self-referencing struct
	n1 := &recursiveStruct{Val: 1, Next: nil}
	n1.Next = n1

	got := pretty.Value(n1)

	// Expect: &recursiveStruct{Next: <circular>, Val: 1} (order of fields depends on definition)
	if !strings.Contains(got, "<circular>") {
		t.Errorf("Failed to detect circularity. Got: %s", got)
	}
}

func TestValue_SharedBackingArray(t *testing.T) {
	// Ensure that two different slices sharing the same array are NOT marked circular
	// This tests the "reflect.Slice" removal from the visited check
	arr := [5]int{1, 2, 3, 4, 5}
	s1 := arr[:]
	s2 := arr[:]

	container := []any{s1, s2}
	got := pretty.Value(container)

	// If buggy, it might print: [[1, 2, 3, 4, 5], <circular>]
	// Correct: [[1, 2, 3, 4, 5], [1, 2, 3, 4, 5]]
	if strings.Contains(got, "<circular>") {
		t.Error("False positive circularity detected on shared slice backing arrays")
	}
}

func TestValueOpt_Limits(t *testing.T) {
	cfg := pretty.NewConfig(
		pretty.WithMaxDepth(2),
		pretty.WithSliceLimit(2),
		pretty.WithMaxDepth(2),
		pretty.WithStructFieldsLimit(2),
		pretty.WithBytesLimit(10),
		pretty.WithSensitiveFields("password"),
	)

	t.Run("MaxDepth", func(t *testing.T) {
		// Depth 0 -> 1 -> 2 -> 3 (should stop)
		type Deep struct {
			Next *Deep
		}
		d := &Deep{Next: &Deep{Next: &Deep{Next: &Deep{}}}}
		got := pretty.ValueOpt(d, cfg)
		if !strings.Contains(got, "...") {
			t.Errorf("MaxDepth failed, got: %s", got)
		}
	})

	t.Run("SliceTruncation", func(t *testing.T) {
		s := []int{1, 2, 3, 4, 5}
		got := pretty.ValueOpt(s, cfg)
		// Expect [1, 2, ... and 3 more]
		if !strings.Contains(got, "and 3 more") {
			t.Errorf("Slice truncation failed: %s", got)
		}
	})

	t.Run("StringTruncation", func(t *testing.T) {
		s := "This is a very long string"
		got := pretty.ValueOpt(s, cfg)
		// Expect "This is a ... [truncated]"
		if !strings.Contains(got, "[truncated]") {
			t.Errorf("String truncation failed: %s", got)
		}
	})

	t.Run("SensitiveFields", func(t *testing.T) {
		type User struct {
			Username string
			Password string
		}
		u := User{Username: "admin", Password: "supersecret"}
		got := pretty.ValueOpt(u, cfg)

		if !strings.Contains(got, "<REDACTED>") {
			t.Error("Sensitive field was not redacted")
		}
		if strings.Contains(got, "supersecret") {
			t.Error("Leaked sensitive data")
		}
	})
}

// TestUTF8Safety checks if the code produces invalid strings when truncating multibyte chars
func TestUTF8Safety(t *testing.T) {
	cfg := pretty.NewConfig(pretty.WithBytesLimit(4))
	// "世" is 3 bytes (E4 B8 96). "界" is 3 bytes.
	// String is 6 bytes total.
	// MaxBytes 4 cuts the second character "界" in the middle (1 byte of it).
	// Standard slice s[:4] leaves a partial rune.
	s := "世界"

	// We just want to ensure it doesn't look completely broken or panic
	// Ideally, your implementation should fix this, but the test here documents current behavior.
	if got, want := pretty.ValueOpt(s, cfg), "\"世... [truncated]\""; got != want {
		// t.Errorf("UTF8 Truncation Result: %s", got)
		t.Errorf("got: %q\nwant: %q", got, want)
	}
}

type Account struct {
	ID        int
	Username  string
	Email     string
	IsActive  bool
	Metadata  map[string]string
	Friends   []int
	CreatedAt time.Time
	Settings  *Settings
}

type Settings struct {
	Theme       string
	Permissions []string
	LastLogin   *time.Time
}

func createComplexObject() Account {
	now := time.Now()
	return Account{
		ID:       12345,
		Username: "gopher_expert",
		Email:    "gopher@example.com",
		IsActive: true,
		Metadata: map[string]string{
			"region": "us-east-1",
			"tier":   "pro",
		},
		Friends:   []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, // Triggers slice limit
		CreatedAt: now,
		Settings: &Settings{
			Theme:       "dark",
			Permissions: []string{"read", "write", "exec"},
			LastLogin:   &now,
		},
	}
}

var (
	obj       = createComplexObject()
	resultStr string
)

// Benchmark Standard fmt.Sprintf
func BenchmarkStandardSprintf(b *testing.B) {
	for b.Loop() {
		resultStr = fmt.Sprintf("%#v", obj)
	}
}

// Benchmark Your Pretty.Value (with default pool-based config)
func BenchmarkPrettyValue(b *testing.B) {
	for b.Loop() {
		resultStr = pretty.Value(obj)
	}
}
