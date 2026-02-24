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

func LoadState(runtimeRoot string) (*State, error) {
	if detectErr := detectInitMarker(runtimeRoot); detectErr != nil {
		return nil, detectErr
	}

	data, err := os.ReadFile(stateFilePath(runtimeRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if detectErr := detectBaseWithoutState(runtimeRoot); detectErr != nil {
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

func SaveState(runtimeRoot string, st *State) error {
	if err := os.MkdirAll(imagesDir(runtimeRoot), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(stateFilePath(runtimeRoot), data, 0644)
}

func SaveWorkspaceMeta(runtimeRoot string, meta *WorkspaceMeta) error {
	if err := os.MkdirAll(workspacesDir(runtimeRoot), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(workspaceMetaPath(runtimeRoot, meta.ID), data, 0644)
}

func LoadWorkspaceMeta(runtimeRoot, id string) (*WorkspaceMeta, error) {
	data, err := os.ReadFile(workspaceMetaPath(runtimeRoot, id))
	if err != nil {
		return nil, err
	}
	var meta WorkspaceMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func ListWorkspaceMeta(runtimeRoot string) ([]WorkspaceMeta, error) {
	entries, err := os.ReadDir(workspacesDir(runtimeRoot))
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
		data, err := os.ReadFile(filepath.Join(workspacesDir(runtimeRoot), entry.Name()))
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

func DeleteWorkspaceMeta(runtimeRoot, id string) error {
	err := os.Remove(workspaceMetaPath(runtimeRoot, id))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func imagesDir(runtimeRoot string) string {
	return filepath.Join(runtimeRoot, "images")
}

func stateFilePath(runtimeRoot string) string {
	return filepath.Join(imagesDir(runtimeRoot), "state.json")
}

func baseImagePath(runtimeRoot string) string {
	return filepath.Join(imagesDir(runtimeRoot), "base.sparsebundle")
}

func initMarkerPath(runtimeRoot string) string {
	return filepath.Join(imagesDir(runtimeRoot), "init-in-progress")
}

func workspacesDir(runtimeRoot string) string {
	return filepath.Join(runtimeRoot, "workspaces")
}

func workspaceMetaPath(runtimeRoot, id string) string {
	return filepath.Join(workspacesDir(runtimeRoot), id+".json")
}

func detectInitMarker(runtimeRoot string) error {
	if _, err := os.Stat(initMarkerPath(runtimeRoot)); err == nil {
		return fmt.Errorf("%w: found %s. Previous initialization may have been cancelled; remove stale image data and rerun `grove migrate --to image`", ErrInitIncomplete, initMarkerPath(runtimeRoot))
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func detectBaseWithoutState(runtimeRoot string) error {
	if _, err := os.Stat(baseImagePath(runtimeRoot)); err == nil {
		return fmt.Errorf("%w: found %s without %s; remove stale image data and rerun `grove migrate --to image`", ErrInitIncomplete, baseImagePath(runtimeRoot), stateFilePath(runtimeRoot))
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}
