package image

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrInitIncomplete = errors.New("image backend initialization appears incomplete")

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
	if detectErr := detectInitMarker(repoRoot); detectErr != nil {
		return nil, detectErr
	}

	data, err := os.ReadFile(stateFilePath(repoRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if detectErr := detectBaseWithoutState(repoRoot); detectErr != nil {
				return nil, detectErr
			}
		}
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

func baseImagePath(repoRoot string) string {
	return filepath.Join(imagesDir(repoRoot), "base.sparsebundle")
}

func initMarkerPath(repoRoot string) string {
	return filepath.Join(imagesDir(repoRoot), "init-in-progress")
}

func workspacesDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".grove", "workspaces")
}

func workspaceMetaPath(repoRoot, id string) string {
	return filepath.Join(workspacesDir(repoRoot), id+".json")
}

func detectInitMarker(repoRoot string) error {
	if _, err := os.Stat(initMarkerPath(repoRoot)); err == nil {
		return fmt.Errorf("%w: found %s. Previous initialization may have been cancelled; remove stale image data and rerun `grove migrate --to image`", ErrInitIncomplete, initMarkerPath(repoRoot))
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func detectBaseWithoutState(repoRoot string) error {
	if _, err := os.Stat(baseImagePath(repoRoot)); err == nil {
		return fmt.Errorf("%w: found %s without %s; remove stale image data and rerun `grove migrate --to image`", ErrInitIncomplete, baseImagePath(repoRoot), stateFilePath(repoRoot))
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}
