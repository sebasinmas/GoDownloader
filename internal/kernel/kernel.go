// Package kernel implements the static microkernel orchestrator, plugin registry, and concurrent task dispatcher.
package kernel

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	// ErrNoPluginFound is returned when no registered plugin can handle the specified URL.
	ErrNoPluginFound = errors.New("no registered plugin can handle the provided URL")
	// ErrNilPlugin is returned when attempting to register a nil plugin.
	ErrNilPlugin = errors.New("cannot register a nil plugin")
)

// Task represents an individual download unit dispatched by the kernel.
type Task struct {
	ID        int
	URL       string
	Cookie    string
	OutputDir string
}

// Result captures the final status and outcome of a download task.
type Result struct {
	TaskID     int
	URL        string
	Filename   string
	BytesRead  int64
	TotalBytes int64
	Err        error
}

// ProgressUpdate contains real-time stream information for a running task.
type ProgressUpdate struct {
	TaskID     int
	URL        string
	Filename   string
	BytesRead  int64
	TotalBytes int64
}

// ProgressFunc is a callback invoked during payload transfer.
type ProgressFunc func(update ProgressUpdate)

// EventType categorizes dispatcher event notifications.
type EventType int

const (
	// EventTaskStarted indicates a task was picked up by an active worker.
	EventTaskStarted EventType = iota
	// EventTaskProgress indicates byte transfer progress for a task.
	EventTaskProgress
	// EventTaskCompleted indicates a task finished successfully.
	EventTaskCompleted
	// EventTaskFailed indicates a task terminated with an error.
	EventTaskFailed
)

// Event represents an atomic status transition sent to kernel observers.
type Event struct {
	Type     EventType
	TaskID   int
	URL      string
	Filename string
	Bytes    int64
	Total    int64
	Err      error
}

// EventHandler receives real-time download events from the kernel dispatcher.
type EventHandler func(event Event)

// DownloaderPlugin defines the microkernel contract that all domain download handlers must fulfill.
type DownloaderPlugin interface {
	Name() string
	CanHandle(rawURL string) bool
	Download(ctx context.Context, task Task, progress ProgressFunc) (*Result, error)
}

// Global registry for plugins.
var (
	registryMu sync.RWMutex
	plugins    []DownloaderPlugin
)

// Register adds a new plugin to the global static microkernel registry.
// Typically invoked inside a plugin package's init() function.
func Register(p DownloaderPlugin) {
	if p == nil {
		panic(ErrNilPlugin)
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	for _, existing := range plugins {
		if existing.Name() == p.Name() {
			return // Idempotent registration
		}
	}
	plugins = append(plugins, p)
}

// RegisteredPlugins returns a copy of all currently registered plugins in order of addition.
func RegisteredPlugins() []DownloaderPlugin {
	registryMu.RLock()
	defer registryMu.RUnlock()

	result := make([]DownloaderPlugin, len(plugins))
	copy(result, plugins)
	return result
}

// ResetRegistry clears the static registry. Primarily used in unit tests.
func ResetRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()
	plugins = nil
}

// Kernel orchestrates URL dispatching and concurrency management across registered plugins.
type Kernel struct {
	plugins     []DownloaderPlugin
	concurrency int
}

// Option configures a Kernel instance.
type Option func(*Kernel)

// WithConcurrency configures the maximum number of parallel downloads.
func WithConcurrency(limit int) Option {
	return func(k *Kernel) {
		if limit > 0 {
			k.concurrency = limit
		}
	}
}

// WithPlugins overrides the plugin set with a custom slice instead of the global registry.
func WithPlugins(custom []DownloaderPlugin) Option {
	return func(k *Kernel) {
		k.plugins = make([]DownloaderPlugin, len(custom))
		copy(k.plugins, custom)
	}
}

// New creates an initialized Kernel instance using either configured options or the global registry.
func New(opts ...Option) *Kernel {
	k := &Kernel{
		plugins:     RegisteredPlugins(),
		concurrency: 5, // Sensible default to prevent DDoS/throttling
	}

	for _, opt := range opts {
		opt(k)
	}

	return k
}

// ResolvePlugin finds the first registered plugin that declares it can handle the given URL.
func (k *Kernel) ResolvePlugin(rawURL string) (DownloaderPlugin, error) {
	for _, p := range k.plugins {
		if p.CanHandle(rawURL) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNoPluginFound, rawURL)
}

// Dispatch executes the given list of tasks concurrently using goroutines and sync.WaitGroup,
// constrained by the kernel's concurrency limit.
func (k *Kernel) Dispatch(ctx context.Context, tasks []Task, onEvent EventHandler) []Result {
	results := make([]Result, len(tasks))
	if len(tasks) == 0 {
		return results
	}

	sem := make(chan struct{}, k.concurrency)
	var wg sync.WaitGroup

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, t Task) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[idx] = Result{
					TaskID: t.ID,
					URL:    t.URL,
					Err:    ctx.Err(),
				}
				emitEvent(onEvent, Event{
					Type:   EventTaskFailed,
					TaskID: t.ID,
					URL:    t.URL,
					Err:    ctx.Err(),
				})
				return
			}

			results[idx] = k.executeTask(ctx, t, onEvent)
		}(i, task)
	}

	wg.Wait()
	return results
}

func (k *Kernel) executeTask(ctx context.Context, task Task, onEvent EventHandler) Result {
	emitEvent(onEvent, Event{
		Type:   EventTaskStarted,
		TaskID: task.ID,
		URL:    task.URL,
	})

	plugin, err := k.ResolvePlugin(task.URL)
	if err != nil {
		res := Result{
			TaskID: task.ID,
			URL:    task.URL,
			Err:    err,
		}
		emitEvent(onEvent, Event{
			Type:   EventTaskFailed,
			TaskID: task.ID,
			URL:    task.URL,
			Err:    err,
		})
		return res
	}

	progressWrapper := func(u ProgressUpdate) {
		emitEvent(onEvent, Event{
			Type:     EventTaskProgress,
			TaskID:   u.TaskID,
			URL:      u.URL,
			Filename: u.Filename,
			Bytes:    u.BytesRead,
			Total:    u.TotalBytes,
		})
	}

	res, err := plugin.Download(ctx, task, progressWrapper)
	if err != nil {
		failureResult := Result{
			TaskID: task.ID,
			URL:    task.URL,
			Err:    err,
		}
		emitEvent(onEvent, Event{
			Type:   EventTaskFailed,
			TaskID: task.ID,
			URL:    task.URL,
			Err:    err,
		})
		return failureResult
	}

	emitEvent(onEvent, Event{
		Type:     EventTaskCompleted,
		TaskID:   res.TaskID,
		URL:      res.URL,
		Filename: res.Filename,
		Bytes:    res.BytesRead,
		Total:    res.TotalBytes,
	})
	return *res
}

func emitEvent(handler EventHandler, event Event) {
	if handler != nil {
		handler(event)
	}
}
