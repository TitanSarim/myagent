package router

import "testing"

func TestHeuristic(t *testing.T) {
	cases := []struct {
		prompt string
		want   Mode
	}{
		{"what is a mutex?", ModeFast},
		{"implement pagination for the orders API", ModeCode},
		{"refactor the auth middleware", ModeCode},
		{"design the architecture for a payment service", ModeSmart},
		{"review this migration plan", ModeSmart},
		{"fix the failing checkout test", ModeCode},
	}
	for _, tc := range cases {
		got := heuristic(tc.prompt)
		if got != tc.want {
			t.Errorf("heuristic(%q)=%s want %s", tc.prompt, got, tc.want)
		}
	}
}
