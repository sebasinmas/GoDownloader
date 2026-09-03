package logger_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"godownloader/internal/logger"
)

func TestLogger_WriteAndRead(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test_debug.txt")

	l, err := logger.New(logPath)
	if err != nil {
		t.Fatalf("failed creating logger: %v", err)
	}

	l.Printf("System initialized with %d workers", 5)
	l.LogTaskStart(1, "https://campusvirtual.ufro.cl/test.pdf")
	l.LogTaskRedirect(1, "https://campusvirtual.ufro.cl/view.php", "https://campusvirtual.ufro.cl/pluginfile.php", 303)
	l.LogTaskSuccess(1, "test.pdf", 1024)

	if err := l.Close(); err != nil {
		t.Fatalf("failed closing logger: %v", err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading log file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "GoDownloader Debug Log") {
		t.Errorf("expected header in log, got: %s", contentStr)
	}
	if !strings.Contains(contentStr, "System initialized with 5 workers") {
		t.Errorf("expected worker log entry")
	}
	if !strings.Contains(contentStr, "[Task 1] REDIRECT (HTTP 303)") {
		t.Errorf("expected redirect entry")
	}
}

func TestLogger_ConcurrentLogging(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "concurrent.txt")

	l, err := logger.New(logPath)
	if err != nil {
		t.Fatalf("failed creating logger: %v", err)
	}
	defer func() { _ = l.Close() }()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(taskID int) {
			defer wg.Done()
			l.LogTaskStart(taskID, "https://example.com/file.pdf")
			l.LogTaskSuccess(taskID, "file.pdf", 500)
		}(i)
	}

	wg.Wait()
}

func TestRedactCookie(t *testing.T) {
	tests := []struct {
		input    string
		contains string
		excludes string
	}{
		{
			input:    "",
			contains: "(none provided)",
			excludes: "",
		},
		{
			input:    "MoodleSession=abcdef1234567890ghijkl",
			contains: "MoodleSession=abc***jkl (len: 22)",
			excludes: "1234567890",
		},
		{
			input:    "MoodleSession=12345; user=student",
			contains: "MoodleSession=*** (len: 5)",
			excludes: "12345",
		},
	}

	for _, tc := range tests {
		redacted := logger.RedactCookie(tc.input)
		if !strings.Contains(redacted, tc.contains) {
			t.Errorf("input '%s': expected to contain '%s', got '%s'", tc.input, tc.contains, redacted)
		}
		if tc.excludes != "" && strings.Contains(redacted, tc.excludes) {
			t.Errorf("input '%s': leaked secret '%s' in redacted output '%s'", tc.input, tc.excludes, redacted)
		}
	}
}
