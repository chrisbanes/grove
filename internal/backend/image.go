package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chrisbanes/grove/internal/config"
	"github.com/chrisbanes/grove/internal/image"
	"github.com/chrisbanes/grove/internal/workspace"
)

type imageBackend struct{}

func (imageBackend) Name() string {
	return "image"
}

func (imageBackend) CreateWorkspace(goldenRoot string, cfg *config.Config, opts CreateOptions) (*workspace.Info, error) {
	st, err := image.LoadState(goldenRoot)
	if err != nil {
		return nil, fmt.Errorf("loading image backend state: %w", err)
	}

	existing, err := workspace.List(cfg)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(existing) >= cfg.MaxWorkspaces {
		return nil, fmt.Errorf("max workspaces (%d) reached — destroy one first", cfg.MaxWorkspaces)
	}

	id, err := workspace.GenerateID(opts.BranchForID)
	if err != nil {
		return nil, fmt.Errorf("generating workspace ID: %w", err)
	}
	wsPath := filepath.Join(cfg.WorkspaceDir, id)

	if err := os.MkdirAll(cfg.WorkspaceDir, 0755); err != nil {
		return nil, fmt.Errorf("creating workspace directory: %w", err)
	}

	if _, err := image.CreateWorkspace(goldenRoot, goldenRoot, wsPath, id, st, nil); err != nil {
		return nil, fmt.Errorf("image workspace create failed: %w", err)
	}

	info := &workspace.Info{
		ID:           id,
		GoldenCopy:   goldenRoot,
		GoldenCommit: opts.GoldenCommit,
		CreatedAt:    time.Now().UTC(),
		Branch:       opts.Branch,
		Path:         wsPath,
	}
	if err := workspace.WriteMarker(wsPath, info); err != nil {
		_ = image.DestroyWorkspace(goldenRoot, id, nil)
		return nil, fmt.Errorf("writing workspace marker: %w", err)
	}

	return info, nil
}

func (imageBackend) DestroyWorkspace(goldenRoot string, cfg *config.Config, id string) error {
	return destroyWorkspace(goldenRoot, cfg, id)
}

func (imageBackend) RefreshBase(goldenRoot, commit string, excludes []string, onProgress func(int, string)) error {
	if _, err := image.RefreshBase(goldenRoot, goldenRoot, nil, commit, excludes, onProgress); err != nil {
		return fmt.Errorf("image backend refresh failed: %w", err)
	}
	return nil
}
