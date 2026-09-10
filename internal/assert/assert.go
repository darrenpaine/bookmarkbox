package assert

import (
	"strings"
	"testing"
)

// Check values are equal and log if not
func Equal[V comparable](t *testing.T, actual, expected V) {
	t.Helper()
	if actual != expected {
		t.Errorf("got: %v; want: %v", actual, expected)
	}
}

// Check for a substring and log if not
func StringContains(t *testing.T, actual, expectedSubstring string) {
	t.Helper()
	if !strings.Contains(actual, expectedSubstring) {
		t.Errorf("got: %q; expected to contain: %q", actual, expectedSubstring)
	}
}

// Check for error content
func NilError(t *testing.T, actual error) {
	t.Helper()
	if actual != nil {
		t.Errorf("got: %v; expected: nil", actual)
	}
}
