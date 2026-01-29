package logger

import (
	"fmt"
	"time"
)

const (
	LevelInfo  LogLevel = "INFO"
	LevelWarn  LogLevel = "WARN"
	LevelError LogLevel = "ERROR"
	LevelDebug LogLevel = "DEBUG"
)

// NewLogger creates a new logger with the specified maximum size
func NewLogger(maxSize int, minLevel LogLevel) *Logger {
	return &Logger{
		entries:  make([]LogEntry, 0),
		maxSize:  maxSize,
		minLevel: minLevel,
	}
}

// log is the internal logging method
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if levelPriority[level] < levelPriority[l.minLevel] {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	message := fmt.Sprintf(format, args...)
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
	}

	l.entries = append(l.entries, entry)

	// Keep only the last maxSize entries
	if len(l.entries) > l.maxSize {
		l.entries = l.entries[len(l.entries)-l.maxSize:]
	}
}

// Print logs an info message (fmt.Print style)
func (l *Logger) Print(args ...interface{}) {
	l.log(LevelInfo, fmt.Sprint(args...))
}

// Println logs an info message (fmt.Println style)
func (l *Logger) Println(args ...interface{}) {
	l.log(LevelInfo, fmt.Sprint(args...))
}

// Printf logs an info message (fmt.Printf style)
func (l *Logger) Printf(format string, args ...interface{}) {
	l.log(LevelInfo, format, args...)
}

// Info logs an informational message
func (l *Logger) Info(args ...interface{}) {
	l.log(LevelInfo, fmt.Sprint(args...))
}

// Infof logs an informational message with formatting
func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(LevelInfo, format, args...)
}

// Error logs an error message
func (l *Logger) Error(args ...interface{}) {
	l.log(LevelError, fmt.Sprint(args...))
}

// Errorf logs an error message with formatting
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(LevelError, format, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(args ...interface{}) {
	l.log(LevelWarn, fmt.Sprint(args...))
}

// Warnf logs a warning message with formatting
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.log(LevelWarn, format, args...)
}

// Debug logs a debug message
func (l *Logger) Debug(args ...interface{}) {
	l.log(LevelDebug, fmt.Sprint(args...))
}

// Debugf logs a debug message with formatting
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.log(LevelDebug, format, args...)
}

func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minLevel = level
}

// GetEntries returns a copy of all log entries
func (l *Logger) GetEntries() []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// Return a copy
	entries := make([]LogEntry, len(l.entries))
	copy(entries, l.entries)
	return entries
}

// Clear removes all log entries
func (l *Logger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = make([]LogEntry, 0)
}

// Count returns the number of log entries
func (l *Logger) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.entries)
}

// ExportToString returns all logs as a formatted string
func (l *Logger) ExportToString() string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result string
	for _, entry := range l.entries {
		result += fmt.Sprintf("[%s] %-5s %s\n",
			entry.Timestamp.Format("2006-01-02 15:04:05"),
			entry.Level,
			entry.Message)
	}
	return result
}
