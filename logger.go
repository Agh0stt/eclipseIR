package main

import (
	"fmt"
	"os"
	"os/exec"
)

// ─────────────────────────────────────────────
//  logger.go — EclipseIR CLI Logger
// ─────────────────────────────────────────────

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
	colorBold   = "\033[1m"
)

type Logger struct {
	cfg      *Config
	useColor bool
}

func NewLogger(cfg *Config) *Logger {
	return &Logger{cfg: cfg, useColor: !cfg.ColorOff}
}

func (l *Logger) color(c, s string) string {
	if !l.useColor {
		return s
	}
	return c + s + colorReset
}

func (l *Logger) Step(tag, detail string) {
	if !l.cfg.Verbose {
		return
	}
	label := l.color(colorCyan, fmt.Sprintf("[%-10s]", tag))
	fmt.Fprintf(os.Stderr, "%s %s\n", label, detail)
}

func (l *Logger) Ok(msg string) {
	tick := l.color(colorGreen, "✓")
	fmt.Fprintf(os.Stderr, "%s %s\n", tick, msg)
}

func (l *Logger) Error(msg string) {
	label := l.color(colorRed+colorBold, "[error]")
	fmt.Fprintf(os.Stderr, "%s %s\n", label, msg)
}

func (l *Logger) Warn(msg string) {
	label := l.color(colorYellow, "[warn] ")
	fmt.Fprintf(os.Stderr, "%s %s\n", label, msg)
}

func (l *Logger) Info(msg string) {
	label := l.color(colorGray, "[info] ")
	fmt.Fprintf(os.Stderr, "%s %s\n", label, msg)
}

// ── Shell runner ──────────────────────────────

func runCmd(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
