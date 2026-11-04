package greet

import "testing"

func TestGreet(t *testing.T) {
	if got := Greet("Go"); got != "Hello, Go" {
		t.Errorf("got %q, want %q", got, "Hello, Go")
	}
}
