package grafana

import "encoding/json"

type searchDashboard struct {
	UID         string   `json:"uid"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Tags        []string `json:"tags"`
	Type        string   `json:"type"`
	FolderUID   string   `json:"folderUid"`
	FolderTitle string   `json:"folderTitle"`
	IsStarred   bool     `json:"isStarred"`
}

type dashboardEnvelope struct {
	Dashboard rawDashboard  `json:"dashboard"`
	Meta      dashboardMeta `json:"meta"`
}

type rawDashboard struct {
	UID    string     `json:"uid"`
	Title  string     `json:"title"`
	Tags   []string   `json:"tags"`
	Panels []rawPanel `json:"panels"`
}

type dashboardMeta struct {
	URL string `json:"url"`
}

type rawPanel struct {
	ID         int             `json:"id"`
	Title      string          `json:"title"`
	Type       string          `json:"type"`
	Datasource json.RawMessage `json:"datasource"`
	GridPos    GridPosition    `json:"gridPos"`
	Targets    []rawTarget     `json:"targets"`
	Panels     []rawPanel      `json:"panels"`
}

type rawTarget struct {
	RefID      string          `json:"refId"`
	Datasource json.RawMessage `json:"datasource"`
	Expr       string          `json:"expr"`
	Query      string          `json:"query"`
	Hide       bool            `json:"hide"`
}
