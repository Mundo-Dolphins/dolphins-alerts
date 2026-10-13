package injury

import (
	"testing"
)

func TestComputePlayersHash_IsDeterministicAndOrderIndependent(t *testing.T) {
	a := []InjuryPlayer{
		{Team: "A", Position: "QB", Name: "One", Injury: "Knee", Status: "Full", GameStatus: "Not specified"},
		{Team: "B", Position: "WR", Name: "Two", Injury: "Hamstring", Status: "Limited", GameStatus: "Probable"},
	}

	h1 := ComputePlayersHash(a)

	// re-order slice
	b := []InjuryPlayer{a[1], a[0]}
	h2 := ComputePlayersHash(b)

	if h1 != h2 {
		t.Fatalf("ComputePlayersHash should be order-independent but got %s and %s", h1, h2)
	}

	// identical input should produce same hash
	h3 := ComputePlayersHash(a)
	if h1 != h3 {
		t.Fatalf("ComputePlayersHash inconsistent across calls: %s != %s", h1, h3)
	}
}
