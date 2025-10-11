package log

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Package log provides a thin wrapper around the Go standard library logger.
// It adds:
//   - Named (service/datasource) loggers via ForService(name)
//   - Automatic message prefix: "[<name>>]" (mirrors project spec; we render "[name>]" so there is a visual '>' separator)
//   - Warn and Debug levels (Info is the default level, Error is also provided)
//   - Ability to enable debug globally or selectively per service
//
// This is intentionally minimal so we can gradually refactor existing code that
// currently imports the stdlib log package. Please avoid adding features not requested.
//
// Usage:
//   l := log.ForService("github")
//   l.Infof("indexed %d repos", n)
//   l.Warnf("rate limit low: %d", remaining)
//   l.Debugf("raw response: %s", body) // only prints if debug enabled (globally or for "github")
//
// To enable debug globally:
//   log.SetGlobalDebug(true)
//
// To enable debug for a specific service only:
//   log.EnableDebugFor("github")
//
// NOTE: The package name intentionally collides with stdlib "log". When importing
// this package alongside the standard library, alias one of them, e.g.:
//   import (
//     stdlog "log"
//     ergslog "github.com/your/module/ergs/pkg/log"
//   )
//
// For internal code we will gradually migrate from stdlog to this wrapper.

// Logger represents a named logger with helper methods.
type Logger struct {
	name     string
	std      *log.Logger
	warnOnce sync.Once
}

// writerHolder wraps an io.Writer so that atomic.Value always stores the same
// concrete type, avoiding the "inconsistently typed value" panic when changing
// from *os.File to *bytes.Buffer (or any other writer) in tests or runtime config.
type writerHolder struct {
	w io.Writer
}

var (
	// globalDebug holds global debug enablement.
	globalDebug atomic.Bool

	// serviceDebug stores per-service debug overrides.
	serviceDebug sync.Map // map[string]*atomic.Bool

	// loggers caches created named loggers.
	loggers sync.Map // map[string]*Logger

	// outputWriter holds the destination for all loggers (wrapped in writerHolder).
	outputWriter atomic.Value // writerHolder
)

func init() {
	outputWriter.Store(writerHolder{w: os.Stderr})
}

// ForService returns (and memoizes) a named logger for the given service or datasource.
// The name SHOULD be stable (e.g. datasource slug).
func ForService(name string) *Logger {
	if name == "" {
		name = "unknown"
	}
	if l, ok := loggers.Load(name); ok {
		return l.(*Logger)
	}
	current := outputWriter.Load().(writerHolder).w
	std := log.New(current, "", log.LstdFlags|log.Lmicroseconds)
	logger := &Logger{name: name, std: std}
	actual, _ := loggers.LoadOrStore(name, logger)
	return actual.(*Logger)
}

// SetGlobalDebug enables or disables debug logging globally.
func SetGlobalDebug(enabled bool) {
	globalDebug.Store(enabled)
}

// GlobalDebug returns whether global debug logging is enabled.
func GlobalDebug() bool {
	return globalDebug.Load()
}

// EnableDebugFor enables debug logging for a specific service/datasource.
func EnableDebugFor(name string) {
	if name == "" {
		return
	}
	val, _ := serviceDebug.LoadOrStore(name, &atomic.Bool{})
	val.(*atomic.Bool).Store(true)
}

// DisableDebugFor disables debug logging for a specific service/datasource.
func DisableDebugFor(name string) {
	if name == "" {
		return
	}
	if val, ok := serviceDebug.Load(name); ok {
		val.(*atomic.Bool).Store(false)
	}
}

// DebugEnabledFor returns whether debug is enabled for the given service (either
// globally or specifically for the service).
func DebugEnabledFor(name string) bool {
	if globalDebug.Load() {
		return true
	}
	if val, ok := serviceDebug.Load(name); ok {
		return val.(*atomic.Bool).Load()
	}
	return false
}

// SetOutput sets the output writer for all subsequently created loggers.
// Existing loggers will also adopt the new writer.
func SetOutput(w io.Writer) {
	if w == nil {
		return
	}
	outputWriter.Store(writerHolder{w: w})
	loggers.Range(func(_, v any) bool {
		l := v.(*Logger)
		l.std.SetOutput(w)
		return true
	})
}

// prefix builds the standard prefix for the logger, following the spec.
func (l *Logger) prefix() string {
	return "[" + l.name + ">]"
}

// logInternal formats and outputs the final log line.
func (l *Logger) logInternal(level string, msg string) {
	if level != "" {
		level = level + " "
	}
	l.std.Println(level + l.prefix() + " " + msg)
}

// Infof logs an informational message with fmt.Sprintf semantics.
func (l *Logger) Infof(format string, args ...any) {
	l.logInternal(LevelInfo, fmt.Sprintf(format, args...))
}

// Warnf logs a warning message.
func (l *Logger) Warnf(format string, args ...any) {
	l.warnOnce.Do(func() {
		l.logInternal(LevelWarn, "warnings active for this logger")
	})
	l.logInternal(LevelWarn, fmt.Sprintf(format, args...))
}

// Errorf logs an error message.
func (l *Logger) Errorf(format string, args ...any) {
	l.logInternal(LevelError, fmt.Sprintf(format, args...))
}

// Debugf logs a debug message if debug is enabled (globally or for this logger's service).
func (l *Logger) Debugf(format string, args ...any) {
	if !DebugEnabledFor(l.name) {
		return
	}
	l.logInternal(LevelDebug, fmt.Sprintf(format, args...))
}

// Flush is a no-op placeholder (future: buffered/async logging).
func Flush() {}

// Timestamp returns current time (exposed to allow deterministic overrides in tests later if needed).
var Timestamp = func() time.Time {
	return time.Now()
}

// Level names are currently fixed. Expose constants for potential future checks.
const (
	LevelInfo  = "INFO"
	LevelWarn  = "WARN"
	LevelError = "ERROR"
	LevelDebug = "DEBUG"
)

// TODO (future): structured fields, JSON output, sampling. Avoid implementing until requested.
