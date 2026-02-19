package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mandelsoft/ttyprogress"
)

type progressState struct {
	min     int
	max     int
	percent int
}

func newProgressState(min, max int) *progressState {
	return &progressState{min: min, max: max, percent: clampPercent(min)}
}

func (s *progressState) updateClone(copied, total int) {
	next := mapPercent(copied, total, s.min, s.max)
	if next < s.percent {
		return
	}
	s.percent = clampPercent(next)
}

type progressRenderer struct {
	w            io.Writer
	tty          bool
	lastPercent  int
	lastPhase    string
	ttyContext   ttyprogress.Context
	ttyBar       ttyprogress.Bar
	lastWidth    int
	wroteTTYLine bool
}

const progressPhaseVariable = "phase"

func newProgressRenderer(w io.Writer, tty bool) *progressRenderer {
	r := &progressRenderer{
		w:           w,
		tty:         tty,
		lastPercent: -1,
	}
	if tty {
		if _, ok := w.(*os.File); ok {
			ctx, bar, err := newFancyProgress(w)
			if err == nil {
				r.ttyContext = ctx
				r.ttyBar = bar
			}
		}
	}
	return r
}

func newFancyProgress(w io.Writer) (ttyprogress.Context, ttyprogress.Bar, error) {
	ctx := ttyprogress.For(w)
	bar, err := ttyprogress.NewBar().
		SetPredefined(10).
		SetTotal(100).
		SetWidth(ttyprogress.ReserveTerminalSize(45)).
		PrependElapsed().
		PrependMessage("create").
		AppendCompleted().
		AppendMessage("phase:").
		AppendVariable(progressPhaseVariable).
		Add(ctx)
	if err != nil {
		_ = ctx.Close()
		_ = ctx.Wait(context.Background())
		return nil, nil, err
	}
	return ctx, bar, nil
}

func (r *progressRenderer) Update(percent int, phase string) {
	percent = clampPercent(percent)
	if r.ttyBar != nil {
		if percent == r.lastPercent && phase == r.lastPhase {
			return
		}
		phaseChanged := phase != r.lastPhase
		r.lastPercent = percent
		r.lastPhase = phase
		r.ttyBar.SetVariable(progressPhaseVariable, phase)
		r.ttyBar.Set(percent)
		if phaseChanged {
			_ = r.ttyBar.Flush()
		}
		return
	}

	if r.tty {
		if percent == r.lastPercent && phase == r.lastPhase {
			return
		}
		r.lastPercent = percent
		r.lastPhase = phase
		r.wroteTTYLine = true
		line := fmt.Sprintf("\r[%s] %3d%% %s", renderBar(percent, 24), percent, phase)
		width := len(line) - 1 // exclude leading carriage return
		if r.lastWidth > width {
			line += strings.Repeat(" ", r.lastWidth-width)
		}
		r.lastWidth = width
		fmt.Fprint(r.w, line)
		return
	}

	emit := false
	if phase != r.lastPhase {
		emit = true
	}
	if r.lastPercent < 0 || percent-r.lastPercent >= 5 {
		emit = true
	}
	if percent == 100 {
		emit = true
	}
	if !emit {
		return
	}
	r.lastPercent = percent
	r.lastPhase = phase
	fmt.Fprintf(r.w, "[%d%%] %s\n", percent, phase)
}

func (r *progressRenderer) Done() {
	if r.ttyContext != nil {
		if r.ttyBar != nil && !r.ttyBar.IsClosed() {
			_ = r.ttyBar.Close()
		}
		_ = r.ttyContext.Close()
		_ = r.ttyContext.Wait(context.Background())
		return
	}

	if r.tty && r.wroteTTYLine {
		fmt.Fprintln(r.w)
	}
}

func renderBar(percent, width int) string {
	if width <= 0 {
		return ""
	}
	filled := (clampPercent(percent) * width) / 100
	if filled > width {
		filled = width
	}
	return strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
}

func mapPercent(copied, total, min, max int) int {
	if max < min {
		min, max = max, min
	}
	if total <= 0 {
		return min
	}
	if copied < 0 {
		copied = 0
	}
	if copied > total {
		copied = total
	}
	span := max - min
	return min + (copied*span)/total
}

func clampPercent(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func isTerminalFile(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
