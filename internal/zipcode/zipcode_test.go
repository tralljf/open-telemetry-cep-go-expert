package zipcode

import "testing"

func TestValid(t *testing.T) {
	tests := []struct {
		name string
		cep  string
		want bool
	}{
		{name: "valid", cep: "29902555", want: true},
		{name: "short", cep: "2990255", want: false},
		{name: "long", cep: "299025555", want: false},
		{name: "letters", cep: "29902A55", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Valid(tt.cep); got != tt.want {
				t.Fatalf("Valid(%q) = %v, want %v", tt.cep, got, tt.want)
			}
		})
	}
}
