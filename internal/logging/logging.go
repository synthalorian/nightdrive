package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Level represents a log severity level.
type Level int

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	FatalLevel
)

func (l Level) String() string {
	switch l {
	case DebugLevel:
		return "debug"
	case InfoLevel:
		return "info"
	case WarnLevel:
		return "warn"
	case ErrorLevel:
		return "error"
	case FatalLevel:
		return "fatal"
	default:
		return "unknown"
	}
}

// Logger writes structured JSON logs.
type Logger struct {
	out       io.Writer
	level     Level
	mu        sync.Mutex
	component string
}

// New creates a new structured logger.
func New(out io.Writer, level Level) *Logger {
	if out == nil {
		out = os.Stderr
	}
	return &Logger{out: out, level: level}
}

// WithComponent returns a logger scoped to a component name.
func (l *Logger) WithComponent(name string) *Logger {
	return &Logger{out: l.out, level: l.level, component: name}
}

// log writes a structured log entry.
func (l *Logger) log(level Level, msg string, fields map[string]interface{}) {
	if level < l.level {
		return
	}
	entry := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"level":     level.String(),
		"message":   msg,
	}
	if l.component != "" {
		entry["component"] = l.component
	}
	for k, v := range fields {
		entry[k] = v
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = fmt.Fprintln(l.out, string(data))
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, fields ...map[string]interface{}) {
	l.log(DebugLevel, msg, mergeFields(fields))
}

// Info logs an info message.
func (l *Logger) Info(msg string, fields ...map[string]interface{}) {
	l.log(InfoLevel, msg, mergeFields(fields))
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, fields ...map[string]interface{}) {
	l.log(WarnLevel, msg, mergeFields(fields))
}

// Error logs an error message.
func (l *Logger) Error(msg string, fields ...map[string]interface{}) {
	l.log(ErrorLevel, msg, mergeFields(fields))
}

// Fatal logs a fatal message and exits.
func (l *Logger) Fatal(msg string, fields ...map[string]interface{}) {
	l.log(FatalLevel, msg, mergeFields(fields))
	os.Exit(1)
}

func mergeFields(fields []map[string]interface{}) map[string]interface{} {
	if len(fields) == 0 {
		return nil
	}
	m := make(map[string]interface{})
	for _, f := range fields {
		for k, v := range f {
			m[k] = v
		}
	}
	return m
}

// Default is the package-level default logger.
var Default = New(os.Stderr, InfoLevel)

// Package-level helpers.
func Debug(msg string, fields ...map[string]interface{}) { Default.Debug(msg, fields...) }
func Info(msg string, fields ...map[string]interface{})  { Default.Info(msg, fields...) }
func Warn(msg string, fields ...map[string]interface{})  { Default.Warn(msg, fields...) }
func Error(msg string, fields ...map[string]interface{}) { Default.Error(msg, fields...) }
func Fatal(msg string, fields ...map[string]interface{}) { Default.Fatal(msg, fields...) }

// NewFileLogger creates a logger that writes to a rotating file.
func NewFileLogger(dir, prefix string, level Level, maxBytes int64) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, prefix+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	// Wrap with a simple rotation-aware writer.
	w := &rotateWriter{
		file:     f,
		path:     path,
		maxBytes: maxBytes,
	}
	return New(w, level), nil
}

type rotateWriter struct {
	file     *os.File
	path     string
	maxBytes int64
	size     int64
	mu       sync.Mutex
}

func (w *rotateWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size+int64(len(p)) > w.maxBytes && w.maxBytes > 0 {
		// Rotate: close current, rename to .1, open new.
		w.file.Close()
		_ = os.Rename(w.path, w.path+".1")
		f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return 0, err
		}
		w.file = f
		w.size = 0
	}
	n, err = w.file.Write(p)
	w.size += int64(n)
	return n, err
}

// SetDefaultLevel adjusts the global default logger level.
func SetDefaultLevel(l Level) {
	Default.level = l
}

// GoStdlibAdapter returns a *log.Logger that writes through the structured logger.
func (l *Logger) GoStdlibAdapter() *log.Logger {
	return log.New(&stdLibAdapter{logger: l}, "", 0)
}

type stdLibAdapter struct {
	logger *Logger
}

func (a *stdLibAdapter) Write(p []byte) (n int, err error) {
	msg := string(p)
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}
	a.logger.Info(msg)
	return len(p), nil
}

// StackTrace returns a string representation of the current goroutine stack.
func StackTrace() string {
	buf := make([]byte, 64<<10)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}
