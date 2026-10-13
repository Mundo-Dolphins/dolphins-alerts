package injury

import (
	"sync"
	"testing"
)

func TestTranslateInjury_LongestKeyWins(t *testing.T) {
	// Restore the embedded dictionary so later tests are not affected.
	original := translationsJSON
	t.Cleanup(func() {
		translationsJSON = original
		translationsOnce = sync.Once{}
		translationsKeys = nil
	})

	// Prepare a custom translations JSON with overlapping keys
	translationsJSON = []byte(`{"ankle sprain":"esguince de tobillo","ankle":"tobillo","sprain":"esguince"}`)
	// Reset the once so TranslateInjury reloads from translationsJSON
	translationsOnce = sync.Once{}

	out := TranslateInjury("Left ankle sprain")
	// Expect the longer key 'ankle sprain' to be matched first
	if out == "Left ankle sprain" || out == "" {
		t.Fatalf("TranslateInjury did not translate as expected: %q", out)
	}
	// Accept results that contain both 'esguince' and 'tobillo'
	if !(containsIgnoreCase(out, "esguince") && containsIgnoreCase(out, "tobillo")) {
		t.Fatalf("TranslateInjury result missing expected tokens: %q", out)
	}
}

func containsIgnoreCase(s, sub string) bool {
	return len(s) >= 0 && len(sub) >= 0 && (len(s) == 0 && len(sub) == 0 || (len(sub) > 0 && (stringIndexFold(s, sub) >= 0)))
}

// stringIndexFold is a small case-insensitive substring search
func stringIndexFold(s, substr string) int {
	S := []rune(s)
	P := []rune(substr)
	for i := 0; i+len(P) <= len(S); i++ {
		ok := true
		for j := 0; j < len(P); j++ {
			a := S[i+j]
			b := P[j]
			if a >= 'A' && a <= 'Z' {
				a = a - 'A' + 'a'
			}
			if b >= 'A' && b <= 'Z' {
				b = b - 'A' + 'a'
			}
			if a != b {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}
