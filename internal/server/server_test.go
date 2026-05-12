package server_test

import (
	"testing"

	"github.com/lyonbrown4d/spack/internal/server"
)

func TestShouldVaryAccept(t *testing.T) {
	tests := []struct {
		name            string
		sourceMediaType string
		explicitFormat  string
		expected        bool
	}{
		{
			name:            "image source without explicit format",
			sourceMediaType: "image/png",
			explicitFormat:  "",
			expected:        true,
		},
		{
			name:            "image source with explicit format",
			sourceMediaType: "image/png",
			explicitFormat:  "jpeg",
			expected:        false,
		},
		{
			name:            "non-image source",
			sourceMediaType: "text/html; charset=utf-8",
			explicitFormat:  "",
			expected:        false,
		},
		{
			name:            "media type normalization",
			sourceMediaType: " IMAGE/JPEG ",
			explicitFormat:  "",
			expected:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := server.ShouldVaryAcceptForTest(tt.sourceMediaType, tt.explicitFormat)
			if got != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}
