package httpapi

import "testing"

func TestEventPageLimit(t *testing.T) {
	for input, expected := range map[string]int{"": 100, "1": 1, "500": 500} {
		if got, valid := eventPageLimit(input); !valid || got != expected {
			t.Fatalf("eventPageLimit(%q) = %d, %t", input, got, valid)
		}
	}
	for _, input := range []string{"0", "501", "nope"} {
		if _, valid := eventPageLimit(input); valid {
			t.Fatalf("eventPageLimit(%q) accepted invalid value", input)
		}
	}
}
