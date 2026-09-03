package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"godownloader/internal/kernel"
)

// Styling definitions with Lip Gloss
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1).
			MarginBottom(1)

	cardBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(1, 2).
			MarginTop(1)

	successBadge = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#50FA7B"))

	failedBadge = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF5555"))

	downloadingBadge = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#8BE9FD"))

	pendingBadge = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6272A4"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6272A4"))

	boldStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#F8F8F2"))
)

type itemState struct {
	taskID   int
	url      string
	filename string
	status   kernel.EventType
	bytes    int64
	total    int64
	err      error
}

type kernelEventMsg kernel.Event
type allDoneMsg struct {
	results []kernel.Result
}

type progressModel struct {
	kernel      *kernel.Kernel
	tasks       []kernel.Task
	items       []itemState
	spinner     spinner.Model
	startTime   time.Time
	done        bool
	results     []kernel.Result
	eventChan   chan kernel.Event
	logFilePath string
}

func newProgressModel(k *kernel.Kernel, tasks []kernel.Task, logFilePath string) *progressModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#BD93F9"))

	items := make([]itemState, len(tasks))
	for i, t := range tasks {
		items[i] = itemState{
			taskID:   t.ID,
			url:      t.URL,
			filename: fmt.Sprintf("file_%d", t.ID),
			status:   kernel.EventType(-1), // Initial pending state
		}
	}

	return &progressModel{
		kernel:      k,
		tasks:       tasks,
		items:       items,
		spinner:     s,
		startTime:   time.Now(),
		eventChan:   make(chan kernel.Event, 100),
		logFilePath: logFilePath,
	}
}

func (m *progressModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.startDispatcher(),
		m.waitForEvents(),
	)
}

func (m *progressModel) startDispatcher() tea.Cmd {
	return func() tea.Msg {
		results := m.kernel.Dispatch(context.Background(), m.tasks, func(ev kernel.Event) {
			m.eventChan <- ev
		})
		close(m.eventChan)
		return allDoneMsg{results: results}
	}
}

func (m *progressModel) waitForEvents() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-m.eventChan
		if !ok {
			return nil
		}
		return kernelEventMsg(ev)
	}
}

func (m *progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" || (m.done && msg.String() == "enter") {
			return m, tea.Quit
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case kernelEventMsg:
		m.applyEvent(kernel.Event(msg))
		return m, m.waitForEvents()

	case allDoneMsg:
		m.done = true
		m.results = msg.results
		return m, nil
	}

	return m, nil
}

func (m *progressModel) applyEvent(ev kernel.Event) {
	for i := range m.items {
		if m.items[i].taskID == ev.TaskID {
			m.items[i].status = ev.Type
			if ev.Filename != "" {
				m.items[i].filename = ev.Filename
			}
			m.items[i].bytes = ev.Bytes
			m.items[i].total = ev.Total
			m.items[i].err = ev.Err
			break
		}
	}
}

func (m *progressModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("⚡ GoDownloader • Concurrent Resource Transfer"))
	b.WriteString("\n\n")

	for _, it := range m.items {
		b.WriteString(m.renderItem(it))
		b.WriteString("\n")
	}

	if m.done {
		b.WriteString(m.renderSummary())
	}

	return b.String()
}

func (m *progressModel) renderItem(it itemState) string {
	var statusBadge string
	switch it.status {
	case kernel.EventTaskCompleted:
		statusBadge = successBadge.Render("  ✓ DONE   ")
	case kernel.EventTaskFailed:
		statusBadge = failedBadge.Render("  ✗ FAILED ")
	case kernel.EventTaskProgress:
		statusBadge = downloadingBadge.Render(fmt.Sprintf("%s DOWNLOADING", m.spinner.View()))
	case kernel.EventTaskStarted:
		statusBadge = downloadingBadge.Render(fmt.Sprintf("%s STARTING   ", m.spinner.View()))
	default:
		statusBadge = pendingBadge.Render("  • PENDING  ")
	}

	filename := boldStyle.Render(truncateString(it.filename, 32))
	progressText := dimStyle.Render(formatProgress(it.bytes, it.total))

	if it.err != nil {
		progressText = failedBadge.Render(fmt.Sprintf("(%s)", it.err.Error()))
	}

	return fmt.Sprintf(" %s %-32s  %s", statusBadge, filename, progressText)
}

func (m *progressModel) renderSummary() string {
	var successful, failed int
	var totalBytes int64

	for _, r := range m.results {
		if r.Err == nil {
			successful++
			totalBytes += r.BytesRead
		} else {
			failed++
		}
	}

	duration := time.Since(m.startTime).Round(time.Millisecond)

	logLine := ""
	if m.logFilePath != "" {
		logLine = fmt.Sprintf("  Debug Log:        %s\n", boldStyle.Render(m.logFilePath))
	}

	helpHint := dimStyle.Render("Press Enter or 'q' to exit.")
	if failed > 0 && m.logFilePath != "" {
		helpHint = fmt.Sprintf("%s\n  %s", failedBadge.Render("Check debug log for error causes."), helpHint)
	}

	summaryText := fmt.Sprintf(
		"Downloads Completed in %s\n\n"+
			"  Total Files:      %d\n"+
			"  %s:   %d\n"+
			"  %s:        %d\n"+
			"  Transferred:      %s\n"+
			"%s\n"+
			"%s",
		duration,
		len(m.tasks),
		successBadge.Render("Successful"),
		successful,
		failedBadge.Render("Failed"),
		failed,
		formatBytes(totalBytes),
		logLine,
		helpHint,
	)

	return cardBorder.Render(summaryText) + "\n"
}

// RunProgressUI starts the Bubble Tea parallel download progress interface.
func RunProgressUI(k *kernel.Kernel, tasks []kernel.Task, logFilePath string) ([]kernel.Result, error) {
	model := newProgressModel(k, tasks, logFilePath)
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to run progress UI: %w", err)
	}

	if pm, ok := finalModel.(*progressModel); ok {
		return pm.results, nil
	}
	return nil, nil
}

func formatBytes(b int64) string {
	if b <= 0 {
		return "0 B"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatProgress(bytes, total int64) string {
	if total <= 0 {
		return formatBytes(bytes)
	}
	pct := float64(bytes) / float64(total) * 100.0
	if pct > 100 {
		pct = 100
	}
	return fmt.Sprintf("%s / %s (%.0f%%)", formatBytes(bytes), formatBytes(total), pct)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
