package clone

import "testing"

func TestMatchExclude(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		relPath string
		want    bool
	}{
		// Basename matching (no / in pattern)
		{"basename wildcard matches file", "*.lock", "yarn.lock", true},
		{"basename wildcard matches nested file", "*.lock", "packages/foo/yarn.lock", true},
		{"basename wildcard no match", "*.lock", "yarn.txt", false},
		{"basename exact matches dir", "__pycache__", "__pycache__", true},
		{"basename exact matches nested dir", "__pycache__", "src/lib/__pycache__", true},
		{"basename exact no match", "__pycache__", "pycache", false},

		// Path matching (/ in pattern)
		{"path pattern matches exact", ".gradle/configuration-cache", ".gradle/configuration-cache", true},
		{"path pattern no match at wrong depth", ".gradle/configuration-cache", "sub/.gradle/configuration-cache", false},
		{"path pattern no match partial", ".gradle/configuration-cache", ".gradle/caches", false},

		// Edge cases
		{"empty relPath", "*.lock", "", false},
		{"root file basename match", "*.lock", "package.lock", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchExclude(tt.pattern, tt.relPath)
			if got != tt.want {
				t.Errorf("matchExclude(%q, %q) = %v, want %v", tt.pattern, tt.relPath, got, tt.want)
			}
		})
	}
}

func TestIsExcluded(t *testing.T) {
	excludes := []string{"*.lock", "__pycache__", ".gradle/configuration-cache"}

	tests := []struct {
		name    string
		relPath string
		want    bool
	}{
		{"matches wildcard", "packages/yarn.lock", true},
		{"matches basename", "src/__pycache__", true},
		{"matches path", ".gradle/configuration-cache", true},
		{"no match", "src/main.go", false},
		{"grove dir never excluded", ".grove", false},
		{"grove subpath never excluded", ".grove/config.json", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isExcluded(tt.relPath, excludes)
			if got != tt.want {
				t.Errorf("isExcluded(%q, ...) = %v, want %v", tt.relPath, got, tt.want)
			}
		})
	}
}
