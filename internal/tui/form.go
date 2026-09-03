// Package tui provides terminal user interfaces using Huh and Bubble Tea.
package tui

import (
	"errors"
	"net/url"
	"strings"

	"github.com/charmbracelet/huh"
)

var (
	// ErrFormAborted indicates the user cancelled or exited the interactive form.
	ErrFormAborted = errors.New("form aborted by user")
)

// CleanURLs parses, sanitizes, and filters raw pasted multiline URL text.
// It trims whitespace, strips stray quotation marks or angle brackets,
// skips empty lines, and filters out non-HTTP/HTTPS URLs.
func CleanURLs(raw string) []string {
	lines := strings.Split(raw, "\n")
	var result []string
	seen := make(map[string]bool)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		trimmed = strings.Trim(trimmed, `"'<>`)
		if trimmed == "" {
			continue
		}

		u, err := url.ParseRequestURI(trimmed)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			continue
		}

		if !seen[trimmed] {
			seen[trimmed] = true
			result = append(result, trimmed)
		}
	}

	return result
}

// FormData contains the sanitized inputs gathered from the interactive Huh form.
type FormData struct {
	Cookie string
	URLs   []string
}

// RunInteractiveForm launches the 2-step Huh form:
// Step 1: Session Cookie
// Step 2: Multi-line URLs textarea
func RunInteractiveForm() (*FormData, error) {
	var (
		cookie  string
		rawURLs string
	)

	theme := huh.ThemeCharm()

	step1 := huh.NewGroup(
		huh.NewInput().
			Key("cookie").
			Title("Step 1: Session Cookie").
			Description("Enter your session cookie (e.g., MoodleSession=123...):\n(Kept strictly in memory. Never logged or saved to disk)").
			Placeholder("MoodleSession=abc123xyz456").
			Value(&cookie).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return errors.New("session cookie cannot be empty")
				}
				return nil
			}),
	)

	step2 := huh.NewGroup(
		huh.NewText().
			Key("urls").
			Title("Step 2: Resource URLs").
			Description("Paste your links here (one per line, press Esc then Enter to submit):").
			Placeholder("https://campusvirtual.ufro.cl/mod/resource/view.php?id=123/Apunte%201.pdf\nhttps://campusvirtual.ufro.cl/mod/resource/view.php?id=124/Guia%202.pdf").
			Value(&rawURLs).
			Lines(8).
			Validate(func(s string) error {
				valid := CleanURLs(s)
				if len(valid) == 0 {
					return errors.New("please paste at least one valid HTTP/HTTPS URL")
				}
				return nil
			}),
	)

	form := huh.NewForm(step1, step2).WithTheme(theme)

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, ErrFormAborted
		}
		return nil, err
	}

	return &FormData{
		Cookie: strings.TrimSpace(cookie),
		URLs:   CleanURLs(rawURLs),
	}, nil
}
