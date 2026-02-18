package utils

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

const (
	colorReset  = "\033[0m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
)

type LoggerHandler struct {
	level    LogLevel
	useColor bool
	logger   *log.Logger
	mu       sync.Mutex
}

func NewLoggerHandler(level string) *LoggerHandler {
	return &LoggerHandler{
		level:    parseLogLevel(level),
		useColor: shouldUseColor(),
		logger:   log.New(os.Stdout, "", 0),
	}
}

func parseLogLevel(level string) LogLevel {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "DEBUG":
		return LogLevelDebug
	case "INFO":
		return LogLevelInfo
	case "WARN", "WARNING":
		return LogLevelWarn
	case "ERROR":
		return LogLevelError
	default:
		return LogLevelInfo
	}
}

func shouldUseColor() bool {
	if strings.EqualFold(os.Getenv("NO_COLOR"), "1") || strings.EqualFold(os.Getenv("NO_COLOR"), "true") {
		return false
	}
	if strings.EqualFold(os.Getenv("LOG_COLOR"), "0") || strings.EqualFold(os.Getenv("LOG_COLOR"), "false") {
		return false
	}
	return true
}

func (l *LoggerHandler) SetLevel(level string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = parseLogLevel(level)
}

func (l *LoggerHandler) SetUseColor(useColor bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.useColor = useColor
}

func (l *LoggerHandler) Debugf(format string, args ...interface{}) {
	l.logf(LogLevelDebug, format, args...)
}

func (l *LoggerHandler) Infof(format string, args ...interface{}) {
	l.logf(LogLevelInfo, format, args...)
}

func (l *LoggerHandler) Warnf(format string, args ...interface{}) {
	l.logf(LogLevelWarn, format, args...)
}

func (l *LoggerHandler) Errorf(format string, args ...interface{}) {
	l.logf(LogLevelError, format, args...)
}

func (l *LoggerHandler) logf(level LogLevel, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if level < l.level {
		return
	}

	timestamp := time.Now().Format(time.RFC3339)
	levelText := levelToText(level)
	color := levelToColor(level)
	message := fmt.Sprintf(format, args...)
	source := callerFileName()

	if l.useColor {
		l.logger.Printf("%s[%s] [%s] [%s]%s %s", color, timestamp, levelText, source, colorReset, message)
		return
	}

	l.logger.Printf("[%s] [%s] [%s] %s", timestamp, levelText, source, message)
}

func callerFileName() string {
	const thisFile = "logger_handler.go"

	pcs := make([]uintptr, 16)
	n := runtime.Callers(2, pcs)
	if n == 0 {
		return "unknown:0"
	}

	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		base := filepath.Base(frame.File)
		if base != thisFile {
			ext := filepath.Ext(base)
			return strings.TrimSuffix(base, ext)
		}
		if !more {
			break
		}
	}

	return "unknown"
}

func levelToText(level LogLevel) string {
	switch level {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

func levelToColor(level LogLevel) string {
	switch level {
	case LogLevelDebug:
		return colorCyan
	case LogLevelInfo:
		return colorGreen
	case LogLevelWarn:
		return colorYellow
	case LogLevelError:
		return colorRed
	default:
		return colorGreen
	}
}

var defaultLogger = NewLoggerHandler(os.Getenv("LOG_LEVEL"))

func SetLogLevel(level string) {
	defaultLogger.SetLevel(level)
}

func SetLogColor(useColor bool) {
	defaultLogger.SetUseColor(useColor)
}

func Debugf(format string, args ...interface{}) {
	defaultLogger.Debugf(format, args...)
}

func Infof(format string, args ...interface{}) {
	defaultLogger.Infof(format, args...)
}

func Warnf(format string, args ...interface{}) {
	defaultLogger.Warnf(format, args...)
}

func Errorf(format string, args ...interface{}) {
	defaultLogger.Errorf(format, args...)
}
