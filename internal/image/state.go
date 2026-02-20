package image

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// State stores image backend metadata for a golden copy.
type State struct {
	Backend        string `json:"backend"`
	BasePath       string `json:"base_path"`
	BaseGeneration int    `json:"base_generation"`
	LastSyncCommit string `json:"last_sync_commit,omitempty"`
}

// WorkspaceMeta stores image metadata for a workspace.
type WorkspaceMeta struct {
	ID             string    `json:"id"`
	Mountpoint     string    `json:"mountpoint"`
	Device         string    `json:"device"`
	ShadowPath     string    `json:"shadow_path"`
	BaseGeneration int       `json:"base_generation"`
	CreatedAt      time.Time `json:"created_at"`
}

func LoadState(repoRoot string) (*State, error) {
	data, err := os.ReadFile(stateFilePath(repoRoot))
	if err != nil {
		return nil, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func SaveState(repoRoot string, st *State) error {
	if err := os.MkdirAll(imagesDir(repoRoot), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(stateFilePath(repoRoot), data, 0644)
}

func SaveWorkspaceMeta(repoRoot string, meta *WorkspaceMeta) error {
	if err := os.MkdirAll(workspacesDir(repoRoot), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(workspaceMetaPath(repoRoot, meta.ID), data, 0644)
}

func LoadWorkspaceMeta(repoRoot, id string) (*WorkspaceMeta, error) {
	data, err := os.ReadFile(workspaceMetaPath(repoRoot, id))
	if err != nil {
		return nil, err
	}
	var meta WorkspaceMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func ListWorkspaceMeta(repoRoot string) ([]WorkspaceMeta, error) {
	entries, err := os.ReadDir(workspacesDir(repoRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]WorkspaceMeta, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(workspacesDir(repoRoot), entry.Name()))
		if err != nil {
			return nil, err
		}
		var meta WorkspaceMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			return nil, err
		}
		out = append(out, meta)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func DeleteWorkspaceMeta(repoRoot, id string) error {
	err := os.Remove(workspaceMetaPath(repoRoot, id))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func imagesDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".grove", "images")
}

func stateFilePath(repoRoot string) string {
	return filepath.Join(imagesDir(repoRoot), "state.json")
}

func workspacesDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".grove", "workspaces")
}

func workspaceMetaPath(repoRoot, id string) string {
	return filepath.Join(workspacesDir(repoRoot), id+".json")
}

