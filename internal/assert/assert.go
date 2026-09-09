package assert

import "testing"

func Equal[V comparable](t *testing.T, actual, expected V) {
	t.Helper()
	if actual != expected {
		t.Errorf("got: %v; want: %v", actual, expected)
	}
}
