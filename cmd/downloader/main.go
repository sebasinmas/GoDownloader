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
	concurrencyFlag := flag.Int("concurrency", 5, "Número de descargas concurrentes")
	outputDirFlag := flag.String("output", ".", "Directorio donde guardar los archivos descargados")

	var logCfg loggerFlag
	flag.Var(&logCfg, "logger", "Habilitar registro de depuración en archivo (por defecto 'godownloader_debug.txt') o especificar ruta (ej. -logger debug.txt)")
	flag.Var(&logCfg, "log", "Alias para -logger")
	logFilePathExplicit := flag.String("logfile", "", "Especificar ruta personalizada para el archivo de registro")
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
			fmt.Println("\nDescarga cancelada por el usuario.")
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error al capturar datos del formulario: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "Error durante la ejecución de las descargas: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "Advertencia: no se pudo inicializar el registro de depuración: %v\n", err)
		return nil, ""
	}

	l.Printf("GoDownloader inicializado. Concurrencia: %d | Directorio: %s", concurrency, outputDir)
	l.Printf("Cookie de sesión: %s", logger.RedactCookie(form.Cookie))
	l.Printf("Total de URLs en cola: %d", len(form.URLs))
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
		fmt.Printf("\n📄 Registro de depuración guardado en: %s\n", logPath)
	}

	if hasErrors {
		os.Exit(1)
	}
}
