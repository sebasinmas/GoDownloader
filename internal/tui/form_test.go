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

func TestNormalizeCookie(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "MoodleSession=abc123xyz",
			expected: "MoodleSession=abc123xyz",
		},
		{
			input:    "  Cookie: MoodleSession=abc123xyz  ",
			expected: "MoodleSession=abc123xyz",
		},
		{
			input:    `"MoodleSession=abc123xyz"`,
			expected: "MoodleSession=abc123xyz",
		},
		{
			input:    "abc123xyz",
			expected: "MoodleSession=abc123xyz",
		},
		{
			input:    "custom_session=123; tracking=456",
			expected: "custom_session=123; tracking=456",
		},
		{
			input:    "MoodleSession=MoodleSession=abc123xyz",
			expected: "MoodleSession=abc123xyz",
		},
		{
			input:    "Cookie: MoodleSession=MoodleSession=abc123xyz",
			expected: "MoodleSession=abc123xyz",
		},
		{
			input:    `MoodleSession:"p07s7j0vkcf687k3a6m68nu5c9"`,
			expected: "MoodleSession=p07s7j0vkcf687k3a6m68nu5c9",
		},
		{
			input:    `"MoodleSession": "p07s7j0vkcf687k3a6m68nu5c9"`,
			expected: "MoodleSession=p07s7j0vkcf687k3a6m68nu5c9",
		},
		{
			input:    `MoodleSession="p07s7j0vkcf687k3a6m68nu5c9"`,
			expected: "MoodleSession=p07s7j0vkcf687k3a6m68nu5c9",
		},
		{
			input:    `"p07s7j0vkcf687k3a6m68nu5c9"`,
			expected: "MoodleSession=p07s7j0vkcf687k3a6m68nu5c9",
		},
	}

	for _, tc := range tests {
		actual := tui.NormalizeCookie(tc.input)
		if actual != tc.expected {
			t.Errorf("input '%s': expected '%s', got '%s'", tc.input, tc.expected, actual)
		}
	}
}
