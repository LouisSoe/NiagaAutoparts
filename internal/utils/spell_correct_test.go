package utils

import (
	"testing"
)

func TestCorrectWord(t *testing.T) {
	dictionary := []string{"kampas", "filter", "oli", "busi", "v-belt"}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Exact match",
			input:    "kampas",
			expected: "kampas",
		},
		{
			name:     "1 distance typo (kampes -> kampas)",
			input:    "kampes",
			expected: "kampas",
		},
		{
			name:     "1 distance typo (flter -> filter)",
			input:    "flter",
			expected: "filter",
		},
		{
			name:     "Out of threshold distance (xyz123 -> xyz123)",
			input:    "xyz123",
			expected: "xyz123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CorrectWord(tt.input, dictionary)
			if got != tt.expected {
				t.Errorf("CorrectWord(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}
