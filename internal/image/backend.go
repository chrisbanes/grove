package image

import (
	"fmt"
	"os"
	"path/filepath"
)

const defaultBaseSizeGB = 200

func InitBase(runtimeRoot, goldenRoot string, runner Runner, baseSizeGB int, excludes []string, onProgress func(int, string)) (_ *State, err error) {
	if baseSizeGB <= 0 {
		baseSizeGB = defaultBaseSizeGB
	}

	if err := os.MkdirAll(imagesDir(runtimeRoot), 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(baseMountpoint(runtimeRoot), 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(initMarkerPath(runtimeRoot), []byte("initializing\n"), 0644); err != nil {
		return nil, err
	}
	defer func() {
		removeErr := os.Remove(initMarkerPath(runtimeRoot))
		if err == nil && removeErr != nil && !os.IsNotExist(removeErr) {
			err = removeErr
		}
	}()

	basePath := baseImagePath(runtimeRoot)
	// Remove any stale sparsebundle left from a previously interrupted init.
	if err := os.RemoveAll(basePath); err != nil {
		return nil, err
	}
	if onProgress != nil {
		onProgress(0, "creating base image")
	}
	if err := CreateSparseBundle(runner, basePath, "grove-base", baseSizeGB); err != nil {
		return nil, err
	}

	vol, err := Attach(runner, basePath, baseMountpoint(runtimeRoot))
	if err != nil {
		return nil, err
	}
	defer func() {
		detachErr := Detach(runner, vol.Device)
		if err == nil && detachErr != nil {
			err = detachErr
		}
	}()

	if onProgress != nil {
		onProgress(5, "syncing golden copy")
		err = SyncBaseWithProgress(runner, goldenRoot, vol.MountPoint, excludes, func(pct int) {
			onProgress(mapPercent(pct, 100, 5, 95), "syncing golden copy")
		})
	} else {
		err = SyncBase(runner, goldenRoot, vol.MountPoint, excludes)
	}
	if err != nil {
		return nil, err
	}

	st := &State{
		Backend:        "image",
		BasePath:       basePath,
		BaseGeneration: 1,
	}
	if err := SaveState(runtimeRoot, st); err != nil {
		return nil, err
	}
	if onProgress != nil {
		onProgress(100, "done")
	}
	return st, nil
}

func RefreshBase(runtimeRoot, goldenRoot string, runner Runner, commit string, excludes []string, onProgress func(int, string)) (_ *State, err error) {
	metas, err := ListWorkspaceMeta(runtimeRoot)
	if err != nil {
		return nil, err
	}
	if len(metas) > 0 {
		return nil, fmt.Errorf("cannot refresh base with active image workspaces (%d)", len(metas))
	}

	st, err := LoadState(runtimeRoot)
	if err != nil {
		return nil, err
	}
	if st.BasePath == "" {
		st.BasePath = baseImagePath(runtimeRoot)
	}
	if err := os.MkdirAll(baseMountpoint(runtimeRoot), 0755); err != nil {
		return nil, err
	}

	vol, err := Attach(runner, st.BasePath, baseMountpoint(runtimeRoot))
	if err != nil {
		return nil, err
	}
	defer func() {
		detachErr := Detach(runner, vol.Device)
		if err == nil && detachErr != nil {
			err = detachErr
		}
	}()

	if onProgress != nil {
		onProgress(5, "syncing golden copy")
		err = SyncBaseWithProgress(runner, goldenRoot, vol.MountPoint, excludes, func(pct int) {
			onProgress(mapPercent(pct, 100, 5, 95), "syncing golden copy")
		})
	} else {
		err = SyncBase(runner, goldenRoot, vol.MountPoint, excludes)
	}
	if err != nil {
		return nil, err
	}

	st.Backend = "image"
	st.BaseGeneration++
	st.LastSyncCommit = commit

	if err := SaveState(runtimeRoot, st); err != nil {
		return nil, err
	}
	if onProgress != nil {
		onProgress(100, "done")
	}
	return st, nil
}

func baseMountpoint(runtimeRoot string) string {
	return filepath.Join(runtimeRoot, "mnt", "base")
}

func mapPercent(value, total, min, max int) int {
	if total <= 0 {
		return min
	}
	if value > total {
		value = total
	}
	return min + (value*(max-min))/total
}
