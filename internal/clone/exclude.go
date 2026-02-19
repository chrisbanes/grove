package clone

import (
	"fmt"
	"io/fs"
	"os"
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

// clonePlan holds the results of walking the source tree with exclude patterns.
type clonePlan struct {
	// totalEntries is the count of non-excluded entries (for progress reporting).
	totalEntries int
	// dirsWithExcludes maps relative directory paths that contain excluded
	// descendants. The key "." represents the source root.
	dirsWithExcludes map[string]bool
}

// buildClonePlan walks src and computes which entries are excluded.
func buildClonePlan(src string, excludes []string) (*clonePlan, error) {
	plan := &clonePlan{
		dirsWithExcludes: make(map[string]bool),
	}
	if len(excludes) == 0 {
		count, err := countAllEntries(src)
		if err != nil {
			return nil, err
		}
		plan.totalEntries = count
		return plan, nil
	}

	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			plan.totalEntries++
			return nil
		}
		if isExcluded(rel, excludes) {
			markAncestors(rel, plan.dirsWithExcludes)
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		plan.totalEntries++
		return nil
	})
	return plan, err
}

func countAllEntries(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(_ string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}

func markAncestors(rel string, dirs map[string]bool) {
	for {
		parent := filepath.Dir(rel)
		if parent == "." || parent == rel {
			dirs["."] = true
			return
		}
		dirs[parent] = true
		rel = parent
	}
}

// SelectiveClone clones src to dst, excluding paths matching the given globs.
// If excludes is empty, falls back to a single full clone.
func SelectiveClone(cloner Cloner, src, dst string, excludes []string) error {
	if len(excludes) == 0 {
		return cloner.Clone(src, dst)
	}

	plan, err := buildClonePlan(src, excludes)
	if err != nil {
		return fmt.Errorf("planning clone: %w", err)
	}

	return executeClonePlan(cloner, src, dst, ".", excludes, plan)
}

// executeClonePlan recursively clones children of srcDir into dstDir,
// skipping excluded entries and recursing into directories that contain excludes.
func executeClonePlan(cloner Cloner, srcRoot, dstRoot, rel string, excludes []string, plan *clonePlan) error {
	srcDir := filepath.Join(srcRoot, rel)
	dstDir := filepath.Join(dstRoot, rel)

	if rel == "." {
		srcDir = srcRoot
		dstDir = dstRoot
	}

	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		childRel := filepath.Join(rel, entry.Name())
		if rel == "." {
			childRel = entry.Name()
		}

		if isExcluded(childRel, excludes) {
			continue
		}

		childSrc := filepath.Join(srcDir, entry.Name())
		childDst := filepath.Join(dstDir, entry.Name())

		if entry.IsDir() && plan.dirsWithExcludes[childRel] {
			if err := executeClonePlan(cloner, srcRoot, dstRoot, childRel, excludes, plan); err != nil {
				return err
			}
			continue
		}

		// Fast path: clone the entire entry with a single cp -c -R
		if err := cloner.Clone(childSrc, childDst); err != nil {
			return err
		}
	}
	return nil
}
