package main

import "testing"

func TestTemperatureConversion(t *testing.T) {
	tempC := 28.5

	if got := round(tempC*1.8 + 32); got != 83.3 {
		t.Fatalf("fahrenheit = %v, want 83.3", got)
	}
	if got := round(tempC + 273.15); got != 301.65 {
		t.Fatalf("kelvin = %v, want 301.65", got)
	}
}
