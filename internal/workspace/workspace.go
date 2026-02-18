package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chrisbanes/grove/internal/clone"
	"github.com/chrisbanes/grove/internal/config"
)

// Info holds metadata about a workspace.
type Info struct {
	ID           string    `json:"id"`
	GoldenCopy   string    `json:"golden_copy"`
	GoldenCommit string    `json:"golden_commit"`
	CreatedAt    time.Time `json:"created_at"`
	Branch       string    `json:"branch"`
	Path         string    `json:"path"`
}

// CreateOpts holds options for creating a workspace.
type CreateOpts struct {
	Branch       string
	GoldenCommit string
}

// Create makes a new workspace by CoW-cloning the golden copy.
func Create(goldenRoot string, cfg *config.Config, cloner clone.Cloner, opts CreateOpts) (*Info, error) {
	// Check max workspace limit
	existing, err := List(cfg)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(existing) >= cfg.MaxWorkspaces {
		return nil, fmt.Errorf("max workspaces (%d) reached — destroy one first", cfg.MaxWorkspaces)
	}

	id, err := GenerateID(opts.Branch)
	if err != nil {
		return nil, fmt.Errorf("generating workspace ID: %w", err)
	}

	wsPath := filepath.Join(cfg.WorkspaceDir, id)

	// Ensure parent directory exists
	if err := os.MkdirAll(cfg.WorkspaceDir, 0755); err != nil {
		return nil, fmt.Errorf("creating workspace directory: %w", err)
	}

	// CoW clone
	if err := cloner.Clone(goldenRoot, wsPath); err != nil {
		os.RemoveAll(wsPath) // clean up partial clone
		return nil, fmt.Errorf("clone failed: %w", err)
	}

	info := &Info{
		ID:           id,
		GoldenCopy:   goldenRoot,
		GoldenCommit: opts.GoldenCommit,
		CreatedAt:    time.Now().UTC(),
		Branch:       opts.Branch,
		Path:         wsPath,
	}

	// Write workspace marker
	if err := writeMarker(wsPath, info); err != nil {
		os.RemoveAll(wsPath)
		return nil, fmt.Errorf("writing workspace marker: %w", err)
	}

	return info, nil
}

// List returns all workspaces in the configured workspace directory.
func List(cfg *config.Config) ([]Info, error) {
	entries, err := os.ReadDir(cfg.WorkspaceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var workspaces []Info
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		wsPath := filepath.Join(cfg.WorkspaceDir, entry.Name())
		info, err := readMarker(wsPath)
		if err != nil {
			continue // skip directories without valid markers
		}
		info.Path = wsPath
		workspaces = append(workspaces, *info)
	}
	return workspaces, nil
}

// Destroy removes a workspace by ID or path.
func Destroy(cfg *config.Config, idOrPath string) error {
	wsPath, err := resolveWorkspace(cfg, idOrPath)
	if err != nil {
		return err
	}
	return os.RemoveAll(wsPath)
}

// Get returns info for a workspace by ID or path.
func Get(cfg *config.Config, idOrPath string) (*Info, error) {
	wsPath, err := resolveWorkspace(cfg, idOrPath)
	if err != nil {
		return nil, err
	}
	info, err := readMarker(wsPath)
	if err != nil {
		return nil, err
	}
	info.Path = wsPath
	return info, nil
}

// IsWorkspace returns true if path contains a .grove/workspace.json marker.
func IsWorkspace(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".grove", config.WorkspaceFile))
	return err == nil
}

// resolveWorkspace finds a workspace path from an ID or path.
func resolveWorkspace(cfg *config.Config, idOrPath string) (string, error) {
	// Try as direct path
	if filepath.IsAbs(idOrPath) {
		if IsWorkspace(idOrPath) {
			return idOrPath, nil
		}
		return "", fmt.Errorf("not a grove workspace: %s", idOrPath)
	}
	// Try as ID
	wsPath := filepath.Join(cfg.WorkspaceDir, idOrPath)
	if IsWorkspace(wsPath) {
		return wsPath, nil
	}
	return "", fmt.Errorf("workspace not found: %s", idOrPath)
}

func writeMarker(wsPath string, info *Info) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	markerPath := filepath.Join(wsPath, ".grove", config.WorkspaceFile)
	return os.WriteFile(markerPath, data, 0644)
}

func readMarker(wsPath string) (*Info, error) {
	data, err := os.ReadFile(filepath.Join(wsPath, ".grove", config.WorkspaceFile))
	if err != nil {
		return nil, err
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// Slugify converts a branch name to a URL/filesystem-safe slug.
// Lowercases, replaces non-alphanumeric chars with hyphens, collapses
// consecutive hyphens, truncates to 20 chars, and trims trailing hyphens.
func Slugify(branch string) string {
	if branch == "" {
		return ""
	}
	var b strings.Builder
	branch = strings.ToLower(branch)
	prev := false
	for _, r := range branch {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			prev = false
		} else if !prev && b.Len() > 0 {
			b.WriteByte('-')
			prev = true
		}
	}
	s := b.String()
	s = strings.TrimRight(s, "-")
	if len(s) > 20 {
		s = s[:20]
		s = strings.TrimRight(s, "-")
	}
	return s
}

// GenerateID creates a workspace ID. If branch is non-empty, the ID is
// "{branch-slug}-{4-hex}". Otherwise it is just "{4-hex}".
func GenerateID(branch string) (string, error) {
	bytes := make([]byte, 2)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	suffix := hex.EncodeToString(bytes)
	slug := Slugify(branch)
	if slug == "" {
		return suffix, nil
	}
	return slug + "-" + suffix, nil
}
