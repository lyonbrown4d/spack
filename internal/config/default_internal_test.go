package config

import "testing"

func TestMemoryCacheMaxBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		memoryLimit int64
		want        int64
	}{
		{
			name:        "unknown memory uses fallback",
			memoryLimit: 0,
			want:        memoryCacheFallbackBytes,
		},
		{
			name:        "small memory clamps to minimum",
			memoryLimit: 64 * 1024 * 1024,
			want:        memoryCacheMinimumBytes,
		},
		{
			name:        "medium memory uses one sixteenth",
			memoryLimit: 1024 * 1024 * 1024,
			want:        64 * 1024 * 1024,
		},
		{
			name:        "large memory clamps to maximum",
			memoryLimit: 64 * 1024 * 1024 * 1024,
			want:        memoryCacheMaximumBytes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := memoryCacheMaxBytes(tt.memoryLimit); got != tt.want {
				t.Fatalf("memoryCacheMaxBytes(%d) = %d, want %d", tt.memoryLimit, got, tt.want)
			}
		})
	}
}
