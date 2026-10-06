// Package report builds the repository-wide vulnerability report: it picks the
// latest artifact of each package, flattens Xray results into rows, sorts them
// and writes them as CSV.
package report

import (
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"jfrog_manager/internal/models"
)

// rootComponent is the component of an artifact that sits in no folder at all.
const rootComponent = "(root)"

// versionStart matches the package name in front of a version: the shortest
// prefix followed by a dash and a digit, optionally "v"-prefixed.
var versionStart = regexp.MustCompile(`^(.+?)-v?\d`)

// rpmArch matches the architecture token of an RPM file name.
var rpmArch = regexp.MustCompile(`\.([A-Za-z0-9_]+)\.rpm$`)

// modifiedLayouts are the timestamp layouts Artifactory uses for lastModified.
var modifiedLayouts = []string{time.RFC3339, "2006-01-02T15:04:05.000-0700"}

// CommonRoot returns the longest directory prefix shared by the directories of
// all artifacts, comparing whole path segments. It is empty when the artifacts
// share no leading folder.
func CommonRoot(artifacts []models.Artifact) string {
	if len(artifacts) == 0 {
		return ""
	}
	common := dirSegments(artifacts[0].Path)
	for _, a := range artifacts[1:] {
		segs := dirSegments(a.Path)
		n := 0
		for n < len(common) && n < len(segs) && common[n] == segs[n] {
			n++
		}
		common = common[:n]
	}
	return strings.Join(common, "/")
}

// ComponentOf returns the component an artifact belongs to: the first path
// segment below root. An artifact sitting directly in root gets the last
// segment of root, or "(root)" when there is no root either.
func ComponentOf(artifactPath, root string) string {
	dir := strings.Join(dirSegments(artifactPath), "/")
	rest := strings.TrimPrefix(strings.TrimPrefix(dir, root), "/")
	if rest != "" {
		return strings.SplitN(rest, "/", 2)[0]
	}
	if root != "" {
		return path.Base(root)
	}
	return rootComponent
}

// PackageName returns the package name of a file: the part in front of the
// version, or the file name without its extension when no version is found.
func PackageName(fileName string) string {
	if m := versionStart.FindStringSubmatch(fileName); m != nil {
		return m[1]
	}
	return strings.TrimSuffix(fileName, path.Ext(fileName))
}

// VersionOf returns the version part of a file name: what is left between the
// package name and the variant. It is empty when the name carries no version.
func VersionOf(fileName string) string {
	pkg := PackageName(fileName)
	rest := strings.TrimPrefix(fileName, pkg)
	if !strings.HasPrefix(rest, "-") {
		return ""
	}
	rest = strings.TrimPrefix(rest, "-")
	if variant := variantOf(fileName); variant != "" {
		rest = strings.TrimSuffix(rest, "."+variant)
	}
	return rest
}

// variantOf returns what distinguishes builds of the same package version:
// the file extension, plus the architecture for RPMs.
func variantOf(fileName string) string {
	if m := rpmArch.FindStringSubmatch(fileName); m != nil {
		return m[1] + ".rpm"
	}
	return strings.TrimPrefix(path.Ext(fileName), ".")
}

// LatestArtifacts keeps the most recently modified artifact of each package.
// Artifacts belong to the same package when they share folder, package name
// and variant. The result is ordered by path.
func LatestArtifacts(artifacts []models.Artifact) []models.Artifact {
	latest := map[string]models.Artifact{}
	for _, a := range artifacts {
		key := packageKey(a)
		current, seen := latest[key]
		if !seen || modifiedAfter(a.LastModified, current.LastModified) {
			latest[key] = a
		}
	}

	result := make([]models.Artifact, 0, len(latest))
	for _, a := range latest {
		result = append(result, a)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}

// packageKey identifies the package an artifact is a version of: its folder,
// package name and variant.
func packageKey(a models.Artifact) string {
	return path.Dir(a.Path) + "\x00" + PackageName(a.Name) + "\x00" + variantOf(a.Name)
}

// modifiedAfter reports whether timestamp a is later than b. Timestamps that
// cannot be parsed are compared as strings.
func modifiedAfter(a, b string) bool {
	ta, okA := parseModified(a)
	tb, okB := parseModified(b)
	if okA && okB {
		return ta.After(tb)
	}
	return a > b
}

func parseModified(s string) (time.Time, bool) {
	for _, layout := range modifiedLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// dirSegments returns the folder segments of an artifact path, without the file name.
func dirSegments(artifactPath string) []string {
	dir := path.Dir(strings.Trim(artifactPath, "/"))
	if dir == "." || dir == "" {
		return nil
	}
	return strings.Split(dir, "/")
}
