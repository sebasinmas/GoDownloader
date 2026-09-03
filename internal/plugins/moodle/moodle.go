// Package moodle provides an authenticated downloader plugin for Moodle LMS platforms and general HTTP endpoints.
package moodle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"godownloader/internal/kernel"
)

var (
	// ErrAuthenticationFailed indicates invalid or expired session cookie.
	ErrAuthenticationFailed = errors.New("authentication failed: invalid or expired session cookie")
	// ErrUnexpectedStatus indicates non-2xx HTTP status.
	ErrUnexpectedStatus = errors.New("unexpected HTTP response status")
)

func init() {
	kernel.Register(New())
}

// Plugin handles resource downloads from Moodle platforms (e.g. UFRO Campus Virtual) and generic HTTP endpoints.
type Plugin struct {
	client *http.Client
}

// Option configures a Plugin instance.
type Option func(*Plugin)

// WithHTTPClient allows passing a customized http.Client (e.g. for testing).
func WithHTTPClient(client *http.Client) Option {
	return func(p *Plugin) {
		if client != nil {
			p.client = client
		}
	}
}

// New creates a new Plugin with default settings.
func New(opts ...Option) *Plugin {
	p := &Plugin{
		client: defaultHTTPClient(),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func defaultHTTPClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			target := strings.ToLower(req.URL.String())
			if strings.Contains(target, "/login") || strings.Contains(target, "login.php") {
				return fmt.Errorf("%w: redirected to login page (%s)", ErrAuthenticationFailed, req.URL.String())
			}
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			// Propagate cookie header to same-host redirects
			if len(via) > 0 && req.URL.Host == via[0].URL.Host {
				if cookie := via[0].Header.Get("Cookie"); cookie != "" {
					req.Header.Set("Cookie", cookie)
				}
			}
			return nil
		},
		Timeout: 60 * time.Second,
	}
}

// Name returns the identifier of this plugin.
func (p *Plugin) Name() string {
	return "moodle"
}

// CanHandle determines if the given URL is supported.
// Matches HTTP/HTTPS URLs, serving as the Moodle and general HTTP handler.
func (p *Plugin) CanHandle(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// Download downloads the resource specified by task.URL using the given cookie.
func (p *Plugin) Download(ctx context.Context, task kernel.Task, progress kernel.ProgressFunc) (*kernel.Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, task.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if task.Cookie != "" {
		req.Header.Set("Cookie", task.Cookie)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/pdf,*/*")

	resp, err := p.client.Do(req)
	if err != nil {
		if errors.Is(err, ErrAuthenticationFailed) || strings.Contains(err.Error(), ErrAuthenticationFailed.Error()) {
			return nil, ErrAuthenticationFailed
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := checkResponseStatus(resp); err != nil {
		return nil, err
	}

	filename := ExtractFilename(task.URL, resp.Header, task.ID)
	outDir := task.OutputDir
	if outDir == "" {
		outDir = "."
	}

	targetPath := filepath.Join(outDir, filename)
	bytesWritten, err := writeStreamToFile(resp.Body, targetPath, resp.ContentLength, task, filename, progress)
	if err != nil {
		_ = os.Remove(targetPath)
		return nil, err
	}

	return &kernel.Result{
		TaskID:     task.ID,
		URL:        task.URL,
		Filename:   filename,
		BytesRead:  bytesWritten,
		TotalBytes: resp.ContentLength,
		Err:        nil,
	}, nil
}

func checkResponseStatus(resp *http.Response) error {
	if resp.StatusCode == http.StatusSeeOther || resp.StatusCode == http.StatusFound {
		location := resp.Header.Get("Location")
		if strings.Contains(strings.ToLower(location), "login") {
			return fmt.Errorf("%w: redirect to %s", ErrAuthenticationFailed, location)
		}
	}

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("%w: server returned status %d", ErrAuthenticationFailed, resp.StatusCode)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: status %d (%s)", ErrUnexpectedStatus, resp.StatusCode, resp.Status)
	}

	return nil
}

func writeStreamToFile(reader io.Reader, targetPath string, totalBytes int64, task kernel.Task, filename string, progress kernel.ProgressFunc) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return 0, fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(targetPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create file: %w", err)
	}
	defer func() { _ = file.Close() }()

	buf := make([]byte, 32*1024)
	var downloaded int64

	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			if _, writeErr := file.Write(buf[:n]); writeErr != nil {
				return downloaded, fmt.Errorf("failed writing to file: %w", writeErr)
			}
			downloaded += int64(n)
			if progress != nil {
				progress(kernel.ProgressUpdate{
					TaskID:     task.ID,
					URL:        task.URL,
					Filename:   filename,
					BytesRead:  downloaded,
					TotalBytes: totalBytes,
				})
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return downloaded, fmt.Errorf("stream read error: %w", readErr)
		}
	}

	return downloaded, nil
}

// ExtractFilename determines the best filename from the URL, Content-Disposition header, or fallback.
func ExtractFilename(rawURL string, header http.Header, taskID int) string {
	if header != nil {
		if cd := header.Get("Content-Disposition"); cd != "" {
			if _, params, err := mime.ParseMediaType(cd); err == nil {
				if fname, ok := params["filename"]; ok && strings.TrimSpace(fname) != "" {
					return sanitizeFilename(fname)
				}
			}
		}
	}

	parsed, err := url.Parse(rawURL)
	if err == nil {
		base := path.Base(parsed.Path)
		if decoded, unescapeErr := url.PathUnescape(base); unescapeErr == nil && isMeaningfulFilename(decoded) {
			return sanitizeFilename(decoded)
		}

		// Check query parameters (e.g., ?file=name.pdf)
		for _, paramVal := range parsed.Query() {
			for _, val := range paramVal {
				if decoded, unescapeErr := url.QueryUnescape(val); unescapeErr == nil && strings.HasSuffix(strings.ToLower(decoded), ".pdf") {
					return sanitizeFilename(decoded)
				}
			}
		}
	}

	return fmt.Sprintf("download_%d.pdf", taskID)
}

func isMeaningfulFilename(name string) bool {
	clean := strings.ToLower(strings.TrimSpace(name))
	if clean == "" || clean == "." || clean == "/" {
		return false
	}
	// Common web script handlers should not be treated as target media filenames
	scriptExtensions := []string{".php", ".aspx", ".asp", ".jsp", ".do", ".cgi", ".html", ".htm"}
	for _, ext := range scriptExtensions {
		if strings.HasSuffix(clean, ext) {
			return false
		}
	}
	return strings.Contains(clean, ".")
}

func sanitizeFilename(name string) string {
	cleaned := filepath.Base(filepath.Clean(name))
	cleaned = strings.Trim(cleaned, `"' `)
	// Replace problematic path characters
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		"\x00", "",
	)
	cleaned = replacer.Replace(cleaned)
	if cleaned == "" || cleaned == "." {
		return "download.pdf"
	}
	return cleaned
}
