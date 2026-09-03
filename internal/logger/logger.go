// Package logger provides structured, privacy-safe debug logging for GoDownloader.
package logger

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Logger writes detailed, privacy-safe execution logs to a file.
type Logger struct {
	mu       sync.Mutex
	file     *os.File
	filePath string
}

// New creates and initializes a Logger writing to targetPath.
func New(targetPath string) (*Logger, error) {
	if strings.TrimSpace(targetPath) == "" {
		targetPath = "godownloader_debug.txt"
	}

	f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	l := &Logger{
		file:     f,
		filePath: targetPath,
	}

	l.writeHeader()
	return l, nil
}

func (l *Logger) writeHeader() {
	header := fmt.Sprintf(
		"================================================================================\n"+
			" GoDownloader Debug Log - %s\n"+
			"================================================================================\n\n",
		time.Now().Format("2006-01-02 15:04:05 MST"),
	)
	_, _ = l.file.WriteString(header)
}

// FilePath returns the location of the log file.
func (l *Logger) FilePath() string {
	return l.filePath
}

// Close flushes and closes the underlying log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		return err
	}
	return nil
}

// Printf logs a formatted message with a timestamp.
func (l *Logger) Printf(format string, args ...any) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}

	timestamp := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(l.file, "[%s] %s\n", timestamp, msg)
}

// LogTaskStart records the beginning of a download task.
func (l *Logger) LogTaskStart(taskID int, rawURL string) {
	l.Printf("[Task %d] QUEUED -> %s", taskID, rawURL)
}

// LogTaskRedirect records an HTTP redirect.
func (l *Logger) LogTaskRedirect(taskID int, fromURL, toURL string, status int) {
	l.Printf("[Task %d] REDIRECT (HTTP %d): %s -> %s", taskID, status, fromURL, toURL)
}

// LogTaskResponse records HTTP response details.
func (l *Logger) LogTaskResponse(taskID int, status int, contentType string, contentLength int64, disposition string) {
	l.Printf("[Task %d] RESPONSE (HTTP %d) Content-Type: %s | Length: %d | Disposition: %s",
		taskID, status, contentType, contentLength, disposition)
}

// LogTaskSuccess records completion of a task.
func (l *Logger) LogTaskSuccess(taskID int, filename string, bytesRead int64) {
	l.Printf("[Task %d] SUCCESS -> Saved '%s' (%d bytes)", taskID, filename, bytesRead)
}

// LogTaskError records a task failure with diagnostic explanation.
func (l *Logger) LogTaskError(taskID int, rawURL string, err error) {
	l.Printf("[Task %d] ERROR for %s: %v", taskID, rawURL, err)
}

// RedactCookie creates a safe, obfuscated summary of a session cookie for logging,
// ensuring secrets are never persisted to disk.
func RedactCookie(rawCookie string) string {
	trimmed := strings.TrimSpace(rawCookie)
	if trimmed == "" {
		return "(none provided)"
	}

	parts := strings.Split(trimmed, ";")
	var redactedParts []string

	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		name := kv[0]
		if len(kv) == 1 {
			redactedParts = append(redactedParts, fmt.Sprintf("%s=[HIDDEN, len: %d]", name, len(name)))
			continue
		}
		val := kv[1]
		if len(val) <= 6 {
			redactedParts = append(redactedParts, fmt.Sprintf("%s=*** (len: %d)", name, len(val)))
		} else {
			prefix := val[:3]
			suffix := val[len(val)-3:]
			redactedParts = append(redactedParts, fmt.Sprintf("%s=%s***%s (len: %d)", name, prefix, suffix, len(val)))
		}
	}

	return strings.Join(redactedParts, "; ")
}
