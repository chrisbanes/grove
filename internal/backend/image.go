package backend

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chrisbanes/grove/internal/config"
	"github.com/chrisbanes/grove/internal/image"
	"github.com/chrisbanes/grove/internal/workspace"
)

type imageBackend struct{}

const createInitBaseSizeGB = 200

var (
	imageLoadState = image.LoadState
	imageInitBase  = image.InitBase
)

func (imageBackend) Name() string {
	return "image"
}

func (imageBackend) CreateWorkspace(goldenRoot string, cfg *config.Config, opts CreateOptions) (*workspace.Info, error) {
	runtimeRoot, err := config.EnsureImageRuntimeRoot(goldenRoot, cfg)
	if err != nil {
		return nil, fmt.Errorf("resolving image runtime root: %w", err)
	}
	excludes, err := config.BuildImageSyncExcludes(goldenRoot, cfg)
	if err != nil {
		return nil, fmt.Errorf("computing image sync excludes: %w", err)
	}

	st, _, err := loadOrInitImageState(runtimeRoot, goldenRoot, excludes, nil)
	if err != nil {
		return nil, err
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

	if _, err := image.CreateWorkspace(runtimeRoot, goldenRoot, wsPath, id, st, nil); err != nil {
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
		_ = image.DestroyWorkspace(runtimeRoot, id, nil)
		return nil, fmt.Errorf("writing workspace marker: %w", err)
	}

	return info, nil
}

func (imageBackend) DestroyWorkspace(goldenRoot string, cfg *config.Config, id string) error {
	return destroyWorkspace(goldenRoot, cfg, id)
}

func (imageBackend) RefreshBase(goldenRoot, commit string, excludes []string, onProgress func(int, string)) error {
	cfg, err := config.Load(goldenRoot)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	cfg.StateDir = config.ExpandStateDir(cfg.StateDir)
	runtimeRoot, err := config.EnsureImageRuntimeRoot(goldenRoot, cfg)
	if err != nil {
		return fmt.Errorf("resolving image runtime root: %w", err)
	}

	st, initialized, err := loadOrInitImageState(runtimeRoot, goldenRoot, excludes, onProgress)
	if err != nil {
		return err
	}
	if initialized {
		st.LastSyncCommit = commit
		if err := image.SaveState(runtimeRoot, st); err != nil {
			return fmt.Errorf("saving image backend state: %w", err)
		}
		return nil
	}

	if _, err := image.RefreshBase(runtimeRoot, goldenRoot, nil, commit, excludes, onProgress); err != nil {
		return fmt.Errorf("image backend refresh failed: %w", err)
	}
	return nil
}

func loadOrInitImageState(runtimeRoot, goldenRoot string, excludes []string, onProgress func(int, string)) (*image.State, bool, error) {
	st, err := imageLoadState(runtimeRoot)
	if err == nil {
		return st, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, false, fmt.Errorf("loading image backend state: %w", err)
	}

	st, err = imageInitBase(runtimeRoot, goldenRoot, nil, createInitBaseSizeGB, excludes, onProgress)
	if err != nil {
		return nil, false, fmt.Errorf("initializing image backend: %w", err)
	}
	return st, true, nil
}
