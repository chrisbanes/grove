package main

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestProgressState_NonDecreasing(t *testing.T) {
	s := newProgressState(5, 95)
	s.updateClone(10, 100)
	first := s.percent
	s.updateClone(5, 100)
	if s.percent < first {
		t.Fatalf("percent regressed: first=%d second=%d", first, s.percent)
	}
}

func TestProgressState_ClampBounds(t *testing.T) {
	s := newProgressState(5, 95)
	s.updateClone(200, 100)
	if s.percent < 0 || s.percent > 100 {
		t.Fatalf("percent out of bounds: %d", s.percent)
	}
}

func TestProgressRenderer_NonTTYLineMode(t *testing.T) {
	var buf bytes.Buffer
	r := newProgressRenderer(&buf, false, "create")
	r.Update(35, "clone")
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("[35%]")) {
		t.Fatalf("expected non-tty line output to include percent, got: %q", out)
	}
	if !bytes.Contains([]byte(out), []byte("clone")) {
		t.Fatalf("expected non-tty output to include phase, got: %q", out)
	}
}

func TestProgressRenderer_TTYUsesBar(t *testing.T) {
	var buf bytes.Buffer
	r := newProgressRenderer(&buf, true, "create")
	if r.bar == nil {
		t.Fatal("expected progressbar for tty output")
	}
	r.Update(50, "clone")
	r.Done()
}

func TestProgressRenderer_NonTTYNoBar(t *testing.T) {
	var buf bytes.Buffer
	r := newProgressRenderer(&buf, false, "create")
	if r.bar != nil {
		t.Fatal("expected no progressbar for non-tty output")
	}
}

func TestProgressRenderer_DoneIsIdempotent(t *testing.T) {
	var buf bytes.Buffer
	r := newProgressRenderer(&buf, true, "create")
	r.Update(100, "done")
	r.Done()
	r.Done() // should not panic
}

func TestResolveProgress_FallsBackToTTYDetection(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("progress", false, "")
	// Flag not changed — should fall back to isTerminalFile(os.Stderr).
	// In test, stderr is typically not a TTY, so expect false.
	got := resolveProgress(cmd)
	if got {
		t.Fatal("expected false when stderr is not a TTY and flag not set")
	}
}

func TestResolveProgress_RespectsExplicitTrue(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("progress", false, "")
	_ = cmd.Flags().Set("progress", "true")
	got := resolveProgress(cmd)
	if !got {
		t.Fatal("expected true when --progress explicitly set")
	}
}

func TestResolveProgress_RespectsExplicitFalse(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("progress", false, "")
	_ = cmd.Flags().Set("progress", "false")
	got := resolveProgress(cmd)
	if got {
		t.Fatal("expected false when --progress=false explicitly set")
	}
}
