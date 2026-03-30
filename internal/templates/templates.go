package templates

import (
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
			r := strings.NewReplacer("/", "-", ".", "-", " ", "-")
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

	return tmpl, nil
}
