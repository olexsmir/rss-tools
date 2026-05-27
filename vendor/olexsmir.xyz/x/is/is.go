package is

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Equal asserts that two values are equal.
func Equal[T any](tb testing.TB, expected, got T) {
	tb.Helper()

	if !areEqual(expected, got) {
		tb.Errorf("expected: %#v, got: %#v", expected, got)
	}
}

// NotEqual asserts that two values are not equal.
func NotEqual[T any](tb testing.TB, expected, got T) {
	tb.Helper()

	if areEqual(expected, got) {
		tb.Errorf("expected values to be different, but both are: %#v", got)
	}
}

// Err asserts error conditions with flexible expected values:
//   - nil: asserts no error occurred
//   - string: asserts error message contains the string
//   - error: asserts errors.Is matches
//   - reflect.Type: asserts errors.As succeeds
func Err(tb testing.TB, got error, expected any) {
	tb.Helper()

	if expected != nil && got == nil {
		tb.Error("got: <nil>, expected: error")
		return
	}

	switch e := expected.(type) {
	case nil:
		if got != nil {
			tb.Fatalf("unexpected error: %v", got)
		}

	case string:
		if !strings.Contains(got.Error(), e) {
			tb.Fatalf("expected: %q, got: %q", got.Error(), e)
		}

	case error:
		if !errors.Is(got, e) {
			tb.Fatalf("expected: %T(%v), got: %T(%v)", got, got, e, e)
		}

	case reflect.Type:
		target := reflect.New(e).Interface()
		if !errors.As(got, target) {
			tb.Fatalf("expected: %s, got: %T", e, got)
		}

	default:
		tb.Fatalf("unexpected type: %T", expected)
	}
}

type equaler[T any] interface{ Equal(T) bool }

func areEqual[T any](a, b T) bool {
	if isNil(a) && isNil(b) {
		return true
	}

	// some types provide .Equal(like time.Time, net.IP)
	if eq, ok := any(a).(equaler[T]); ok {
		return eq.Equal(b)
	}
	if aBytes, ok := any(a).([]byte); ok {
		bBytes := any(b).([]byte)
		return bytes.Equal(aBytes, bBytes)
	}

	return reflect.DeepEqual(a, b)
}

func isNil(v any) bool {
	if v == nil {
		return true
	}

	// non-nil interface can hold a nil value, check the underlying value.
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return rv.IsNil()
	default:
		return false
	}
}
