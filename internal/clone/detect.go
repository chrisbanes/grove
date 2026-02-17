package clone

import (
	"fmt"
	"runtime"
	"strings"
)

// NewCloner returns the appropriate Cloner for the current platform
// and filesystem. Returns an error if CoW is not supported.
func NewCloner(path string) (Cloner, error) {
	switch runtime.GOOS {
	case "darwin":
		ok, err := isAPFS(path)
		if err != nil {
			return nil, fmt.Errorf("filesystem detection failed: %w", err)
		}
		if !ok {
			return nil, fmt.Errorf(
				"filesystem at %s does not support copy-on-write clones.\n"+
					"Grove requires APFS (macOS) or Btrfs/XFS with reflink support (Linux)", path)
		}
		return &APFSCloner{}, nil
	case "linux":
		return nil, fmt.Errorf("linux reflink support not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// helper functions

func splitLines(s string) []string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	var result []string
	for _, l := range lines {
		if l != "" {
			result = append(result, l)
		}
	}
	return result
}

func splitFields(s string) []string {
	return strings.Fields(s)
}

func containsString(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
