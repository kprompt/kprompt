package grafana

import "context"

// Querier is the read-only dashboard capability exposed to CLI intents.
type Querier interface {
	ListDashboards(context.Context, SearchRequest) ([]DashboardSummary, error)
	GetDashboard(context.Context, string) (Dashboard, error)
}

// SearchRequest filters Grafana dashboards.
type SearchRequest struct {
	Query string
	Tag   string
	Limit int
}

// ShowRequest identifies a dashboard by UID or search text.
type ShowRequest struct {
	UID   string
	Query string
	Limit int
}

// ShowResult is either one detailed dashboard or a list of search matches.
type ShowResult struct {
	Query      string             `json:"query,omitempty"`
	Dashboard  *Dashboard         `json:"dashboard,omitempty"`
	Dashboards []DashboardSummary `json:"dashboards,omitempty"`
}

// DashboardSummary is one dashboard returned by Grafana search.
type DashboardSummary struct {
	UID         string   `json:"uid"`
	Title       string   `json:"title"`
	URL         string   `json:"url,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	FolderUID   string   `json:"folderUid,omitempty"`
	FolderTitle string   `json:"folderTitle,omitempty"`
	Starred     bool     `json:"starred,omitempty"`
}

// Dashboard contains stable dashboard identity and flattened panel metadata.
type Dashboard struct {
	UID    string   `json:"uid"`
	Title  string   `json:"title"`
	URL    string   `json:"url,omitempty"`
	Tags   []string `json:"tags,omitempty"`
	Panels []Panel  `json:"panels"`
}

// Panel is backend-neutral Grafana panel metadata.
type Panel struct {
	ID         int          `json:"id"`
	Title      string       `json:"title"`
	Type       string       `json:"type"`
	Datasource Datasource   `json:"datasource,omitempty"`
	Grid       GridPosition `json:"grid,omitempty"`
	Targets    []Target     `json:"targets,omitempty"`
}

// Datasource identifies a panel datasource across Grafana JSON formats.
type Datasource struct {
	UID  string `json:"uid,omitempty"`
	Type string `json:"type,omitempty"`
	Name string `json:"name,omitempty"`
}

// GridPosition locates a panel on a dashboard.
type GridPosition struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Target describes one panel query without retaining arbitrary query payloads.
type Target struct {
	RefID      string `json:"refId,omitempty"`
	Datasource string `json:"datasource,omitempty"`
	Expression string `json:"expression,omitempty"`
	Hidden     bool   `json:"hidden,omitempty"`
}
