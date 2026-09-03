// Package main provides the CLI entrypoint for GoDownloader.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"godownloader/internal/kernel"
	_ "godownloader/internal/plugins/moodle" // Auto-registers Moodle / HTTP plugin
	"godownloader/internal/tui"
)

func main() {
	concurrencyFlag := flag.Int("concurrency", 5, "Number of concurrent downloads")
	outputDirFlag := flag.String("output", ".", "Directory to save downloaded files")
	flag.Parse()

	formData, err := tui.RunInteractiveForm()
	if err != nil {
		if errors.Is(err, tui.ErrFormAborted) {
			fmt.Println("\nDownload cancelled by user.")
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error gathering form input: %v\n", err)
		os.Exit(1)
	}

	tasks := make([]kernel.Task, len(formData.URLs))
	for i, u := range formData.URLs {
		tasks[i] = kernel.Task{
			ID:        i + 1,
			URL:       u,
			Cookie:    formData.Cookie,
			OutputDir: *outputDirFlag,
		}
	}

	k := kernel.New(kernel.WithConcurrency(*concurrencyFlag))

	results, err := tui.RunProgressUI(k, tasks)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error during download execution: %v\n", err)
		os.Exit(1)
	}

	var hasErrors bool
	for _, res := range results {
		if res.Err != nil {
			hasErrors = true
			break
		}
	}

	if hasErrors {
		os.Exit(1)
	}
}
