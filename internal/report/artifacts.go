package report

import (
	"sort"

	"jfrog_manager/internal/models"
)

// Views of the artifact list.
const (
	ViewLatest = "latest"
	ViewAll    = "all"
)

// ArtifactRow is an artifact annotated for the artifact list.
type ArtifactRow struct {
	models.Artifact
	Package   string
	Version   string
	Component string
	// Latest marks the most recently modified version of its package.
	Latest bool
	// Older is the number of other versions of the same package.
	Older int
}

// ComponentCount is an entry of the component filter.
type ComponentCount struct {
	Name   string
	Count  int
	Active bool
}

// ArtifactView is the artifact list of a repository, narrowed to a view
// (latest versions or all files) and optionally to one component.
type ArtifactView struct {
	View      string
	Component string
	Rows      []ArtifactRow
	// Components lists every component with its row count in the current view.
	Components []ComponentCount
	// InView is the number of rows in the current view before the component filter.
	InView   int
	Files    int
	Packages int
}

// NormalizeView returns the view to use for the given input, defaulting to latest.
func NormalizeView(view string) string {
	if view == ViewAll {
		return ViewAll
	}
	return ViewLatest
}

// BuildArtifactView annotates the artifacts of a repository and narrows them
// to the requested view and component. An empty component means all of them.
func BuildArtifactView(artifacts []models.Artifact, view, component string) ArtifactView {
	view = NormalizeView(view)
	root := CommonRoot(artifacts)

	latest := map[string]bool{}
	for _, a := range LatestArtifacts(artifacts) {
		latest[a.Path] = true
	}
	versions := map[string]int{}
	for _, a := range artifacts {
		versions[packageKey(a)]++
	}

	result := ArtifactView{View: view, Component: component, Rows: []ArtifactRow{}, Files: len(artifacts), Packages: len(latest)}
	counts := map[string]int{}
	for _, a := range artifacts {
		if view == ViewLatest && !latest[a.Path] {
			continue
		}
		row := ArtifactRow{
			Artifact:  a,
			Package:   PackageName(a.Name),
			Version:   VersionOf(a.Name),
			Component: ComponentOf(a.Path, root),
			Latest:    latest[a.Path],
			Older:     versions[packageKey(a)] - 1,
		}
		counts[row.Component]++
		result.InView++
		if component == "" || row.Component == component {
			result.Rows = append(result.Rows, row)
		}
	}

	sort.SliceStable(result.Rows, func(i, j int) bool {
		a, b := result.Rows[i], result.Rows[j]
		if a.Component != b.Component {
			return a.Component < b.Component
		}
		if a.Package != b.Package {
			return a.Package < b.Package
		}
		if a.LastModified != b.LastModified {
			return modifiedAfter(a.LastModified, b.LastModified)
		}
		return a.Path < b.Path
	})

	for name, count := range counts {
		result.Components = append(result.Components, ComponentCount{Name: name, Count: count, Active: name == component})
	}
	sort.Slice(result.Components, func(i, j int) bool { return result.Components[i].Name < result.Components[j].Name })

	return result
}
