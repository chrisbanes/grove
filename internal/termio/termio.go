package termio

import (
	"fmt"
	"os"
	"os/exec"

	"golang.org/x/term"
)

type terminalState struct {
	fd    int
	state *term.State
}

type termOps interface {
	isTerminal(fd int) bool
	getState(fd int) (*term.State, error)
	restore(fd int, state *term.State) error
}

type realTermOps struct{}

func (realTermOps) isTerminal(fd int) bool {
	return term.IsTerminal(fd)
}

func (realTermOps) getState(fd int) (*term.State, error) {
	return term.GetState(fd)
}

func (realTermOps) restore(fd int, state *term.State) error {
	return term.Restore(fd, state)
}

// RunInteractive runs a command attached to the current process stdio and
// restores terminal settings afterward for any attached TTY file descriptors.
func RunInteractive(cmd *exec.Cmd) error {
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	return runWithTTYRestore(cmd, realTermOps{})
}

func runWithTTYRestore(cmd *exec.Cmd, ops termOps) error {
	states := captureStates(ops, cmd.Stdin, cmd.Stdout, cmd.Stderr)
	runErr := cmd.Run()
	restoreErr := restoreStates(ops, states)

	if runErr != nil {
		if restoreErr != nil {
			return fmt.Errorf("%w (and failed to restore terminal state: %v)", runErr, restoreErr)
		}
		return runErr
	}
	if restoreErr != nil {
		return fmt.Errorf("restore terminal state: %w", restoreErr)
	}
	return nil
}

func captureStates(ops termOps, streams ...any) []terminalState {
	states := make([]terminalState, 0, len(streams))
	seen := make(map[int]struct{}, len(streams))
	for _, stream := range streams {
		file, ok := stream.(*os.File)
		if !ok || file == nil {
			continue
		}
		fd := int(file.Fd())
		if _, exists := seen[fd]; exists {
			continue
		}
		seen[fd] = struct{}{}
		if !ops.isTerminal(fd) {
			continue
		}
		state, err := ops.getState(fd)
		if err != nil {
			continue
		}
		states = append(states, terminalState{fd: fd, state: state})
	}
	return states
}

func restoreStates(ops termOps, states []terminalState) error {
	for i := len(states) - 1; i >= 0; i-- {
		state := states[i]
		if err := ops.restore(state.fd, state.state); err != nil {
			return err
		}
	}
	return nil
}
