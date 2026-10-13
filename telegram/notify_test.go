package telegram

import "testing"

func TestUrlEncode(t *testing.T) {
	// compose string with percent, newline, space and ampersand
	s := "%\n &"
	got := urlEncode(s)
	want := "%25%0A%20%26"
	if got != want {
		t.Fatalf("urlEncode=%q, want=%q", got, want)
	}
}
