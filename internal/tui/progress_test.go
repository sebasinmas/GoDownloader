package tui

import (
	"strings"
	"testing"
	"time"

	"godownloader/internal/kernel"
)

func TestProgressModel_RenderVisual(t *testing.T) {
	tasks := []kernel.Task{
		{ID: 1, URL: "https://campusvirtual.ufro.cl/mod/resource/view.php?id=101/Clase_01.pdf"},
		{ID: 2, URL: "https://campusvirtual.ufro.cl/mod/resource/view.php?id=102/Guia_02.pdf"},
		{ID: 3, URL: "https://campusvirtual.ufro.cl/mod/resource/view.php?id=103/Lectura.pdf"},
	}

	m := newProgressModel(nil, tasks, "")
	m.done = true
	m.totalDuration = 2340 * time.Millisecond
	m.results = []kernel.Result{
		{TaskID: 1, URL: tasks[0].URL, Filename: "Clase_01.pdf", BytesRead: 2450000, TotalBytes: 2450000},
		{TaskID: 2, URL: tasks[1].URL, Filename: "Guia_02.pdf", BytesRead: 4890000, TotalBytes: 4890000},
		{TaskID: 3, URL: tasks[2].URL, Filename: "Lectura.pdf", BytesRead: 1780000, TotalBytes: 1780000},
	}

	out := m.View()
	if !strings.Contains(out, "GoDownloader") {
		t.Errorf("expected view to contain 'GoDownloader'")
	}
	if !strings.Contains(out, "Descargas completadas") {
		t.Errorf("expected view to contain 'Descargas completadas'")
	}
	if !strings.Contains(out, "SebaSinMas") {
		t.Errorf("expected view to contain 'SebaSinMas'")
	}
}
