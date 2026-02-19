package clone

import (
	"path/filepath"
	"strings"
)

// matchExclude checks whether relPath matches a single exclude pattern.
// If the pattern contains no /, it matches against the basename.
// If the pattern contains /, it matches against the full relative path.
func matchExclude(pattern, relPath string) bool {
	if relPath == "" {
		return false
	}
	if strings.Contains(pattern, "/") {
		matched, _ := filepath.Match(pattern, relPath)
		return matched
	}
	matched, _ := filepath.Match(pattern, filepath.Base(relPath))
	return matched
}

// isExcluded checks whether relPath matches any of the exclude patterns.
// The .grove directory is never excluded.
func isExcluded(relPath string, excludes []string) bool {
	if relPath == ".grove" || strings.HasPrefix(relPath, ".grove/") || strings.HasPrefix(relPath, ".grove\\") {
		return false
	}
	for _, pattern := range excludes {
		if matchExclude(pattern, relPath) {
			return true
		}
	}
	return false
}
