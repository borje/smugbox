package httpx

import "testing"

func TestAccepts(t *testing.T) {
	tests := []struct {
		header string
		coding string
		want   bool
	}{
		{"", "gzip", false},
		{"gzip", "gzip", true},
		{"gzip", "br", false},
		{"zstd, gzip", "zstd", true},
		{"zstd, gzip", "gzip", true},
		{"gzip, deflate, br", "br", true},
		{"GZIP", "gzip", true},
		{" gzip ", "gzip", true},
		{"gzip;q=0.5", "gzip", true},
		{"gzip;q=0", "gzip", false},
		{"gzip;q=0.0", "gzip", false},
		{"*", "br", true},
		{"*;q=0", "br", false},
		// An explicit entry wins over the wildcard either way round.
		{"*, gzip;q=0", "gzip", false},
		{"*;q=0, gzip", "gzip", true},
		{"identity", "gzip", false},
		{"gzip;q=0.5;x=1", "gzip", true},
		// A malformed qvalue falls back to q=1 rather than rejecting.
		{"gzip;q=nonsense", "gzip", true},
	}
	for _, tt := range tests {
		if got := Accepts(tt.header, tt.coding); got != tt.want {
			t.Errorf("Accepts(%q, %q) = %v, want %v", tt.header, tt.coding, got, tt.want)
		}
	}
}
