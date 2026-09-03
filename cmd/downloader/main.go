// Package main provides the CLI entrypoint for GoDownloader.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"godownloader/internal/kernel"
	"godownloader/internal/logger"
	"godownloader/internal/plugins/moodle"
	"godownloader/internal/tui"
)

type loggerFlag struct {
	enabled  bool
	filePath string
}

func (f *loggerFlag) String() string {
	return f.filePath
}

func (f *loggerFlag) Set(s string) error {
	f.enabled = true
	switch s {
	case "true", "":
		f.filePath = "godownloader_debug.txt"
	case "false":
		f.enabled = false
	default:
		f.filePath = s
	}
	return nil
}

func (f *loggerFlag) IsBoolFlag() bool {
	return true
}

func main() {
	concurrencyFlag := flag.Int("concurrency", 5, "Number of concurrent downloads")
	outputDirFlag := flag.String("output", ".", "Directory to save downloaded files")

	var logCfg loggerFlag
	flag.Var(&logCfg, "logger", "Enable debug logging to file (default 'godownloader_debug.txt') or specify path (e.g. -logger debug.txt)")
	flag.Var(&logCfg, "log", "Alias for -logger")
	logFilePathExplicit := flag.String("logfile", "", "Specify custom log file path")
	flag.Parse()

	// Check if a path argument followed -logger or -log
	if flag.NArg() > 0 && logCfg.enabled && logCfg.filePath == "godownloader_debug.txt" {
		arg := flag.Arg(0)
		if strings.HasSuffix(arg, ".txt") || strings.HasSuffix(arg, ".log") {
			logCfg.filePath = arg
		}
	}
	if *logFilePathExplicit != "" {
		logCfg.enabled = true
		logCfg.filePath = *logFilePathExplicit
	}

	formData, err := tui.RunInteractiveForm()
	if err != nil {
		if errors.Is(err, tui.ErrFormAborted) {
			fmt.Println("\nDownload cancelled by user.")
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error gathering form input: %v\n", err)
		os.Exit(1)
	}

	appLogger, logPath := setupLogger(logCfg, *concurrencyFlag, *outputDirFlag, formData)
	if appLogger != nil {
		defer func() { _ = appLogger.Close() }()
	}

	tasks := createTasks(formData, *outputDirFlag)
	k := initKernel(*concurrencyFlag, appLogger)

	results, err := tui.RunProgressUI(k, tasks, logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error during download execution: %v\n", err)
		os.Exit(1)
	}

	handleCompletion(results, logPath)
}

func setupLogger(cfg loggerFlag, concurrency int, outputDir string, form *tui.FormData) (*logger.Logger, string) {
	if !cfg.enabled {
		return nil, ""
	}

	l, err := logger.New(cfg.filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize debug logger: %v\n", err)
		return nil, ""
	}

	l.Printf("GoDownloader initialized. Concurrency: %d | Output: %s", concurrency, outputDir)
	l.Printf("Session Cookie: %s", logger.RedactCookie(form.Cookie))
	l.Printf("Total URLs Queued: %d", len(form.URLs))
	return l, cfg.filePath
}

func createTasks(form *tui.FormData, outputDir string) []kernel.Task {
	tasks := make([]kernel.Task, len(form.URLs))
	for i, u := range form.URLs {
		tasks[i] = kernel.Task{
			ID:        i + 1,
			URL:       u,
			Cookie:    form.Cookie,
			OutputDir: outputDir,
		}
	}
	return tasks
}

func initKernel(concurrency int, l *logger.Logger) *kernel.Kernel {
	opts := []kernel.Option{
		kernel.WithConcurrency(concurrency),
	}
	if l != nil {
		opts = append(opts,
			kernel.WithLogger(l),
			kernel.WithPlugins([]kernel.DownloaderPlugin{
				moodle.New(moodle.WithLogger(l)),
			}),
		)
	}
	return kernel.New(opts...)
}

func handleCompletion(results []kernel.Result, logPath string) {
	var hasErrors bool
	for _, res := range results {
		if res.Err != nil {
			hasErrors = true
			break
		}
	}

	if logPath != "" {
		fmt.Printf("\n📄 Debug log written to: %s\n", logPath)
	}

	if hasErrors {
		os.Exit(1)
	}
}
