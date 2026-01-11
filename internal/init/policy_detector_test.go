package init

import (
	"testing"
)

func TestGeneralizePathPatterns(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  map[string]bool // Use map for easier comparison (order doesn't matter)
	}{
		{
			name:  "empty paths",
			paths: []string{},
			want:  map[string]bool{"/*": true},
		},
		{
			name:  "numeric IDs",
			paths: []string{"/users/123", "/users/456"},
			want:  map[string]bool{"/users/*": true},
		},
		{
			name:  "UUIDs",
			paths: []string{"/items/550e8400-e29b-41d4-a716-446655440000"},
			want:  map[string]bool{"/items/*": true},
		},
		{
			name:  "mixed IDs",
			paths: []string{"/api/v1/users/123/posts/abc-def-123"},
			want:  map[string]bool{"/api/v1/users/*/posts/*": true},
		},
		{
			name:  "static paths",
			paths: []string{"/api/v1/chat/completions", "/api/v1/models"},
			want: map[string]bool{
				"/api/v1/chat/completions/*": true,
				"/api/v1/models/*":           true,
			},
		},
		{
			name:  "alphanumeric IDs with separators",
			paths: []string{"/orders/abc-def-123", "/orders/foo_bar_baz"},
			want:  map[string]bool{"/orders/*": true},
		},
		{
			name:  "very long alphanumeric (hash)",
			paths: []string{"/files/abcd1234efgh5678ijkl9012mnop3456qrst"},
			want:  map[string]bool{"/files/*": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GeneralizePathPatterns(tt.paths)

			// Convert to map for easier comparison
			gotMap := make(map[string]bool)
			for _, pattern := range got {
				gotMap[pattern] = true
			}

			// Check all expected patterns exist
			for expectedPattern := range tt.want {
				if !gotMap[expectedPattern] {
					t.Errorf("expected pattern %q not found in result", expectedPattern)
				}
			}

			// Check no unexpected patterns
			for gotPattern := range gotMap {
				if !tt.want[gotPattern] {
					t.Errorf("unexpected pattern %q in result", gotPattern)
				}
			}
		})
	}
}

func TestInferMaxBodyBytes(t *testing.T) {
	tests := []struct {
		name        string
		observedMax int64
		wantMin     int64 // Check result is at least this
		wantMax     int64 // Check result is at most this
	}{
		{
			name:        "zero observed (default)",
			observedMax: 0,
			wantMin:     1048576, // 1MB
			wantMax:     1048576,
		},
		{
			name:        "small body (100KB)",
			observedMax: 102400,
			wantMin:     122880,  // 120KB (100KB * 1.2)
			wantMax:     1048576, // Rounded up to 1MB
		},
		{
			name:        "medium body (500KB)",
			observedMax: 512000,
			wantMin:     614400,  // 600KB (500KB * 1.2)
			wantMax:     1048576, // Rounded up to 1MB
		},
		{
			name:        "large body (2MB)",
			observedMax: 2097152,
			wantMin:     2516582, // 2.4MB (2MB * 1.2)
			wantMax:     3145728, // Rounded up to 3MB
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferMaxBodyBytes(tt.observedMax)

			if got < tt.wantMin {
				t.Errorf("InferMaxBodyBytes(%d) = %d, want at least %d", tt.observedMax, got, tt.wantMin)
			}
			if got > tt.wantMax {
				t.Errorf("InferMaxBodyBytes(%d) = %d, want at most %d", tt.observedMax, got, tt.wantMax)
			}
		})
	}
}

func TestFormatBodySize(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{
			name:  "bytes",
			bytes: 500,
			want:  "500 bytes",
		},
		{
			name:  "kilobytes",
			bytes: 2048,
			want:  "2.00 KB",
		},
		{
			name:  "megabytes",
			bytes: 1048576,
			want:  "1.00 MB",
		},
		{
			name:  "gigabytes",
			bytes: 2147483648,
			want:  "2.00 GB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatBodySize(tt.bytes)
			if got != tt.want {
				t.Errorf("FormatBodySize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}
