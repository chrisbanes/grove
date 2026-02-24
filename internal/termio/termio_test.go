package termio

import (
	"os"
	"os/exec"
	"testing"

	"golang.org/x/term"
)

type fakeTermOps struct {
	restoreErr error
	getCalls   []int
	restores   []int
}

func (f *fakeTermOps) isTerminal(fd int) bool {
	return true
}

func (f *fakeTermOps) getState(fd int) (*term.State, error) {
	f.getCalls = append(f.getCalls, fd)
	return &term.State{}, nil
}

func (f *fakeTermOps) restore(fd int, _ *term.State) error {
	f.restores = append(f.restores, fd)
	return f.restoreErr
}

func TestRunWithTTYRestore_RestoresOnSuccess(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 0")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	ops := &fakeTermOps{}
	if err := runWithTTYRestore(cmd, ops); err != nil {
		t.Fatalf("runWithTTYRestore returned error: %v", err)
	}

	if len(ops.getCalls) != 3 {
		t.Fatalf("expected 3 getState calls, got %d", len(ops.getCalls))
	}
	if len(ops.restores) != 3 {
		t.Fatalf("expected 3 restore calls, got %d", len(ops.restores))
	}
}

func TestRunWithTTYRestore_RestoresOnCommandError(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 7")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	ops := &fakeTermOps{}
	if err := runWithTTYRestore(cmd, ops); err == nil {
		t.Fatal("expected command error")
	}
	if len(ops.restores) != 3 {
		t.Fatalf("expected 3 restore calls, got %d", len(ops.restores))
	}
}

func TestRunWithTTYRestore_ReturnsRestoreError(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 0")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	ops := &fakeTermOps{restoreErr: os.ErrPermission}
	if err := runWithTTYRestore(cmd, ops); err == nil {
		t.Fatal("expected restore error")
	}
}
