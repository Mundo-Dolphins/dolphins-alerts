package espn

import "testing"

func TestFahrenheitToCelsius(t *testing.T) {
	tests := []struct {
		f    int
		want int
	}{
		{32, 0},
		{77, 25},
		{86, 30},
		{0, -17},
	}

	for _, tt := range tests {
		got := fahrenheitToCelsius(tt.f)
		if got != tt.want {
			t.Fatalf("fahrenheitToCelsius(%d) = %d; want %d", tt.f, got, tt.want)
		}
	}
}

func TestTranslateWeather_WithTemperatureToken(t *testing.T) {
	w := &Weather{DisplayValue: "Partly cloudy 75°F", Temperature: 75}
	got := TranslateWeather(w)
	want := "Parcialmente nublado (75°F / 23°C)"
	if got != want {
		t.Fatalf("TranslateWeather() = %q; want %q", got, want)
	}
}

func TestTranslateWeather_AddTemperatureWhenNotPresent(t *testing.T) {
	w := &Weather{DisplayValue: "Sunny", Temperature: 86}
	got := TranslateWeather(w)
	want := "Soleado (86°F / 30°C)"
	if got != want {
		t.Fatalf("TranslateWeather() = %q; want %q", got, want)
	}
}

func TestTranslateScoringType(t *testing.T) {
	if got := TranslateScoringType("Field Goal"); got != "Gol de campo" {
		t.Fatalf("TranslateScoringType(Field Goal) = %q; want %q", got, "Gol de campo")
	}

	// Unknown type should be returned as-is
	if got := TranslateScoringType("WeirdScore"); got != "WeirdScore" {
		t.Fatalf("TranslateScoringType(WeirdScore) = %q; want %q", got, "WeirdScore")
	}
}
