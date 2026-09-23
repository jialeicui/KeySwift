package statemachine

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSameInputDevice(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		actual   string
		want     bool
	}{
		{
			name:     "same device name",
			expected: "HHKB-Hybrid_1 Keyboard",
			actual:   "HHKB-Hybrid_1 Keyboard",
			want:     true,
		},
		{
			name:     "event node reused by another device",
			expected: "HHKB-Hybrid_1 Keyboard",
			actual:   "MX Anywhere 3",
			want:     false,
		},
		{
			name:     "empty actual name",
			expected: "HHKB-Hybrid_1 Keyboard",
			actual:   "",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isSameInputDevice(tt.expected, tt.actual))
		})
	}
}
