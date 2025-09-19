package logger

import (
	"io"
	"log"
	"os"
	"strings"
)

type LogLevel int

// Log levels
const (
	LevelTrace LogLevel = iota
	LevelDebug
	LevelInfo
	LevelWarning
	LevelError
)

var currentLogLevel LogLevel = LevelInfo // Default log level

// Function to set global log flags
func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds) // Standard log flags
	log.SetOutput(os.Stdout)
}

// Method to return the string representation of the LogLevel
func (l LogLevel) String() string {
	switch l {
	case LevelTrace:
		return "TRACE"
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarning:
		return "WARNING"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Function to infer the log level from a string
func ParseLogLevel(levelStr string) LogLevel {
	switch strings.ToUpper(levelStr) {
	case "TRACE":
		return LevelTrace
	case "DEBUG":
		return LevelDebug
	case "INFO":
		return LevelInfo
	case "WARNING":
		return LevelWarning
	case "ERROR":
		return LevelError
	default:
		log.Printf("WARNING: Invalid log level '%s' in config. Defaulting to INFO.", levelStr)
		return LevelInfo
	}
}

// Function to set the global log level
func SetLogLevel(level LogLevel) {
	currentLogLevel = level
	log.Printf("INFO: Log level set to %s", level)
}

// Function to get the current global log level
func GetLogLevel() LogLevel {
	return currentLogLevel
}

// Function wrapper for stdlib log.SetOutput
func SetOutput(w io.Writer) {
	log.SetOutput(w)
}

// Function to log TRACE level messages
func Tracef(format string, v ...interface{}) {
	if currentLogLevel <= LevelTrace {
		log.Printf("TRACE: "+format, v...)
	}
}

// Function to log DEBUG level messages
func Debugf(format string, v ...interface{}) {
	if currentLogLevel <= LevelDebug {
		log.Printf("DEBUG: "+format, v...)
	}
}

// Function to log INFO level messages
func Infof(format string, v ...interface{}) {
	if currentLogLevel <= LevelInfo {
		log.Printf("INFO: "+format, v...)
	}
}

// Function to log WARNING level messages
func Warnf(format string, v ...interface{}) {
	if currentLogLevel <= LevelWarning {
		log.Printf("WARNING: "+format, v...)
	}
}

// Function to log ERROR level messages
func Errorf(format string, v ...interface{}) {
	if currentLogLevel <= LevelError {
		log.Printf("ERROR: "+format, v...)
	}
}

// Function to log FATAL level messages and exit
func Fatalf(format string, v ...interface{}) {
	log.Fatalf("FATAL: "+format, v...)
}

// LineWriter is an io.Writer that logs each line written to it.
// It allows treating the logger as a standard writer, which can be useful
// for redirecting output from other components or libraries into the logging system.
type LineWriter struct {
	level  LogLevel
	prefix string
}

// Function to create a new LineWriter
func NewLineWriter(level LogLevel, prefix string) *LineWriter {
	return &LineWriter{
		level:  level,
		prefix: prefix,
	}
}

// Method to implement the io.Writer interface
func (lw *LineWriter) Write(p []byte) (n int, err error) {
	// Only process if the specified level is enabled
	if currentLogLevel > lw.level {
		return len(p), nil
	}

	lines := strings.SplitAfter(string(p), "\n")

	for _, line := range lines {
		line = strings.TrimRight(line, "\n")
		if line == "" {
			continue
		}

		// Add prefix if specified
		logLine := line
		if lw.prefix != "" {
			logLine = lw.prefix + " " + line
		}

		// Log at the specified level
		switch lw.level {
		case LevelDebug:
			Debugf("%s", logLine)
		case LevelInfo:
			Infof("%s", logLine)
		case LevelWarning:
			Warnf("%s", logLine)
		case LevelError:
			Errorf("%s", logLine)
		}
	}

	return len(p), nil
}
