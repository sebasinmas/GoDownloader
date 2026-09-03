package kernel_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"godownloader/internal/kernel"
)

type mockPlugin struct {
	name      string
	prefix    string
	delay     time.Duration
	failWith  error
	callCount int64
}

func (m *mockPlugin) Name() string {
	return m.name
}

func (m *mockPlugin) CanHandle(rawURL string) bool {
	return strings.HasPrefix(rawURL, m.prefix)
}

func (m *mockPlugin) Download(ctx context.Context, task kernel.Task, progress kernel.ProgressFunc) (*kernel.Result, error) {
	atomic.AddInt64(&m.callCount, 1)

	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if progress != nil {
		progress(kernel.ProgressUpdate{
			TaskID:     task.ID,
			URL:        task.URL,
			Filename:   "mock.pdf",
			BytesRead:  100,
			TotalBytes: 100,
		})
	}

	if m.failWith != nil {
		return nil, m.failWith
	}

	return &kernel.Result{
		TaskID:     task.ID,
		URL:        task.URL,
		Filename:   "mock.pdf",
		BytesRead:  100,
		TotalBytes: 100,
		Err:        nil,
	}, nil
}

func TestRegistry_RegisterAndRetrieve(t *testing.T) {
	kernel.ResetRegistry()
	defer kernel.ResetRegistry()

	p1 := &mockPlugin{name: "p1", prefix: "https://p1.test"}
	kernel.Register(p1)

	// Idempotent test
	kernel.Register(p1)

	registered := kernel.RegisteredPlugins()
	if len(registered) != 1 {
		t.Fatalf("expected 1 plugin registered, got %d", len(registered))
	}
	if registered[0].Name() != "p1" {
		t.Errorf("expected plugin name 'p1', got '%s'", registered[0].Name())
	}
}

func TestRegistry_NilPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic when registering nil plugin, got none")
		}
	}()
	kernel.Register(nil)
}

func TestKernel_ResolvePlugin(t *testing.T) {
	p1 := &mockPlugin{name: "moodle", prefix: "https://campusvirtual.ufro.cl"}
	p2 := &mockPlugin{name: "canvas", prefix: "https://canvas.edu"}

	k := kernel.New(kernel.WithPlugins([]kernel.DownloaderPlugin{p1, p2}))

	resolved, err := k.ResolvePlugin("https://campusvirtual.ufro.cl/resource/123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Name() != "moodle" {
		t.Errorf("expected 'moodle', got '%s'", resolved.Name())
	}

	resolvedCanvas, err := k.ResolvePlugin("https://canvas.edu/courses/456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolvedCanvas.Name() != "canvas" {
		t.Errorf("expected 'canvas', got '%s'", resolvedCanvas.Name())
	}

	_, err = k.ResolvePlugin("https://unsupported.com/file.pdf")
	if !errors.Is(err, kernel.ErrNoPluginFound) {
		t.Errorf("expected ErrNoPluginFound, got %v", err)
	}
}

func TestKernel_Dispatch_SuccessAndEvents(t *testing.T) {
	p := &mockPlugin{name: "test-plugin", prefix: "https://test.com"}
	k := kernel.New(
		kernel.WithPlugins([]kernel.DownloaderPlugin{p}),
		kernel.WithConcurrency(2),
	)

	tasks := []kernel.Task{
		{ID: 1, URL: "https://test.com/file1.pdf"},
		{ID: 2, URL: "https://test.com/file2.pdf"},
	}

	var startedCount, completedCount, progressCount int64
	handler := func(ev kernel.Event) {
		switch ev.Type {
		case kernel.EventTaskStarted:
			atomic.AddInt64(&startedCount, 1)
		case kernel.EventTaskProgress:
			atomic.AddInt64(&progressCount, 1)
		case kernel.EventTaskCompleted:
			atomic.AddInt64(&completedCount, 1)
		}
	}

	results := k.Dispatch(context.Background(), tasks, handler)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for _, res := range results {
		if res.Err != nil {
			t.Errorf("task %d unexpected error: %v", res.TaskID, res.Err)
		}
		if res.Filename != "mock.pdf" {
			t.Errorf("expected filename 'mock.pdf', got '%s'", res.Filename)
		}
	}

	if atomic.LoadInt64(&startedCount) != 2 {
		t.Errorf("expected 2 started events, got %d", startedCount)
	}
	if atomic.LoadInt64(&completedCount) != 2 {
		t.Errorf("expected 2 completed events, got %d", completedCount)
	}
	if atomic.LoadInt64(&progressCount) != 2 {
		t.Errorf("expected 2 progress events, got %d", progressCount)
	}
}

func TestKernel_Dispatch_TaskFailure(t *testing.T) {
	expectedErr := errors.New("network failure")
	p := &mockPlugin{name: "failing", prefix: "https://fail.com", failWith: expectedErr}
	k := kernel.New(kernel.WithPlugins([]kernel.DownloaderPlugin{p}))

	tasks := []kernel.Task{
		{ID: 1, URL: "https://fail.com/res1"},
	}

	var failedCount int64
	handler := func(ev kernel.Event) {
		if ev.Type == kernel.EventTaskFailed {
			atomic.AddInt64(&failedCount, 1)
		}
	}

	results := k.Dispatch(context.Background(), tasks, handler)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Err, expectedErr) {
		t.Errorf("expected %v, got %v", expectedErr, results[0].Err)
	}
	if atomic.LoadInt64(&failedCount) != 1 {
		t.Errorf("expected 1 failed event, got %d", failedCount)
	}
}

func TestKernel_Dispatch_ContextCancellation(t *testing.T) {
	p := &mockPlugin{name: "slow", prefix: "https://slow.com", delay: 100 * time.Millisecond}
	k := kernel.New(
		kernel.WithPlugins([]kernel.DownloaderPlugin{p}),
		kernel.WithConcurrency(1),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	tasks := []kernel.Task{
		{ID: 1, URL: "https://slow.com/slow1"},
	}

	results := k.Dispatch(ctx, tasks, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", results[0].Err)
	}
}
