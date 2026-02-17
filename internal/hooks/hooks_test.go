package hooks_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chrisbanes/grove/internal/hooks"
)

func TestRun_HookExists(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".grove", "hooks")
	os.MkdirAll(hooksDir, 0755)

	// Create a hook that writes a marker file
	hookPath := filepath.Join(hooksDir, "post-clone")
	os.WriteFile(hookPath, []byte("#!/bin/bash\ntouch \"$PWD/hook-ran\"\n"), 0755)

	err := hooks.Run(dir, "post-clone")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "hook-ran")); err != nil {
		t.Error("hook did not execute")
	}
}

func TestRun_HookMissing(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".grove", "hooks"), 0755)

	// Should succeed silently — hooks are optional
	err := hooks.Run(dir, "post-clone")
	if err != nil {
		t.Errorf("expected no error for missing hook, got: %v", err)
	}
}

func TestRun_HookFails(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".grove", "hooks")
	os.MkdirAll(hooksDir, 0755)

	hookPath := filepath.Join(hooksDir, "post-clone")
	os.WriteFile(hookPath, []byte("#!/bin/bash\nexit 1\n"), 0755)

	err := hooks.Run(dir, "post-clone")
	if err == nil {
		t.Error("expected error for failing hook")
	}
}

func TestRun_HookNotExecutable(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".grove", "hooks")
	os.MkdirAll(hooksDir, 0755)

	hookPath := filepath.Join(hooksDir, "post-clone")
	os.WriteFile(hookPath, []byte("#!/bin/bash\necho ok\n"), 0644) // not executable

	err := hooks.Run(dir, "post-clone")
	if err == nil {
		t.Error("expected error for non-executable hook")
	}
}
