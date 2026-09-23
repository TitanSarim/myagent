package router

import "testing"

func TestHeuristic(t *testing.T) {
	cases := []struct {
		prompt string
		sig    Signals
		want   Mode
	}{
		{"what is a mutex?", Signals{}, ModeFast},
		{"implement pagination for the orders API", Signals{}, ModeCode},
		{"refactor the auth middleware", Signals{}, ModeCode},
		{"design the architecture for a payment service", Signals{}, ModeSmart},
		{"review this migration plan", Signals{}, ModeSmart},
		{"fix the failing checkout test", Signals{}, ModeCode},
		{"look at this", Signals{FailingTests: 3}, ModeCode},
		{"explain briefly", Signals{}, ModeFast},
	}
	for _, tc := range cases {
		got, _ := heuristic(tc.prompt, tc.sig)
		if got != tc.want {
			t.Errorf("heuristic(%q,%+v)=%s want %s", tc.prompt, tc.sig, got, tc.want)
		}
	}
}
