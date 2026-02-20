package image

import (
	"fmt"
	"os"
	"path/filepath"
)

func InitBase(repoRoot string, runner Runner, baseSizeGB int) (_ *State, err error) {
	if baseSizeGB <= 0 {
		baseSizeGB = 20
	}

	basePath := filepath.Join(imagesDir(repoRoot), "base.sparsebundle")
	if err := os.MkdirAll(imagesDir(repoRoot), 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(baseMountpoint(repoRoot), 0755); err != nil {
		return nil, err
	}
	if err := CreateSparseBundle(runner, basePath, "grove-base", baseSizeGB); err != nil {
		return nil, err
	}

	vol, err := Attach(runner, basePath, baseMountpoint(repoRoot))
	if err != nil {
		return nil, err
	}
	defer func() {
		detachErr := Detach(runner, vol.Device)
		if err == nil && detachErr != nil {
			err = detachErr
		}
	}()

	if err := SyncBase(runner, repoRoot, vol.MountPoint); err != nil {
		return nil, err
	}

	st := &State{
		Backend:        "image",
		BasePath:       basePath,
		BaseGeneration: 1,
	}
	if err := SaveState(repoRoot, st); err != nil {
		return nil, err
	}
	return st, nil
}

func RefreshBase(repoRoot, goldenRoot string, runner Runner, commit string) (_ *State, err error) {
	metas, err := ListWorkspaceMeta(repoRoot)
	if err != nil {
		return nil, err
	}
	if len(metas) > 0 {
		return nil, fmt.Errorf("cannot refresh base with active image workspaces (%d)", len(metas))
	}

	st, err := LoadState(repoRoot)
	if err != nil {
		return nil, err
	}
	if st.BasePath == "" {
		st.BasePath = filepath.Join(imagesDir(repoRoot), "base.sparsebundle")
	}
	if err := os.MkdirAll(baseMountpoint(repoRoot), 0755); err != nil {
		return nil, err
	}

	vol, err := Attach(runner, st.BasePath, baseMountpoint(repoRoot))
	if err != nil {
		return nil, err
	}
	defer func() {
		detachErr := Detach(runner, vol.Device)
		if err == nil && detachErr != nil {
			err = detachErr
		}
	}()

	if err := SyncBase(runner, goldenRoot, vol.MountPoint); err != nil {
		return nil, err
	}

	st.Backend = "image"
	st.BaseGeneration++
	st.LastSyncCommit = commit

	if err := SaveState(repoRoot, st); err != nil {
		return nil, err
	}
	return st, nil
}

func baseMountpoint(repoRoot string) string {
	return filepath.Join(repoRoot, ".grove", "mnt", "base")
}

