package image

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func CreateWorkspace(repoRoot, goldenRoot, workspacePath, workspaceID string, st *State, runner Runner) (*WorkspaceMeta, error) {
	_ = goldenRoot
	if st == nil {
		loaded, err := LoadState(repoRoot)
		if err != nil {
			return nil, err
		}
		st = loaded
	}
	if st.BasePath == "" {
		return nil, fmt.Errorf("image backend state missing base_path")
	}
	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		return nil, err
	}

	shadowPath := filepath.Join(repoRoot, ".grove", "shadows", workspaceID+".shadow")
	if err := os.MkdirAll(filepath.Dir(shadowPath), 0755); err != nil {
		return nil, err
	}

	vol, err := AttachWithShadow(runner, st.BasePath, shadowPath, workspacePath)
	if err != nil {
		return nil, err
	}

	meta := &WorkspaceMeta{
		ID:             workspaceID,
		Mountpoint:     workspacePath,
		Device:         vol.Device,
		ShadowPath:     shadowPath,
		BaseGeneration: st.BaseGeneration,
		CreatedAt:      time.Now().UTC(),
	}
	if err := SaveWorkspaceMeta(repoRoot, meta); err != nil {
		_ = Detach(runner, vol.Device)
		_ = os.Remove(shadowPath)
		return nil, err
	}
	return meta, nil
}

func DestroyWorkspace(repoRoot, workspaceID string, runner Runner) error {
	meta, err := LoadWorkspaceMeta(repoRoot, workspaceID)
	if err != nil {
		return err
	}
	if err := Detach(runner, meta.Device); err != nil {
		return err
	}
	if err := os.Remove(meta.ShadowPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := DeleteWorkspaceMeta(repoRoot, workspaceID); err != nil {
		return err
	}
	if err := os.RemoveAll(meta.Mountpoint); err != nil {
		return err
	}
	return nil
}

