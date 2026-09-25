package constants

import "testing"

func TestPressUnitTransitionGraph(t *testing.T) {
	if !CanTransition(PressUnitTransitions, "ready", "setup") {
		t.Fatalf("expected ready -> setup transition to be allowed")
	}
	if CanTransition(PressUnitTransitions, "ready", "unknown") {
		t.Fatal("unknown status must never be accepted")
	}
}
