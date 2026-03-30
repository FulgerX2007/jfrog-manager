package templates

import (
	"fmt"
	"html/template"
	"net/url"
	"path/filepath"
	"strings"

	"jfrog_manager/internal/models"
)

// FuncMap returns the custom template functions used by the application templates.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"toLower":   strings.ToLower,
		"urlEncode": url.QueryEscape,
		"cssID": func(s string) string {
			// Escape underscores first to ensure injectivity — without this,
			// "a/b" and "a_s_b" would both map to "a_s_b".
			s = strings.ReplaceAll(s, "_", "__")
			r := strings.NewReplacer("/", "_s", ".", "_d", " ", "_w", "-", "_h")
			return r.Replace(s)
		},
		"countBySeverity": func(issues []models.XrayIssue, severity string) int {
			count := 0
			for _, issue := range issues {
				if strings.EqualFold(issue.Severity, severity) {
					count++
				}
			}
			return count
		},
	}
}

// Load parses all templates from the given root directory, including subdirectories.
// It registers custom template functions needed by the templates.
func Load(templateDir string) (*template.Template, error) {
	patterns := []string{
		filepath.Join(templateDir, "*.html"),
		filepath.Join(templateDir, "partials", "*.html"),
	}

	tmpl := template.New("").Funcs(FuncMap())

	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		if len(matches) > 0 {
			tmpl, err = tmpl.ParseFiles(matches...)
			if err != nil {
				return nil, err
			}
		}
	}

	// Verify at least one real template was loaded to catch misconfigured template directories
	// early at startup rather than at runtime when ExecuteTemplate is called.
	// template.New("") already creates one root template, so <= 1 means no files were parsed.
	if len(tmpl.Templates()) <= 1 {
		return nil, fmt.Errorf("no templates found in %q", templateDir)
	}

	return tmpl, nil
}
