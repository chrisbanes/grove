package main

import (
	"fmt"
	"io"
	"os"

	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
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
	w           io.Writer
	tty         bool
	lastPercent int
	lastPhase   string
	bar         *progressbar.ProgressBar
}

func newProgressRenderer(w io.Writer, tty bool, label string) *progressRenderer {
	r := &progressRenderer{
		w:           w,
		tty:         tty,
		lastPercent: -1,
	}
	if tty {
		r.bar = progressbar.NewOptions(100,
			progressbar.OptionSetWriter(w),
			progressbar.OptionSetDescription(label),
			progressbar.OptionSetWidth(30),
			progressbar.OptionSetElapsedTime(true),
			progressbar.OptionSetPredictTime(false),
			progressbar.OptionClearOnFinish(),
			progressbar.OptionSetRenderBlankState(true),
			progressbar.OptionUseANSICodes(true),
		)
	}
	return r
}

func (r *progressRenderer) Update(percent int, phase string) {
	percent = clampPercent(percent)
	if r.bar != nil {
		if percent == r.lastPercent && phase == r.lastPhase {
			return
		}
		r.lastPercent = percent
		r.lastPhase = phase
		r.bar.Describe(phase)
		_ = r.bar.Set(percent)
		return
	}

	// Non-TTY: emit line-based progress at meaningful intervals.
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
	if r.bar != nil {
		_ = r.bar.Finish()
		return
	}
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

// resolveProgress returns whether progress output should be enabled.
// If the user explicitly set --progress or --progress=false, that value wins.
// Otherwise, progress is enabled when stderr is a TTY.
func resolveProgress(cmd *cobra.Command) bool {
	if cmd.Flags().Changed("progress") {
		v, _ := cmd.Flags().GetBool("progress")
		return v
	}
	return isTerminalFile(os.Stderr)
}

func isTerminalFile(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
