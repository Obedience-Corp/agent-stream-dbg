package config

import (
	"os"
	"testing"
)

func TestGetEnv(t *testing.T) {
	_ = os.Setenv("TEST_VAR", "test-value")
	defer func() { _ = os.Unsetenv("TEST_VAR") }()

	value := getEnv("TEST_VAR", "default")
	if value != "test-value" {
		t.Errorf("Expected 'test-value', got '%s'", value)
	}

	value = getEnv("NON_EXISTENT", "default")
	if value != "default" {
		t.Errorf("Expected 'default', got '%s'", value)
	}
}

func TestGetEnvBool(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"1", true},
		{"false", false},
		{"False", false},
		{"FALSE", false},
		{"0", false},
		{"invalid", true}, // Falls back to default
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			_ = os.Setenv("TEST_BOOL", tt.value)
			defer func() { _ = os.Unsetenv("TEST_BOOL") }()

			result := getEnvBool("TEST_BOOL", true)
			if result != tt.expected {
				t.Errorf("For value '%s', expected %v, got %v", tt.value, tt.expected, result)
			}
		})
	}
}

func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		value    string
		expected int
	}{
		{"42", 42},
		{"0", 0},
		{"-5", -5},
		{"invalid", 10}, // Falls back to default
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			_ = os.Setenv("TEST_INT", tt.value)
			defer func() { _ = os.Unsetenv("TEST_INT") }()

			result := getEnvInt("TEST_INT", 10)
			if result != tt.expected {
				t.Errorf("For value '%s', expected %d, got %d", tt.value, tt.expected, result)
			}
		})
	}
}
