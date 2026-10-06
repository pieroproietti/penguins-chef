package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultTechnicalLogPath is the shared technical log used by chef.
const DefaultTechnicalLogPath = "/var/log/chef/chef.log"

// LogLevel identifies the severity of an event produced by chef.
type LogLevel string

const (
	LogLevelDebug LogLevel = "DEBUG"
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
)

// LogField is an ordered field attached to a technical log event.
// Ordered fields keep the text format stable and easy to scan.
type LogField struct {
	Key   string
	Value string
}

// TechnicalLogger writes chef events and raw command output to one file.
// It deliberately has no knowledge of distributions or package managers.
type TechnicalLogger struct {
	path string
	mu   sync.Mutex
}

// NewTechnicalLogger creates a logger for path. An empty path uses the
// standard chef technical log.
func NewTechnicalLogger(path string) *TechnicalLogger {
	if path == "" {
		path = DefaultTechnicalLogPath
	}
	return &TechnicalLogger{path: path}
}

// Path returns the destination used by the logger.
func (l *TechnicalLogger) Path() string {
	return l.path
}

// Log writes a structured chef event.
func (l *TechnicalLogger) Log(level LogLevel, message string, fields ...LogField) error {
	var line strings.Builder
	line.WriteString(time.Now().Format("2006-01-02 15:04:05"))
	line.WriteByte(' ')
	line.WriteString(string(level))
	line.WriteString("  ")
	line.WriteString(message)
	for _, field := range fields {
		line.WriteByte(' ')
		line.WriteString(field.Key)
		line.WriteByte('=')
		line.WriteString(formatLogFieldValue(field.Value))
	}
	line.WriteByte('\n')
	return l.write(line.String())
}

func (l *TechnicalLogger) Debug(message string, fields ...LogField) error {
	return l.Log(LogLevelDebug, message, fields...)
}

func (l *TechnicalLogger) Info(message string, fields ...LogField) error {
	return l.Log(LogLevelInfo, message, fields...)
}

func (l *TechnicalLogger) Warn(message string, fields ...LogField) error {
	return l.Log(LogLevelWarn, message, fields...)
}

func (l *TechnicalLogger) Error(message string, fields ...LogField) error {
	return l.Log(LogLevelError, message, fields...)
}

// CommandOutput records a raw line emitted by a command. stream is normally
// stdout or stderr and is intentionally distinct from chef events.
func (l *TechnicalLogger) CommandOutput(stream, line string) error {
	entry := fmt.Sprintf("%s COMMAND %s %s\n", time.Now().Format("2006-01-02 15:04:05"), stream, line)
	return l.write(entry)
}

func (l *TechnicalLogger) write(entry string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(l.path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(entry)
	return err
}

func formatLogFieldValue(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\n\r\"=") {
		return value
	}
	return strconv.Quote(value)
}
