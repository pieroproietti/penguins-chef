// Copyright 2026 Piero Proietti <piero.proietti@gmail.com>.
// All rights reserved.

package utils

import (
	"fmt"
	"os"
)

// ANSI colors and styles
const (
	ColorBold    = "\033[1m"
	ColorDim     = "\033[2m"
	ColorBlue    = "\033[1;34m"
	ColorCyan    = "\033[36m"
	ColorGreen   = "\033[1;32m"
	ColorRed     = "\033[1;31m"
	ColorReset   = "\033[0m"
	ColorYellow  = "\033[33m"
	ColorMagenta = "\033[35m"
	ColorWhite   = "\033[1;37m"
)

// DisableColors allows disabling color output.
var DisableColors bool

func init() {
	// Auto-detection: if os.Stdout is NOT a terminal (e.g., redirected to a log file or pipe),
	// automatically turn off colors to prevent dirty ANSI characters in text.
	stat, _ := os.Stdout.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		DisableColors = true
	}
}

// colorize returns the ANSI code only if colors are enabled,
// otherwise returns an empty string to keep logs clean.
func colorize(colorCode string) string {
	if DisableColors {
		return ""
	}
	return colorCode
}

// --- CENTRALIZED LOGGING SYSTEM ---

// LogNormal prints an informational message with the [tailor] tag
func LogNormal(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Printf("%s[tailor]%s %s\n", colorize(ColorCyan), colorize(ColorReset), msg)
}

// LogSuccess prints a success message
func LogSuccess(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Printf("%s[tailor]%s %s\n", colorize(ColorGreen), colorize(ColorReset), msg)
}

// LogWarning prints a warning message
func LogWarning(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Printf("%s[WARNING]%s %s\n", colorize(ColorYellow), colorize(ColorReset), msg)
}

// LogError prints an error message to standard error
func LogError(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(os.Stderr, "\n%s[ERROR]%s %s\n", colorize(ColorRed), colorize(ColorReset), msg)
}

// Fatal prints an error and exits with code 1
func Fatal(format string, a ...interface{}) {
	LogError(format, a...)
	os.Exit(1)
}

// --- VISUAL HELPERS FOR FORMATTING AND SECTIONS ---

const sectionDivider = "============================================================"

// PrintSection prints a main section divider
func PrintSection(icon, title string) {
	fmt.Println()
	fmt.Printf("%s%s%s\n", colorize(ColorCyan), sectionDivider, colorize(ColorReset))
	fmt.Printf("  %s %s%s%s\n", icon, colorize(ColorBold+ColorWhite), title, colorize(ColorReset))
	fmt.Printf("%s%s%s\n", colorize(ColorCyan), sectionDivider, colorize(ColorReset))
}

// PrintSubSection prints a subsection header (e.g. for individual accessories)
func PrintSubSection(icon, title string) {
	fmt.Println()
	fmt.Printf("%s%s%s %s%s%s\n", colorize(ColorCyan), icon, colorize(ColorReset), colorize(ColorBold), title, colorize(ColorReset))
}

// PrintSummaryBox prints the formatted final summary box
func PrintSummaryBox(title string, rows [][2]string) {
	fmt.Println()
	fmt.Printf("%s%s%s\n", colorize(ColorGreen), sectionDivider, colorize(ColorReset))
	fmt.Printf("  %s%s%s\n", colorize(ColorBold+ColorGreen), title, colorize(ColorReset))
	fmt.Printf("%s%s%s\n", colorize(ColorGreen), sectionDivider, colorize(ColorReset))
	for _, row := range rows {
		fmt.Printf("  %-20s: %s%s%s\n", row[0], colorize(ColorWhite), row[1], colorize(ColorReset))
	}
	fmt.Printf("%s%s%s\n", colorize(ColorGreen), sectionDivider, colorize(ColorReset))
}
