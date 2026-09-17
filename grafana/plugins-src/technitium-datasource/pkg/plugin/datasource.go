package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/data"

	"github.com/trooperthorn/technitium-dns/pkg/models"
)

var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

// Datasource queries a Technitium DNS Server's HTTP API for dashboard
// statistics: query volume over time, response totals, and the top
// clients/domains/blocked-domains tables.
type Datasource struct {
	client *technitiumClient
}

func NewDatasource(_ context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	config, err := models.LoadPluginSettings(settings)
	if err != nil {
		return nil, fmt.Errorf("loading settings: %w", err)
	}

	return &Datasource{
		client: newTechnitiumClient(config.URL, config.Secrets.APIToken),
	}, nil
}

func (d *Datasource) Dispose() {}

// CheckHealth backs the "Save & test" button on the datasource config page.
// It calls the same stats endpoint QueryData does, over the shortest
// window, so a bad URL, an invalid token, and network failures all show up
// the same way "Save & test" would find a real query failing.
func (d *Datasource) CheckHealth(_ context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	config, err := models.LoadPluginSettings(*req.PluginContext.DataSourceInstanceSettings)
	if err != nil {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "Unable to load settings"}, nil
	}
	if config.URL == "" {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "Server URL is missing"}, nil
	}
	if config.Secrets.APIToken == "" {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "API token is missing"}, nil
	}

	client := newTechnitiumClient(config.URL, config.Secrets.APIToken)
	if _, err := client.getDashboardStats("LastHour"); err != nil {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: err.Error()}, nil
	}

	return &backend.CheckHealthResult{Status: backend.HealthStatusOk, Message: "Successfully queried Technitium DNS Server"}, nil
}

// queryModel is the JSON the frontend query editor sends per query row.
type queryModel struct {
	// StatType is one of LastHour, LastDay, LastWeek, LastMonth, LastYear,
	// as Technitium's /api/dashboard/stats/get documents.
	StatType string `json:"statType"`
	// Series selects which part of the response becomes this query's
	// frame(s): "volume" (the time series), "topClients", "topDomains",
	// or "topBlockedDomains" (each a table of name+hits).
	Series string `json:"series"`
}

func (d *Datasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	response := backend.NewQueryDataResponse()

	for _, q := range req.Queries {
		response.Responses[q.RefID] = d.query(ctx, q)
	}

	return response, nil
}

func (d *Datasource) query(_ context.Context, query backend.DataQuery) backend.DataResponse {
	var qm queryModel
	if err := json.Unmarshal(query.JSON, &qm); err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("json unmarshal: %v", err))
	}
	if qm.StatType == "" {
		qm.StatType = "LastDay"
	}
	if qm.Series == "" {
		qm.Series = "volume"
	}

	stats, err := d.client.getDashboardStats(qm.StatType)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusInternal, err.Error())
	}

	var frame *data.Frame
	switch qm.Series {
	case "volume":
		frame = volumeFrame(stats)
	case "topClients":
		frame = topNFrame("top_clients", stats.Response.TopClients)
	case "topDomains":
		frame = topNFrame("top_domains", stats.Response.TopDomains)
	case "topBlockedDomains":
		frame = topNFrame("top_blocked_domains", stats.Response.TopBlockedDomains)
	default:
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("unknown series %q", qm.Series))
	}

	return backend.DataResponse{Frames: data.Frames{frame}}
}

// volumeFrame turns mainChartData's labels/datasets into a wide time series
// frame: one "time" field plus one numeric field per dataset (e.g. Total
// Queries, No Error, Server Failure, ...).
func volumeFrame(stats *dashboardStatsResponse) *data.Frame {
	chart := stats.Response.MainChartData
	frame := data.NewFrame("dashboard_stats",
		data.NewField("time", nil, chart.Labels),
	)
	for _, ds := range chart.Datasets {
		frame.Fields = append(frame.Fields, data.NewField(ds.Label, nil, ds.Data))
	}
	frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeGraph}
	return frame
}

func topNFrame(name string, rows []topRow) *data.Frame {
	names := make([]string, 0, len(rows))
	hits := make([]float64, 0, len(rows))
	for _, r := range rows {
		names = append(names, r.Name)
		hits = append(hits, r.Hits)
	}

	frame := data.NewFrame(name,
		data.NewField("name", nil, names),
		data.NewField("hits", nil, hits),
	)
	frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
	return frame
}
