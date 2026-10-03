package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

func ensureRootPath() {
	if os.Geteuid() != 0 {
		return
	}

	const rootPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

	if os.Getenv("PATH") != rootPath {
		os.Setenv("PATH", rootPath)
	}
}

// Exec executes a shell command and displays real-time output in the terminal.
func Exec(command string) error {
	ensureRootPath()

	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ExecTee executes a shell command with interactive stdin, streaming stdout and stderr
// both to the terminal (or DECSTBM scrolling region) and to the specified log file.
func ExecTee(command string, logFilePath string) error {
	ensureRootPath()

	if err := os.MkdirAll(filepath.Dir(logFilePath), 0755); err != nil {
		// fallback
	}

	f, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return Exec(command)
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	f.WriteString(fmt.Sprintf("[%s] EXEC: %s\n", timestamp, command))

	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = io.MultiWriter(os.Stdout, f)
	cmd.Stderr = io.MultiWriter(os.Stderr, f)
	return cmd.Run()
}

// ExecLogOnly executes a command while preserving its stdout and stderr in
// the technical log without writing either stream to the terminal.
func ExecLogOnly(command string, logFilePath string) error {
	return ExecWithTechnicalLogger(command, NewTechnicalLogger(logFilePath))
}

// ExecWithTechnicalLogger executes a generic shell command and records its
// lifecycle plus raw stdout/stderr in logger. It has no package-manager or
// distribution-specific behavior.
func ExecWithTechnicalLogger(command string, logger *TechnicalLogger) error {
	ensureRootPath()
	if logger == nil {
		logger = NewTechnicalLogger("")
	}

	_ = logger.Info("command started", LogField{Key: "command", Value: command})
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = os.Stdin

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = logger.Error("command setup failed", LogField{Key: "command", Value: command}, LogField{Key: "error", Value: err.Error()})
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = logger.Error("command setup failed", LogField{Key: "command", Value: command}, LogField{Key: "error", Value: err.Error()})
		return err
	}
	if err := cmd.Start(); err != nil {
		_ = logger.Error("command start failed", LogField{Key: "command", Value: command}, LogField{Key: "error", Value: err.Error()})
		return err
	}

	var readers sync.WaitGroup
	readOutput := func(stream string, r io.Reader) {
		defer readers.Done()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			_ = logger.CommandOutput(stream, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			_ = logger.Warn("command output read failed", LogField{Key: "stream", Value: stream}, LogField{Key: "error", Value: err.Error()})
		}
	}
	readers.Add(2)
	go readOutput("stdout", stdout)
	go readOutput("stderr", stderr)

	readers.Wait()
	err = cmd.Wait()
	exitCode := cmd.ProcessState.ExitCode()
	if err != nil {
		_ = logger.Error("command finished", LogField{Key: "exit_code", Value: strconv.Itoa(exitCode)}, LogField{Key: "error", Value: err.Error()})
		return err
	}
	_ = logger.Info("command finished", LogField{Key: "exit_code", Value: strconv.Itoa(exitCode)})
	return nil
}

// ExecInteractive executes an interactive command with logged terminal output.
func ExecInteractive(command string, logFilePath string) error {
	return ExecTee(command, logFilePath)
}

// ExecQuiet executes a command without printing anything to stdout/stderr.
func ExecQuiet(command string) error {
	ensureRootPath()

	cmd := exec.Command("sh", "-c", command)
	return cmd.Run()
}

// ExecLog executes a shell command and redirects stdout and stderr to the specified log file.
func ExecLog(command string, logFilePath string) error {
	return ExecLogMonitor(command, logFilePath, nil)
}

// ExecLogMonitor executes a shell command, writing everything to logfile and notifying onLine per line.
func ExecLogMonitor(command string, logFilePath string, onLine func(line string)) error {
	ensureRootPath()

	if err := os.MkdirAll(filepath.Dir(logFilePath), 0755); err != nil {
		// In case of directory error, continue
	}

	f, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return ExecQuiet(command)
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	f.WriteString(fmt.Sprintf("[%s] EXEC: %s\n", timestamp, command))

	cmd := exec.Command("sh", "-c", command)

	r, w, err := os.Pipe()
	if err != nil {
		cmd.Stdout = f
		cmd.Stderr = f
		return cmd.Run()
	}
	cmd.Stdout = w
	cmd.Stderr = w

	if err := cmd.Start(); err != nil {
		w.Close()
		r.Close()
		return err
	}
	w.Close()

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		f.WriteString(line + "\n")
		if onLine != nil {
			onLine(line)
		}
	}
	r.Close()

	return cmd.Wait()
}

// ExecCapture executes a command and returns the output as a string.
func ExecCapture(command string) (string, error) {
	ensureRootPath()

	var out bytes.Buffer
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = &out
	return out.String(), cmd.Run()
}

// ExecCaptureCombined executes a command and returns combined stdout and stderr as a string.
func ExecCaptureCombined(command string) (string, error) {
	ensureRootPath()

	var out bytes.Buffer
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}
