package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
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
	r := newProgressRenderer(&buf, false)
	r.Update(35, "clone")
	out := buf.String()
	if !strings.Contains(out, "[35%]") {
		t.Fatalf("expected non-tty line output to include percent, got: %q", out)
	}
	if !strings.Contains(out, "clone") {
		t.Fatalf("expected non-tty output to include phase, got: %q", out)
	}
}

func TestProgressRenderer_TTYClearsTrailingChars(t *testing.T) {
	var buf bytes.Buffer
	r := newProgressRenderer(&buf, true)
	r.Update(95, "post-clone hook")
	r.Update(100, "done")

	out := buf.String()
	lastCR := strings.LastIndex(out, "\r")
	if lastCR == -1 {
		t.Fatalf("expected tty output to include carriage return, got %q", out)
	}
	final := out[lastCR+1:]
	if !strings.Contains(final, "done") {
		t.Fatalf("expected final tty segment to include done phase, got %q", final)
	}
	if !strings.HasSuffix(final, " ") {
		t.Fatalf("expected final tty segment to include trailing spaces for line clearing, got %q", final)
	}
}

func TestProgressRenderer_TTYUsesFancyRendererForFiles(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "progress-*.txt")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer f.Close()

	r := newProgressRenderer(f, true)
	if r.ttyBar == nil {
		t.Fatal("expected ttyprogress bar for file-backed tty output")
	}
	r.Update(15, "clone")
	r.Done()
}

func TestProgressRenderer_TTYFallsBackForNonFileWriter(t *testing.T) {
	var buf bytes.Buffer
	r := newProgressRenderer(&buf, true)
	if r.ttyBar != nil {
		t.Fatal("expected fallback tty renderer for non-file writers")
	}
}
