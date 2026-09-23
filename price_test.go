package cexfind

import "testing"

func TestParsePriceRange(t *testing.T) {
	for _, tc := range []struct {
		min, max string
		valid    bool
	}{
		{"", "", true},
		{"0", "", true},
		{"", "100.50", true},
		{"25.50", "100", true},
		{"20", "20", true},
		{"-1", "", false},
		{"", "-1", false},
		{"abc", "", false},
		{"", "abc", false},
		{"100", "25", false},
	} {
		_, err := ParsePriceRange(tc.min, tc.max)
		if (err == nil) != tc.valid {
			t.Errorf("ParsePriceRange(%q, %q) err = %v, valid = %t", tc.min, tc.max, err, tc.valid)
		}
	}
}
