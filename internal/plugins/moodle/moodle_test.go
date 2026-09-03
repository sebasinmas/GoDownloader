package moodle_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"godownloader/internal/kernel"
	"godownloader/internal/plugins/moodle"
)

var dummyPDF = []byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF")

func TestMoodle_AuthenticatedSuccess200(t *testing.T) {
	validCookie := "MoodleSession=ufro_secret_session_token_123"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != validCookie {
			http.Redirect(w, r, "/login/index.php", http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(dummyPDF)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	plugin := moodle.New()

	task := kernel.Task{
		ID:        1,
		URL:       server.URL + "/pluginfile.php/123/Apunte%201.1.pdf",
		Cookie:    validCookie,
		OutputDir: tempDir,
	}

	var progressCalled bool
	res, err := plugin.Download(context.Background(), task, func(u kernel.ProgressUpdate) {
		progressCalled = true
		if u.BytesRead <= 0 {
			t.Errorf("expected positive bytes read, got %d", u.BytesRead)
		}
	})

	if err != nil {
		t.Fatalf("expected successful download, got error: %v", err)
	}

	if res.Filename != "Apunte 1.1.pdf" {
		t.Errorf("expected filename 'Apunte 1.1.pdf', got '%s'", res.Filename)
	}

	expectedPath := filepath.Join(tempDir, "Apunte 1.1.pdf")
	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed reading saved file: %v", err)
	}

	if !bytes.Equal(content, dummyPDF) {
		t.Errorf("saved file content does not match expected dummy PDF")
	}

	if !progressCalled {
		t.Errorf("expected progress callback to be called")
	}
}

func TestMoodle_UnauthenticatedRedirect303(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulates unauthenticated Moodle redirecting to login
		if r.URL.Path == "/login/index.php" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html>Login Page</html>"))
			return
		}
		http.Redirect(w, r, "/login/index.php", http.StatusSeeOther)
	}))
	defer server.Close()

	plugin := moodle.New()
	task := kernel.Task{
		ID:        1,
		URL:       server.URL + "/mod/resource/view.php?id=999",
		Cookie:    "MoodleSession=invalid_expired_cookie",
		OutputDir: t.TempDir(),
	}

	_, err := plugin.Download(context.Background(), task, nil)
	if err == nil {
		t.Fatalf("expected authentication error, got nil")
	}

	if !errors.Is(err, moodle.ErrAuthenticationFailed) {
		t.Errorf("expected ErrAuthenticationFailed, got %v", err)
	}
}

func TestMoodle_Unauthenticated403(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	plugin := moodle.New()
	task := kernel.Task{
		ID:        1,
		URL:       server.URL + "/resource.pdf",
		Cookie:    "",
		OutputDir: t.TempDir(),
	}

	_, err := plugin.Download(context.Background(), task, nil)
	if err == nil {
		t.Fatalf("expected error on 403, got nil")
	}
	if !errors.Is(err, moodle.ErrAuthenticationFailed) {
		t.Errorf("expected ErrAuthenticationFailed, got %v", err)
	}
}

func TestMoodle_ServerError500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	plugin := moodle.New()
	task := kernel.Task{
		ID:        2,
		URL:       server.URL + "/broken.pdf",
		Cookie:    "test=1",
		OutputDir: tempDir,
	}

	_, err := plugin.Download(context.Background(), task, nil)
	if err == nil {
		t.Fatalf("expected error on 500, got nil")
	}

	// Verify no stray file remains
	files, _ := os.ReadDir(tempDir)
	if len(files) != 0 {
		t.Errorf("expected temporary directory to be clean on failure, found %d files", len(files))
	}
}

func TestMoodle_ExtractFilename(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		headerCD string
		taskID   int
		expected string
	}{
		{
			name:     "URL encoded spaces and symbols",
			rawURL:   "https://campusvirtual.ufro.cl/mod/resource/content/1/Apunte%201.1.pdf",
			headerCD: "",
			taskID:   1,
			expected: "Apunte 1.1.pdf",
		},
		{
			name:     "Content-Disposition takes precedence over generic URL",
			rawURL:   "https://campusvirtual.ufro.cl/mod/resource/view.php?id=5543",
			headerCD: `attachment; filename="Guia_Calculo_II.pdf"`,
			taskID:   2,
			expected: "Guia_Calculo_II.pdf",
		},
		{
			name:     "Query param containing pdf filename",
			rawURL:   "https://portal.edu/download.php?file=Laboratorio%203.pdf",
			headerCD: "",
			taskID:   3,
			expected: "Laboratorio 3.pdf",
		},
		{
			name:     "Path traversal attack sanitization",
			rawURL:   "https://campusvirtual.ufro.cl/../../etc/passwd.pdf",
			headerCD: "",
			taskID:   4,
			expected: "passwd.pdf",
		},
		{
			name:     "Fallback to task ID when no filename is deducible",
			rawURL:   "https://campusvirtual.ufro.cl/view.php?id=123",
			headerCD: "",
			taskID:   5,
			expected: "download_5.pdf",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var h http.Header
			if tc.headerCD != "" {
				h = make(http.Header)
				h.Set("Content-Disposition", tc.headerCD)
			}
			result := moodle.ExtractFilename(tc.rawURL, h, tc.taskID)
			if result != tc.expected {
				t.Errorf("expected '%s', got '%s'", tc.expected, result)
			}
		})
	}
}

func TestMoodle_KernelIntegration(t *testing.T) {
	// Verifies that the plugin registered itself via init()
	k := kernel.New()
	p, err := k.ResolvePlugin("https://campusvirtual.ufro.cl/mod/resource/view.php?id=123")
	if err != nil {
		t.Fatalf("expected moodle plugin to resolve for URL: %v", err)
	}
	if p.Name() != "moodle" {
		t.Errorf("expected plugin name 'moodle', got '%s'", p.Name())
	}
}
