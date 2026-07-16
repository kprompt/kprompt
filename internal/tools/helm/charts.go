package helm

import "strings"

// Chart describes a Helm chart install recipe.
type Chart struct {
	AppName  string
	RepoName string
	RepoURL  string
	ChartRef string // e.g. bitnami/redis
}

var knownCharts = map[string]Chart{
	"redis": {
		AppName:  "redis",
		RepoName: "bitnami",
		RepoURL:  "https://charts.bitnami.com/bitnami",
		ChartRef: "bitnami/redis",
	},
	"postgresql": {
		AppName:  "postgresql",
		RepoName: "bitnami",
		RepoURL:  "https://charts.bitnami.com/bitnami",
		ChartRef: "bitnami/postgresql",
	},
	"mongodb": {
		AppName:  "mongodb",
		RepoName: "bitnami",
		RepoURL:  "https://charts.bitnami.com/bitnami",
		ChartRef: "bitnami/mongodb",
	},
	"nginx": {
		AppName:  "nginx",
		RepoName: "bitnami",
		RepoURL:  "https://charts.bitnami.com/bitnami",
		ChartRef: "bitnami/nginx",
	},
}

// Lookup returns a known chart recipe by app name.
func Lookup(app string) (Chart, bool) {
	c, ok := knownCharts[strings.ToLower(strings.TrimSpace(app))]
	return c, ok
}

// FromParams builds a chart spec from explicit intent params.
func FromParams(chartRef, repoName, repoURL string) (Chart, bool) {
	chartRef = strings.TrimSpace(chartRef)
	if chartRef == "" {
		return Chart{}, false
	}
	repoName = strings.TrimSpace(repoName)
	if repoName == "" {
		if i := strings.Index(chartRef, "/"); i > 0 {
			repoName = chartRef[:i]
		}
	}
	repoURL = strings.TrimSpace(repoURL)
	if repoURL == "" {
		return Chart{}, false
	}
	return Chart{
		AppName:  chartRef,
		RepoName: repoName,
		RepoURL:  repoURL,
		ChartRef: chartRef,
	}, true
}
