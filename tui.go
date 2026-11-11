package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	colorReset  = "\033[0m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorDim    = "\033[2m"
)

func init() {
	// check if no-color requested or dumb terminal
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		colorReset = ""
		colorCyan = ""
		colorGreen = ""
		colorYellow = ""
		colorRed = ""
		colorDim = ""
	}
}

// spinner manages a terminal spinner animation
type spinner struct {
	text   string
	frames []string
	stop   chan struct{}
	done   chan struct{}
	mu     sync.Mutex
}

func newSpinner(text string) *spinner {
	return &spinner{
		text:   text,
		frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
}

func (s *spinner) start() {
	go func() {
		defer close(s.done)
		idx := 0
		for {
			select {
			case <-s.stop:
				return
			default:
				s.mu.Lock()
				f := s.frames[idx%len(s.frames)]
				msg := s.text
				s.mu.Unlock()

				fmt.Printf("\r%s%s%s scpkg: %s", colorCyan, f, colorReset, msg)
				idx++
				time.Sleep(80 * time.Millisecond)
			}
		}
	}()
}

func (s *spinner) update(text string) {
	s.mu.Lock()
	s.text = text
	s.mu.Unlock()
}

func (s *spinner) stopWithSuccess(msg string) {
	close(s.stop)
	<-s.done
	fmt.Printf("\r%s✓%s scpkg: %s\n", colorGreen, colorReset, msg)
}

func (s *spinner) stopWithFail(msg string) {
	close(s.stop)
	<-s.done
	fmt.Printf("\r%s✗%s scpkg: %s\n", colorRed, colorReset, msg)
}

// progressBar tracks download progress on a single line
type progressBar struct {
	total     int64
	current   int64
	label     string
	width     int
	lastPrint time.Time
}

func newProgressBar(total int64, label string) *progressBar {
	return &progressBar{
		total: total,
		label: label,
		width: 24,
	}
}

func (p *progressBar) add(n int) {
	p.current += int64(n)
	now := time.Now()
	if now.Sub(p.lastPrint) < 50*time.Millisecond && p.current < p.total {
		return
	}
	p.lastPrint = now

	percent := float64(0)
	if p.total > 0 {
		percent = float64(p.current) / float64(p.total) * 100
	}

	filled := 0
	if p.total > 0 {
		filled = int((float64(p.current) / float64(p.total)) * float64(p.width))
	}
	if filled > p.width {
		filled = p.width
	}

	bar := strings.Repeat("━", filled) + strings.Repeat("─", p.width-filled)
	curKB := float64(p.current) / 1024.0
	totKB := float64(p.total) / 1024.0

	fmt.Printf("\r%s▸%s scpkg: %s [%s%s%s] %.1f%% (%.1f / %.1f kb)",
		colorCyan, colorReset, p.label, colorCyan, bar, colorReset, percent, curKB, totKB)
}

func (p *progressBar) finish(msg string) {
	fmt.Printf("\r%s✓%s scpkg: %s\n", colorGreen, colorReset, msg)
}

func logInfo(format string, a ...any) {
	fmt.Printf("%s▸%s scpkg: %s\n", colorCyan, colorReset, fmt.Sprintf(format, a...))
}

func logSuccess(format string, a ...any) {
	fmt.Printf("%s✓%s scpkg: %s\n", colorGreen, colorReset, fmt.Sprintf(format, a...))
}

func logWarning(format string, a ...any) {
	fmt.Printf("%s!%s scpkg: %s\n", colorYellow, colorReset, fmt.Sprintf(format, a...))
}

func logError(format string, a ...any) {
	fmt.Printf("%s✗%s scpkg: %s\n", colorRed, colorReset, fmt.Sprintf(format, a...))
}
