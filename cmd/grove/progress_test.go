package main

import (
	"bytes"
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
