package logger

import (
	"sync"
	"time"
)

//
// LOGGER
//

// LogLevel represents the severity of a log entry
type LogLevel string

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time
	Level     LogLevel
	Message   string
}

// Logger handles all logging throughout the application
type Logger struct {
	mu      sync.RWMutex
	entries []LogEntry
	maxSize int
}
