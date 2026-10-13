package injury

import (
	"strings"
	"testing"
)

func TestTranslateInjuryEdgeCases(t *testing.T) {
	// Empty or N/A
	if TranslateInjury("") != "No especificado" {
		t.Fatalf("expected No especificado for empty input")
	}
	if TranslateInjury("N/A") != "No especificado" {
		t.Fatalf("expected No especificado for N/A")
	}

	// Overlapping terms: ensure longer keys are matched first.
	// Example: "Achilles tendon" vs "Achilles" (both present in translations)
	in := "Right Achilles tendon strain"
	got := TranslateInjury(in)
	lower := strings.ToLower(got)
	if !strings.Contains(lower, "aquiles") && !strings.Contains(lower, "tendón") {
		t.Fatalf("expected translation for Achilles tendon, got: %s", got)
	}

	// Mixed case match
	in2 := "ANKLE Sprain"
	got2 := TranslateInjury(in2)
	if !strings.Contains(strings.ToLower(got2), "tobillo") && !strings.Contains(strings.ToLower(got2), "esguince") {
		t.Fatalf("expected ankle translation for %q, got %q", in2, got2)
	}
}
