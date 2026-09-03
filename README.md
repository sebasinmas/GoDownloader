# GoDownloader

A high-performance, concurrent CLI application written in Go that downloads course resources and PDFs in parallel from authenticated platforms (specifically Campus Virtual UFRO / Moodle). 

Engineered around a **Modular Monolith with a Static Microkernel** architecture and crafted with the **Charmbracelet** terminal ecosystem (`huh`, `bubbletea`, `lipgloss`).

---

## Features

- **Static Microkernel Architecture**: Loose coupling through interface composition (`DownloaderPlugin`). Plugins self-register in memory via package `init()` without dynamic CGO or runtime `.so` plugins.
- **Privacy First**: Session cookies are held strictly in volatile process memory. Cookies are never written to disk, cached, or logged.
- **2-Step Interactive Form**: Polished terminal interface powered by Charmbracelet `huh`:
  1. Secure cookie input (e.g., `MoodleSession=...`).
  2. Multi-line URL text area supporting bulk pasting (e.g., Ctrl+Shift+V from Firefox/Chrome).
- **Parallel Dispatcher**: Concurrently downloads files using goroutines and `sync.WaitGroup`, protected by a concurrency semaphore to prevent socket saturation or server rate-limiting.
- **Smart Filename Extraction**: URL-decodes target filenames (`Apunte%201.1.pdf` → `Apunte 1.1.pdf`), checks `Content-Disposition` headers, handles query parameter fallbacks, and sanitizes against path-traversal exploits.
- **Real-Time Progress Visualization**: Live Bubble Tea + Lip Gloss TUI displaying per-file progress, spinner animations, humanized transfer metrics, and an execution summary.
- **Zero-Warning Code Quality**: 100% test pass rate with race detection (`-race`), strictly compliant with `golangci-lint` (including `gocognit`, `gocyclo`, and `errcheck`).

---

## Project Structure

```
GoDownloader/
├── .github/
│   └── workflows/
│       └── ci.yml               # GitHub Actions CI workflow (tests + linter)
├── .golangci.yml                # Linter configuration (errcheck, gocognit, gocyclo)
├── cmd/
│   └── downloader/
│       └── main.go              # Application entrypoint and dependency injection
├── internal/
│   ├── kernel/
│   │   ├── kernel.go            # Microkernel orchestrator, Plugin interface, static registry
│   │   └── kernel_test.go       # Kernel dispatch, concurrency, and registry unit tests
│   ├── plugins/
│   │   └── moodle/
│   │       ├── moodle.go        # UFRO/Moodle plugin implementation & auto-registration
│   │       └── moodle_test.go   # httptest test suite (200 OK, 303 Redirect, auth check)
│   └── tui/
│       ├── form.go              # 2-step Huh form and URL cleaning/sanitization
│       ├── form_test.go         # URL parser and sanitization tests
│       └── progress.go          # Bubble Tea model with Lip Gloss styles
├── go.mod
├── go.sum
└── README.md
```

---

## Requirements

- **Go**: 1.22 or newer
- **golangci-lint** (optional, for running local linter checks)

---

## Running the CLI

### Direct Execution
```bash
go run ./cmd/downloader
```

### Build Binary
```bash
go build -o bin/godownloader ./cmd/downloader
./bin/godownloader
```

### CLI Flags
```text
Usage of ./bin/godownloader:
  -concurrency int
        Number of concurrent downloads (default 5)
  -output string
        Directory to save downloaded files (default ".")
```

Example specifying custom output folder and concurrency limit:
```bash
./bin/godownloader -output ./downloads -concurrency 8
```

---

## Interactive Workflow

1. **Step 1 — Session Cookie**:
   Copy your active session cookie from your browser's DevTools (`Storage` or `Application` tab → `Cookies` → e.g. `MoodleSession=abc123xyz456`) and paste it into the prompt.
2. **Step 2 — Paste Links**:
   Paste the list of resource links copied from the portal (one per line). Stray quotes and surrounding whitespace are automatically trimmed. Press `Esc` then `Enter` to submit.
3. **Download Phase**:
   The parallel dispatcher resolves the appropriate plugin, injects the authentication cookie into HTTP headers, and downloads all files simultaneously with live progress reporting.

---

## Running Tests & Quality Checks

### Run All Unit & Integration Tests (with Race Detection)
```bash
go test -v -race -coverprofile=coverage.out ./...
```

### View Coverage Report
```bash
go tool cover -func=coverage.out
```

### Run Linter
Verify code quality against `errcheck`, `gocognit`, `gocyclo`, `govet`, `revive`, and `staticcheck`:
```bash
golangci-lint run ./...
```

---

## Extending with New Plugins (e.g. Canvas, AWS S3)

To add support for an additional platform, create a new package inside `internal/plugins/<name>/` and implement the `kernel.DownloaderPlugin` interface:

```go
package canvas

import (
    "context"
    "strings"
    "godownloader/internal/kernel"
)

func init() {
    kernel.Register(&CanvasPlugin{})
}

type CanvasPlugin struct{}

func (c *CanvasPlugin) Name() string {
    return "canvas"
}

func (c *CanvasPlugin) CanHandle(rawURL string) bool {
    return strings.Contains(rawURL, "instructure.com") || strings.Contains(rawURL, "canvas")
}

func (c *CanvasPlugin) Download(ctx context.Context, task kernel.Task, progress kernel.ProgressFunc) (*kernel.Result, error) {
    // Implement Canvas-specific authenticated API/download logic
    return &kernel.Result{TaskID: task.ID, URL: task.URL}, nil
}
```

Then simply import the new plugin in `cmd/downloader/main.go`:
```go
import _ "godownloader/internal/plugins/canvas"
```
The kernel will automatically detect and route matching URLs to your plugin!
