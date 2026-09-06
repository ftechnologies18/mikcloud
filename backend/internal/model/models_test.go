package model_test

import (
	"testing"

	"mikcloud/hotspot-api/internal/model"
)

func TestNormalizeWifiPhone(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"0707080910", "2250707080910"},
		{"05 01 02 03 04", "2250501020304"},
		{"01-02-03-04-05", "2250102030405"},
		{"2250707080910", "2250707080910"},
		{"+2250707080910", "2250707080910"},
		{"12345678", "12345678"},
		{"123", ""}, // trop court
	}

	for _, tt := range tests {
		got := model.NormalizeWifiPhone(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeWifiPhone(%q) = %q; expected %q", tt.input, got, tt.expected)
		}
	}
}
