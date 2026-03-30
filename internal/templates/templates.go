package templates

import (
	"fmt"
	"html/template"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"jfrog_manager/internal/models"
)

var severityOrder = map[string]int{
	"critical": 0,
	"high":     1,
	"medium":   2,
	"low":      3,
}

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
		"sortBySeverity": func(issues []models.XrayIssue) []models.XrayIssue {
			sorted := make([]models.XrayIssue, len(issues))
			copy(sorted, issues)
			sort.SliceStable(sorted, func(i, j int) bool {
				oi := severityOrder[strings.ToLower(sorted[i].Severity)]
				oj := severityOrder[strings.ToLower(sorted[j].Severity)]
				return oi < oj
			})
			return sorted
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
