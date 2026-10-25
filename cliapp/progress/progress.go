// Package progress renders CLI download progress with a simple, stable line.
package progress

import (
	"fmt"
	"io"
	"strings"

	bubblesprogress "github.com/charmbracelet/bubbles/progress"
)

// State is one progress sample.
type State struct {
	Index        int
	Count        int
	Name         string
	Done         int64
	Total        int64
	OverallDone  int64
	OverallTotal int64
	Speed        int64
}

// Reporter receives progress samples.
type Reporter interface {
	Update(State)
	Finish()
}

// NewAuto returns a live reporter when w is a terminal, otherwise a quiet one.
func NewAuto(w io.Writer, tty bool) Reporter {
	if !tty {
		return Quiet{}
	}
	return &Line{Out: w, Bar: newBar()}
}

// Quiet discards all progress.
type Quiet struct{}

// Update implements Reporter.
func (Quiet) Update(State) {}

// Finish implements Reporter.
func (Quiet) Finish() {}

// Line writes a single overwritten line using bubbles/progress.
type Line struct {
	Out io.Writer
	Bar bubblesprogress.Model
}

func newBar() bubblesprogress.Model {
	m := bubblesprogress.New(bubblesprogress.WithWidth(24), bubblesprogress.WithDefaultGradient())
	return m
}

// Update implements Reporter.
func (l *Line) Update(s State) {
	pct := ratio(s.OverallDone, s.OverallTotal)
	bar := l.Bar.ViewAs(pct)
	if s.OverallTotal <= 0 {
		bar = strings.Repeat("░", 24)
	}
	parts := []string{bar, fmt.Sprintf("%3.0f%%", pct*100)}
	if s.Count > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d", s.Index, s.Count))
	}
	if s.Name != "" {
		parts = append(parts, truncate(s.Name, 28))
	}
	if s.Speed > 0 {
		parts = append(parts, humanSpeed(s.Speed))
	}
	_, _ = fmt.Fprintf(l.Out, "\r\x1b[K%s", strings.Join(parts, "  "))
}

// Finish implements Reporter by clearing the live line.
func (l *Line) Finish() {
	_, _ = fmt.Fprint(l.Out, "\r\x1b[K")
}

func ratio(done, total int64) float64 {
	if total <= 0 {
		return 0
	}
	r := float64(done) / float64(total)
	if r < 0 {
		return 0
	}
	if r > 1 {
		return 1
	}
	return r
}

func truncate(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w <= 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

func humanSpeed(bytesPerSecond int64) string {
	if bytesPerSecond <= 0 {
		return "0.00 MiB/s"
	}
	return fmt.Sprintf("%.2f MiB/s", float64(bytesPerSecond)/(1024*1024))
}
