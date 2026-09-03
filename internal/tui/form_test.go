package tui_test

import (
	"reflect"
	"testing"

	"godownloader/internal/tui"
)

func TestCleanURLs(t *testing.T) {
	raw := `
   https://campusvirtual.ufro.cl/mod/resource/Apunte%201.pdf   
"https://campusvirtual.ufro.cl/mod/resource/Guia%202.pdf"
'https://campusvirtual.ufro.cl/mod/resource/Calculo.pdf'
<https://campusvirtual.ufro.cl/mod/resource/Algebra.pdf>

   
not-a-valid-url
ftp://unsupported-scheme.com/file.pdf
https://campusvirtual.ufro.cl/mod/resource/Apunte%201.pdf
`

	expected := []string{
		"https://campusvirtual.ufro.cl/mod/resource/Apunte%201.pdf",
		"https://campusvirtual.ufro.cl/mod/resource/Guia%202.pdf",
		"https://campusvirtual.ufro.cl/mod/resource/Calculo.pdf",
		"https://campusvirtual.ufro.cl/mod/resource/Algebra.pdf",
	}

	actual := tui.CleanURLs(raw)
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("CleanURLs mismatch.\nExpected: %#v\nActual:   %#v", expected, actual)
	}
}

func TestCleanURLs_Empty(t *testing.T) {
	if len(tui.CleanURLs("")) != 0 {
		t.Errorf("expected empty slice for empty string")
	}
	if len(tui.CleanURLs("   \n\n  \t  ")) != 0 {
		t.Errorf("expected empty slice for whitespace string")
	}
}
